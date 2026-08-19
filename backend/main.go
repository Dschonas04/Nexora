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
	// überschrieben von nichts. Siehe internal/config und config.conf; jeder
	// Wert hat eine Vorgabe, damit der Server auch ohne Konfiguration startet.
	k := config.Laden("")

	// Gefährliche Vorgaben werden benannt, aber nicht bestraft: eine
	// Heimlabor-Installation mit dem Vorgabegeheimnis soll starten -- man soll
	// es nur nicht übersehen können.
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

	// Der Lizenzschlüssel schaltet die kostenpflichtigen Zusätze frei. Fehlt er
	// oder taugt er nicht, läuft der Server mit dem freien Umfang weiter -- ein
	// ungültiger Schlüssel darf den Start nie verhindern.
	lizenz.Laden(k.Lizenz)
	if z := lizenz.Aktuell(); z.Gueltig {
		log.Printf("Lizenz für %s gültig, freigeschaltet: %v", z.Inhaber, z.Funktionen)
	} else {
		log.Printf("keine gültige Lizenz (%s) -- Zusatzfunktionen bleiben gesperrt", z.Grund)
	}

	// Ablage wählen. Der Objektspeicher ist die Ausnahme, die Platte die Regel:
	// wer S3 nicht einrichtet, soll nichts davon merken.
	//
	// Ein nicht erreichbarer Objektspeicher fällt bewusst auf die Platte zurück,
	// statt den Start zu verhindern. Eine Instanz, die läuft und deren neue
	// Anhänge lokal liegen, ist besser als eine, die gar nicht hochkommt --
	// gemeldet wird es deutlich.
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
			log.Printf("ACHTUNG: Objektspeicher nicht erreichbar (%v) -- Anhänge liegen auf der Platte", err)
		} else {
			speicher = s3
		}
	}
	log.Printf("Anhänge: %s", speicher.Name())

	h := &handlers.Server{Pool: pool, Secret: []byte(secret), Ablage: speicher}

	// Seiten aus der Zeit vor dem Suchindex bekommen ihren Fließtext nachgereicht.
	// Ohne das lieferte die Volltextsuche für ältere Seiten stillschweigend nichts
	// -- das sieht wie ein leeres Ergebnis aus, nicht wie ein Fehler.
	h.IndexNachziehen(ctx)

	// Laufzeiteinstellungen aus der Datenbank in den Zwischenspeicher. Sie
	// überschreiben, was in config.conf steht -- gesetzt wurden sie später und
	// mit Absicht.
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
		// Öffentliche Links gehören zum Zusatzumfang. Die Route bleibt bestehen,
		// antwortet ohne Lizenz aber mit 402 statt die Seite auszuliefern.
		r.With(handlers.VerlangeFunktion(lizenz.Freigeben)).
			Get("/public/{token}", h.GetPublicPage)

		// Everything below requires a valid session cookie. Ownership and sharing
		// are checked per request inside the handlers, not here.
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth([]byte(secret)))

			r.Get("/auth/me", h.Me)

			// Sagt der Oberfläche, was freigeschaltet ist, damit sie Gesperrtes gar
			// nicht erst anbietet. Enthält kein Geheimnis.
			r.Get("/lizenz", h.LizenzStatus)

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

			// Version history -- Zusatz. Die Schnappschüsse selbst schreibt der
			// Kern weiter mit; gesperrt ist nur das Ansehen und Zurückholen. Sonst
			// klaffte nach dem Freischalten eine Lücke in der Geschichte.
			r.Group(func(r chi.Router) {
				r.Use(handlers.VerlangeFunktion(lizenz.Versionen))
				r.Get("/pages/{id}/versions", h.ListVersions)
				r.Get("/pages/{id}/versions/{versionId}", h.GetVersion)
				r.Post("/pages/{id}/versions/{versionId}/restore", h.RestoreVersion)
			})

			// Attachments -- Zusatz
			r.Group(func(r chi.Router) {
				r.Use(handlers.VerlangeFunktion(lizenz.Anhaenge))
				r.Get("/pages/{id}/attachments", h.ListAttachments)
				r.Post("/pages/{id}/attachments", h.UploadAttachment)
				r.Get("/pages/{id}/attachments/{attId}", h.DownloadAttachment)
				r.Delete("/pages/{id}/attachments/{attId}", h.DeleteAttachment)
			})

			// Per-user sharing -- Zusatz
			r.Group(func(r chi.Router) {
				r.Use(handlers.VerlangeFunktion(lizenz.Freigeben))
				r.Get("/pages/{id}/shares", h.ListShares)
				r.Post("/pages/{id}/shares", h.AddShare)
				r.Delete("/pages/{id}/shares/{userId}", h.RemoveShare)
			})

			// Einstellungen und Systemzustand. Ausschließlich für Admins, das
			// prüfen die Handler selbst -- deshalb steht hier keine weitere
			// Hürde, sondern nur die Route.
			// Das Aussehen darf jeder lesen -- sonst sähe ein normaler Benutzer
			// die eingestellten Farben nie, denn die Einstellungsseite selbst ist
			// ihm verwehrt.
			r.Get("/design", h.Design)

			r.Get("/einstellungen", h.ListEinstellungen)
			r.Put("/einstellungen", h.SetzeEinstellung)
			r.Delete("/einstellungen", h.LoescheEinstellung)
			r.Get("/system", h.SystemZustand)
			r.Post("/system/suchindex", h.IndexNeuAufbauen)
			r.Get("/system/ablage", h.AblageZustand)
			r.Post("/system/ablage/test", h.S3Testen)

			// Kommentare -- Zusatz. Wer die Seite lesen darf, darf auch
			// mitreden; feiner wird es hier nicht, das prüfen die Handler.
			r.Group(func(r chi.Router) {
				r.Use(handlers.VerlangeFunktion(lizenz.Kommentare))
				r.Get("/pages/{id}/kommentare", h.ListKommentare)
				r.Post("/pages/{id}/kommentare", h.CreateKommentar)
				r.Put("/kommentare/{kommentarId}", h.UpdateKommentar)
				r.Delete("/kommentare/{kommentarId}", h.DeleteKommentar)
				r.Post("/kommentare/{kommentarId}/erledigt", h.ToggleErledigt)
			})

			// Prüfspur -- Zusatz. GESCHRIEBEN wird sie immer, auch ohne
			// Lizenz: eine Spur mit einem Loch genau über dem unlizenzierten
			// Zeitraum wäre wertlos. Nur das Lesen ist kostenpflichtig.
			r.Group(func(r chi.Router) {
				r.Use(handlers.VerlangeFunktion(lizenz.Pruefspur))
				r.Get("/pruefspur", h.ListPruefspur)
				r.Get("/pruefspur/aktionen", h.PruefspurAktionen)
			})

			// Kontenverwaltung bleibt frei -- ohne sie ließe sich eine
			// Mehrbenutzer-Instanz gar nicht betreiben. Die /users-Routen sind
			// Admins vorbehalten, das erzwingen die Handler selbst.
			r.Get("/users", h.ListUsers)
			r.Post("/users", h.CreateUser)
			r.Delete("/users/{id}", h.DeleteUser)
			r.Put("/users/{id}/role", h.SetUserRole)

			// Spaces
			r.Get("/spaces", h.ListSpaces)
			r.Post("/spaces", h.CreateSpace)
			r.Put("/spaces/{id}", h.RenameSpace)
			r.Delete("/spaces/{id}", h.DeleteSpace)

			// Backlinks (pages linking here via [[wiki-link]] or manual links)
			// Markdown-Ausgabe einer Seite. Serverseitig, damit sie auch ohne
			// geladenen Editor funktioniert -- und weil die Umwandlung im Editor
			// ausdrücklich verlustbehaftet ist.
			r.Get("/pages/{id}/markdown", h.ExportMarkdown)

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
