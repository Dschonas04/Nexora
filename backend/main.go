// Command nexora is the API server: it opens the database, applies the schema
// migration and serves the JSON API under /api.
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"nexora/internal/ablage"
	"nexora/internal/config"
	"nexora/internal/db"
	"nexora/internal/handlers"
	"nexora/internal/lizenz"
	"nexora/internal/middleware"
)

func main() {
	// Einstellungen kommen aus config.conf, überschrieben von Umgebungsvariablen,
	// overridden by nothing. See internal/config and config.conf; every value
	// has a default so the server starts without any configuration at all.
	k := config.Laden("")

	// Dangerous defaults are named but not punished: a home lab installation
	// running on the default secret should start, it just must not be possible
	// to overlook.
	for _, w := range k.Warnungen() {
		log.Printf("ACHTUNG: %s", w)
	}

	dbURL := k.DatenbankURL
	secret := k.JWTGeheimnis
	port := k.Port
	dataDir := k.DatenVerzeich

	// Startup budget for connecting and migrating. The pool itself outlives this
	// context, only the setup below is bounded by it.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	// Migrate is idempotent and runs on every boot, so a fresh volume and an
	// existing one end up at the same schema.
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("db migrate: %v", err)
	}
	log.Println("database ready")

	// The license key unlocks the paid extras. If it is missing or no good, the
	// server carries on with the free feature set: an invalid key must never
	// keep it from starting.
	// What was imported through the administration pages wins; the file is the
	// fallback. Otherwise a license could be imported in the browser and would
	// be back to the old one from the file after the next restart.
	schluessel := k.Lizenz
	if ausDB := handlers.LizenzAusDatenbank(ctx, pool); ausDB != "" {
		schluessel = ausDB
	}
	lizenz.Laden(schluessel)
	if z := lizenz.Aktuell(); z.Gueltig {
		log.Printf("Lizenz für %s gültig, freigeschaltet: %v", z.Inhaber, z.Funktionen)
	} else {
		log.Printf("keine gültige Lizenz (%s). Zusatzfunktionen bleiben gesperrt.", z.Grund)
	}

	// Choose where attachments live. The object store is the exception, the disk
	// is the rule: whoever does not set up S3 should never notice it exists.
	//
	// An unreachable object store deliberately falls back to the disk rather
	// than preventing startup. An instance that runs with its new attachments
	// stored locally beats one that does not come up at all, and the fallback is
	// reported loudly.
	var speicher ablage.Ablage = ablage.NeuePlatte(dataDir)
	if k.S3Aktiv && k.S3Endpunkt != "" {
		s3, err := ablage.NeuS3(ctx, ablage.Einstellungen{
			Endpunkt:  k.S3Endpunkt,
			Bucket:    k.S3Bucket,
			Zugriff:   k.S3Zugriff,
			Geheimnis: k.S3Geheimnis,
			Region:    k.S3Region,
			TLS:       k.S3TLS,
			Pfadstil:  k.S3Pfadstil,
		})
		if err != nil {
			log.Printf("ACHTUNG: Objektspeicher nicht erreichbar (%v). Anhänge liegen auf der Platte.", err)
		} else {
			speicher = s3
		}
	}
	log.Printf("Anhänge: %s", speicher.Name())

	// Redis is optional: without it everything keeps working, only without a
	// shared cache. A failure to connect is therefore logged, not fatal.
	rd := handlers.NeuRedis(ctx, k.RedisAdresse, k.RedisPasswort, k.RedisDatenbank, k.RedisVorsilbe)
	defer rd.Schliessen()

	h := &handlers.Server{
		Pool:      pool,
		Secret:    []byte(secret),
		Ablage:    speicher,
		Sitzungen: handlers.NeuerSitzungsSpeicher(),
		Redis:     rd,
		SSO: handlers.SSOEinstellungen{
			Konf:            k,
			OeffentlicheURL: k.OeffentlicheURL,
		},
	}

	// Pages from before the search index get their plain text filled in. Without
	// that, full text search silently returned nothing for older pages, which
	// looks like an empty result rather than a fault.
	h.IndexNachziehen(ctx)

	// Runtime settings from the database into the cache. They override whatever
	// config.conf says, because they were set later and on purpose.
	h.EinstellungenLaden(ctx, k)

	r := chi.NewRouter()
	r.Use(chimw.RealIP) // trust X-Forwarded-For, the SPA is served through nginx
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer) // a panicking handler must not take the process down
	r.Use(chimw.Timeout(30 * time.Second))

	r.Route("/api", func(r chi.Router) {
		// Public: no session required. Registration is open, and the very first
		// account created becomes the workspace admin.
		r.Post("/auth/register", h.Register)
		r.Post("/auth/login", h.Login)
		r.Post("/auth/logout", h.Logout)
		// Signing in with an outside identity. Public, because nobody is signed
		// in yet, which is rather the point.
		r.Get("/auth/sso", h.SSOZustand)
		r.Get("/auth/oidc/start", h.OIDCStart)
		r.Get("/auth/oidc/zurueck", h.OIDCZurueck)
		r.Post("/auth/ldap", h.LDAPAnmeldung)
		// Public links are a paid extra. The route stays in place but answers
		// 402 without a license instead of serving the page.
		r.With(handlers.VerlangeFunktion(lizenz.Freigeben)).
			Get("/public/{token}", h.GetPublicPage)

		// Everything below requires a valid session cookie. Ownership and sharing
		// are checked per request inside the handlers, not here.
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth([]byte(secret), h.SitzungGilt))

			r.Get("/auth/me", h.Me)

			// Tells the interface what is unlocked so it never offers what is
			// locked. Contains no secret.
			r.Get("/lizenz", h.LizenzStatus)
			// Sessions: see your own and end them one at a time. Locking out one
			// device without taking everybody else with it works no other way.
			// Reading and writing Word attachments. Whoever may read the page may
			// read the file; writing additionally needs write access and the
			// attachments license.
			r.Get("/pages/{id}/attachments/{attId}/word", h.WordLesen)
			r.Put("/pages/{id}/attachments/{attId}/word", h.WordSchreiben)
			r.Get("/sitzungen", h.ListSitzungen)
			r.Delete("/sitzungen", h.SitzungenBeenden)
			r.Delete("/sitzungen/{id}", h.SitzungBeenden)
			// Importing and issuing: both administrators only, checked inside the
			// handler. On an ordinary installation issuing answers 501, because
			// no private key lives there.
			r.Put("/system/lizenz", h.LizenzEinlesen)
			r.Post("/system/lizenz/ausstellen", h.LizenzAusstellen)

			// The static /pages/... routes must be registered before /pages/{id},
			// otherwise chi would match "shared" and "trash" as page ids.
			r.Get("/pages", h.ListPages)
			r.With(handlers.VerlangeFunktion(lizenz.Freigeben)).
				Get("/pages/shared", h.ListSharedPages)
			r.Get("/pages/trash", h.ListTrash)
			r.Post("/pages", h.CreatePage)
			r.Get("/pages/{id}", h.GetPage)
			r.Put("/pages/{id}", h.UpdatePage)
			r.Delete("/pages/{id}", h.DeletePage) // moves to the trash
			r.Post("/pages/{id}/restore", h.RestorePage)
			r.Delete("/pages/{id}/purge", h.PurgePage) // deletes for good, cascades to subpages
			r.Post("/pages/{id}/favorite", h.AddFavorite)
			r.Delete("/pages/{id}/favorite", h.RemoveFavorite)
			r.With(handlers.VerlangeFunktion(lizenz.Freigeben)).
				Post("/pages/{id}/share", h.SharePage)
			r.With(handlers.VerlangeFunktion(lizenz.Freigeben)).
				Delete("/pages/{id}/share", h.UnsharePage)
			r.Post("/pages/{id}/tags", h.AttachTag)
			r.Delete("/pages/{id}/tags/{tagId}", h.DetachTag)

			// Version history, a paid extra. The core keeps writing the snapshots
			// regardless; only viewing and restoring are locked. Otherwise there
			// would be a gap in the history right after unlocking.
			r.Group(func(r chi.Router) {
				r.Use(handlers.VerlangeFunktion(lizenz.Versionen))
				r.Get("/pages/{id}/versions", h.ListVersions)
				r.Get("/pages/{id}/versions/{versionId}", h.GetVersion)
				r.Post("/pages/{id}/versions/{versionId}/restore", h.RestoreVersion)
			})

			// Attachments, a paid extra.
			r.Group(func(r chi.Router) {
				r.Use(handlers.VerlangeFunktion(lizenz.Anhaenge))
				r.Get("/pages/{id}/attachments", h.ListAttachments)
				r.Post("/pages/{id}/attachments", h.UploadAttachment)
				r.Get("/pages/{id}/attachments/{attId}", h.DownloadAttachment)
				r.Delete("/pages/{id}/attachments/{attId}", h.DeleteAttachment)
			})

			// Per-user sharing, a paid extra.
			r.Group(func(r chi.Router) {
				r.Use(handlers.VerlangeFunktion(lizenz.Freigeben))
				r.Get("/pages/{id}/shares", h.ListShares)
				r.Post("/pages/{id}/shares", h.AddShare)
				r.Delete("/pages/{id}/shares/{userId}", h.RemoveShare)
			})

			// Settings and system state. Administrators only, which the handlers
			// check themselves, so there is no extra gate here, only the route.
			// Anyone may read the appearance, otherwise an ordinary user would
			// never see the configured colours, since the settings page itself
			// is closed to them.
			// The inbox. Free like the sidebar itself: it only shows what
			// happened elsewhere anyway, and without comments and shares it
			// simply stays empty.
			r.Get("/postfach", h.ListPostfach)
			r.Get("/postfach/anzahl", h.PostfachAnzahl)
			r.Post("/postfach/gelesen", h.PostfachGelesen)
			r.Post("/postfach/{id}/gelesen", h.PostfachGelesen)
			r.Delete("/postfach", h.PostfachLeeren)

			r.Get("/design", h.Design)

			r.Get("/einstellungen", h.ListEinstellungen)
			r.Put("/einstellungen", h.SetzeEinstellung)
			r.Delete("/einstellungen", h.LoescheEinstellung)
			r.Get("/system", h.SystemZustand)
			r.Post("/system/suchindex", h.IndexNeuAufbauen)
			r.Post("/system/anhangindex", h.AnhangIndexNachziehen)
			r.Get("/system/ablage", h.AblageZustand)
			r.Post("/system/ablage/test", h.S3Testen)

			// Maintenance: configuration file, restart, the instance-wide trash.
			// Here too the handlers check the role themselves.
			r.Get("/system/konfig", h.KonfigLesen)
			r.Put("/system/konfig", h.KonfigSchreiben)
			r.Post("/system/neustart", h.Neustart)
			r.Post("/system/papierkorb", h.PapierkorbLeeren)

			// Comments, a paid extra. Whoever may read the page may join the
			// conversation; it gets no finer than that, and the handlers check
			// it.
			r.Group(func(r chi.Router) {
				r.Use(handlers.VerlangeFunktion(lizenz.Kommentare))
				r.Get("/pages/{id}/kommentare", h.ListKommentare)
				r.Post("/pages/{id}/kommentare", h.CreateKommentar)
				r.Put("/kommentare/{kommentarId}", h.UpdateKommentar)
				r.Delete("/kommentare/{kommentarId}", h.DeleteKommentar)
				r.Post("/kommentare/{kommentarId}/erledigt", h.ToggleErledigt)
			})

			// Groups and space permissions, a paid extra.
			//
			// The extra locks MANAGING them. Whether existing permissions still
			// apply is decided by pagePerm itself: without a license they do not
			// take effect, but neither are they deleted. So everything comes
			// back unchanged once the extra is unlocked again.
			r.Group(func(r chi.Router) {
				r.Use(handlers.VerlangeFunktion(lizenz.Gruppen))
				r.Get("/gruppen", h.ListGruppen)
				r.Post("/gruppen", h.CreateGruppe)
				r.Delete("/gruppen/{id}", h.DeleteGruppe)
				r.Get("/gruppen/{id}/mitglieder", h.ListMitglieder)
				r.Put("/gruppen/{id}/mitglieder", h.SetzeMitglied)
				r.Get("/spaces/{id}/rechte", h.ListSpaceRechte)
				r.Put("/spaces/{id}/rechte", h.SetzeSpaceRecht)
			})

			// Space export, a paid extra. The answer is a ZIP stream, which is
			// why no writeJSON stands behind it.
			r.With(handlers.VerlangeFunktion(lizenz.Export)).
				Get("/spaces/{id}/export", h.ExportSpace)

			// Templates, a paid extra. A template is an ordinary page with a flag
			// on it; only creating and listing them is locked.
			r.Group(func(r chi.Router) {
				r.Use(handlers.VerlangeFunktion(lizenz.Vorlagen))
				r.Get("/vorlagen", h.ListVorlagen)
				r.Post("/pages/{id}/vorlage", h.SetzeVorlage)
			})

			// The audit trail, a paid extra. It is always WRITTEN, license or
			// not: a trail with a hole exactly over the unlicensed period would
			// be worthless. Only reading it costs money.
			r.Group(func(r chi.Router) {
				r.Use(handlers.VerlangeFunktion(lizenz.Pruefspur))
				r.Get("/pruefspur", h.ListPruefspur)
				r.Get("/pruefspur/aktionen", h.PruefspurAktionen)
			})

			// Account administration stays free; without it a multi-user instance
			// could not be run at all. The /users routes are reserved for
			// administrators, which the handlers enforce themselves.
			r.Get("/users", h.ListUsers)
			r.Post("/users", h.CreateUser)
			r.Delete("/users/{id}", h.DeleteUser)
			r.Put("/users/{id}/role", h.SetUserRole)

			// Spaces
			r.Get("/spaces", h.ListSpaces)
			r.Post("/spaces", h.CreateSpace)
			r.Put("/spaces/{id}", h.RenameSpace)
			r.Delete("/spaces/{id}", h.DeleteSpace)
			// Open a space to every signed-in account (not to the internet).
			r.Put("/spaces/{id}/oeffentlich", h.SetSpaceOeffentlich)

			// Backlinks (pages linking here via [[wiki-link]] or manual links)
			// Markdown export of one page. Done on the server so it works without
			// a loaded editor, and because the conversion inside the editor is
			// explicitly lossy.
			r.Get("/pages/{id}/markdown", h.ExportMarkdown)

			// Typeset documents, a paid extra. Markdown stays free: getting your
			// own content out of the system must never depend on a license.
			// PDF and Word are presentation, not an escape route.
			r.With(handlers.VerlangeFunktion(lizenz.Export)).Group(func(r chi.Router) {
				r.Get("/pages/{id}/pdf", h.ExportPDF)
				r.Get("/pages/{id}/word", h.ExportWord)
			})

			// Import: single Markdown files or a whole archive. Free like the
			// Markdown export and for the same reason: the way into the system
			// should depend on a license as little as the way out.
			r.Post("/import", h.Import)

			r.Get("/pages/{id}/backlinks", h.Backlinks)

			// Manual page-to-page links (edited via the UI)
			r.Get("/pages/{id}/links", h.ListLinks)
			r.Post("/pages/{id}/links", h.AddLink)
			r.Delete("/pages/{id}/links/{targetId}", h.RemoveLink)

			// Knowledge graph
			r.Get("/graph", h.Graph)

			r.Get("/favorites", h.ListFavorites)
			r.Get("/tags", h.ListTags)
			r.Post("/tags", h.CreateTag)
			r.Get("/tags/{id}/pages", h.SeitenZuTag)
			r.Delete("/tags/{id}", h.DeleteTag)
			r.Get("/search", h.Search)
		})
	})

	// Liveness probe, deliberately outside /api so it needs no session. It pings
	// the database because a backend without one cannot serve anything useful.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			http.Error(w, "db down", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})

	// The trash sweeps itself. Its own context, not the startup one: that
	// expires after thirty seconds, and this clock should run as long as the
	// service does.
	go h.PapierkorbUhr(context.Background())
	// Expired and revoked sessions disappear after a grace period.
	go h.SitzungenUhr(context.Background())

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second, // guards against slow-header clients
	}
	log.Printf("nexora backend listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
