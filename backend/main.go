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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("db migrate: %v", err)
	}
	log.Println("database ready")

	h := &handlers.Server{Pool: pool, Secret: []byte(secret)}

	r := chi.NewRouter()
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	r.Route("/api", func(r chi.Router) {
		// Public
		r.Post("/auth/register", h.Register)
		r.Post("/auth/login", h.Login)
		r.Post("/auth/logout", h.Logout)
		r.Get("/public/{token}", h.GetPublicPage)

		// Authenticated
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth([]byte(secret)))

			r.Get("/auth/me", h.Me)

			r.Get("/pages", h.ListPages)
			r.Post("/pages", h.CreatePage)
			r.Get("/pages/{id}", h.GetPage)
			r.Put("/pages/{id}", h.UpdatePage)
			r.Delete("/pages/{id}", h.DeletePage)
			r.Post("/pages/{id}/favorite", h.AddFavorite)
			r.Delete("/pages/{id}/favorite", h.RemoveFavorite)
			r.Post("/pages/{id}/share", h.SharePage)
			r.Delete("/pages/{id}/share", h.UnsharePage)
			r.Post("/pages/{id}/tags", h.AttachTag)
			r.Delete("/pages/{id}/tags/{tagId}", h.DetachTag)

			r.Get("/favorites", h.ListFavorites)
			r.Get("/tags", h.ListTags)
			r.Post("/tags", h.CreateTag)
			r.Delete("/tags/{id}", h.DeleteTag)
			r.Get("/search", h.Search)
		})
	})

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
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("nexora backend listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
