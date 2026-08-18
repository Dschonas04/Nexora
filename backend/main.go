// Command nexora is the API server: it opens the database, applies the schema
// migration and serves the JSON API under /api.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"nexora/internal/db"
	"nexora/internal/handlers"
	"nexora/internal/middleware"
)

// env reads an environment variable and falls back to def when it is unset or
// empty. Every setting has a working default so the binary starts without any
// configuration, which is convenient for local runs but means a missing .env in
// production silently uses the weak defaults below.
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	dbURL := env("DATABASE_URL", "postgres://nexora:nexora@localhost:5432/nexora?sslmode=disable")
	secret := env("JWT_SECRET", "change-me-in-production")
	port := env("PORT", "8080")
	dataDir := env("NEXORA_DATA_DIR", "/data/attachments") // attachments live on disk, not in the database

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

	h := &handlers.Server{Pool: pool, Secret: []byte(secret), DataDir: dataDir}

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
		r.Get("/public/{token}", h.GetPublicPage)

		// Everything below requires a valid session cookie. Ownership and sharing
		// are checked per request inside the handlers, not here.
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth([]byte(secret)))

			r.Get("/auth/me", h.Me)

			// The static /pages/... routes must be registered before /pages/{id},
			// otherwise chi would match "shared" and "trash" as page ids.
			r.Get("/pages", h.ListPages)
			r.Get("/pages/shared", h.ListSharedPages)
			r.Get("/pages/trash", h.ListTrash)
			r.Post("/pages", h.CreatePage)
			r.Get("/pages/{id}", h.GetPage)
			r.Put("/pages/{id}", h.UpdatePage)
			r.Delete("/pages/{id}", h.DeletePage) // moves to the trash
			r.Post("/pages/{id}/restore", h.RestorePage)
			r.Delete("/pages/{id}/purge", h.PurgePage) // deletes for good, cascades to subpages
			r.Post("/pages/{id}/favorite", h.AddFavorite)
			r.Delete("/pages/{id}/favorite", h.RemoveFavorite)
			r.Post("/pages/{id}/share", h.SharePage)
			r.Delete("/pages/{id}/share", h.UnsharePage)
			r.Post("/pages/{id}/tags", h.AttachTag)
			r.Delete("/pages/{id}/tags/{tagId}", h.DetachTag)

			// Version history
			r.Get("/pages/{id}/versions", h.ListVersions)
			r.Get("/pages/{id}/versions/{versionId}", h.GetVersion)
			r.Post("/pages/{id}/versions/{versionId}/restore", h.RestoreVersion)

			// Attachments
			r.Get("/pages/{id}/attachments", h.ListAttachments)
			r.Post("/pages/{id}/attachments", h.UploadAttachment)
			r.Get("/pages/{id}/attachments/{attId}", h.DownloadAttachment)
			r.Delete("/pages/{id}/attachments/{attId}", h.DeleteAttachment)

			// Per-user sharing plus account administration. The /users routes are
			// admin-only, which the handlers enforce.
			r.Get("/pages/{id}/shares", h.ListShares)
			r.Post("/pages/{id}/shares", h.AddShare)
			r.Delete("/pages/{id}/shares/{userId}", h.RemoveShare)
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
