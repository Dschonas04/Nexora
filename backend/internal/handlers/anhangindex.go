package handlers

import (
	"context"
	"io"
	"log"
	"net/http"

	"nexora/internal/middleware"
)

// AnhangIndexNachziehen reads every attachment that has no extracted text yet
// and fills it in.
//
// Needed because attachments uploaded before this existed carry nothing, and a
// search that silently misses every older file looks like an empty result
// rather than a missing index, the same trap as with the page index.
//
// Deliberately an explicit action rather than something that runs at startup:
// it reads every file back out of storage, which over an object store means
// pulling them across the network. That is a decision an administrator should
// take, not a surprise on boot.
func (s *Server) AnhangIndexNachziehen(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, filename, mime FROM attachments
		 WHERE length(trim(inhalt_text)) = 0
		 ORDER BY created_at DESC
		 LIMIT 500`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}

	type anhang struct{ id, name, mime string }
	var offen []anhang
	for rows.Next() {
		var a anhang
		if rows.Scan(&a.id, &a.name, &a.mime) == nil {
			offen = append(offen, a)
		}
	}
	rows.Close()

	gelesen, leer := 0, 0
	for _, a := range offen {
		roh, err := s.lesePlusGrenze(r.Context(), a.id)
		if err != nil {
			log.Printf("Anhang-Volltext nachziehen (%s): %v", a.id, err)
			continue
		}
		txt := textAusAnhang(r.Context(), roh, a.mime, a.name)
		if txt == "" {
			// Ein Leerzeichen statt nichts: sonst läse dieselbe Abfrage die
			// Datei beim nächsten Lauf wieder, obwohl sie nichts hergibt.
			txt = " "
			leer++
		} else {
			gelesen++
		}
		if _, err := s.Pool.Exec(r.Context(),
			`UPDATE attachments SET inhalt_text=$2 WHERE id=$1`, a.id, txt); err != nil {
			log.Printf("Anhang-Volltext schreiben (%s): %v", a.id, err)
		}
	}

	s.spurAusRequest(r, AktAnhangIndex, "system", "anhangindex", "",
		map[string]any{"gelesen": gelesen, "ohneText": leer})
	writeJSON(w, http.StatusOK, map[string]int{
		"betrachtet": len(offen),
		"gelesen":    gelesen,
		"ohneText":   leer,
	})
}

// lesePlusGrenze holt höchstens so viel, wie in den Index passt. Eine
// 200-MB-Datei komplett zu laden, um 400 KB davon zu behalten, wäre Verschwendung.
func (s *Server) lesePlusGrenze(ctx context.Context, key string) ([]byte, error) {
	f, err := s.Ablage.Lesen(ctx, key)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxAnhangText))
}
