// Personal API keys.
//
// A script, n8n or a shell reaches the API with
//
//	Authorization: Bearer nxr_<random>
//
// instead of the session cookie. A key acts as the account that made it and
// can do no more than that account; a key with the right 'lesen' only reads.
// Before this the only way in from outside was a token signed by hand with the
// instance secret -- which works, and means handing out the secret.
//
// Only the SHA-256 of a key is stored. It is shown once and never again.
package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

const schluesselVorsatz = "nxr_"

// APISchluessel is one key as the account settings list it.
type APISchluessel struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Anfang     string     `json:"hint"`
	Recht      string     `json:"scope"` // "lesen" | "schreiben"
	ErstelltAm time.Time  `json:"createdAt"`
	ZuletztAm  *time.Time `json:"lastUsedAt"`
	LaeuftAb   *time.Time `json:"expiresAt"`
	// Schluessel carries the key itself, only in the answer that made it.
	Schluessel string `json:"key,omitempty"`
}

func schluesselHash(k string) string {
	h := sha256.Sum256([]byte(k))
	return hex.EncodeToString(h[:])
}

// SchluesselPruefen is handed to the auth middleware: it answers who a key
// belongs to and whether it may write. The last use is noted at most once a
// minute, so a busy script does not turn every request into a write.
func (s *Server) SchluesselPruefen(ctx context.Context, schluessel string) (uid string, schreiben bool, ok bool) {
	if !strings.HasPrefix(schluessel, schluesselVorsatz) {
		return "", false, false
	}
	var id, recht string
	err := s.Pool.QueryRow(ctx,
		`SELECT id::text, user_id::text, recht FROM api_schluessel
		 WHERE hash = $1 AND (laeuft_ab IS NULL OR laeuft_ab > now())`,
		schluesselHash(schluessel)).Scan(&id, &uid, &recht)
	if err != nil {
		return "", false, false
	}
	_, _ = s.Pool.Exec(ctx,
		`UPDATE api_schluessel SET zuletzt_am = now()
		 WHERE id = $1 AND (zuletzt_am IS NULL OR zuletzt_am < now() - interval '1 minute')`, id)
	return uid, recht == "schreiben", true
}

// ListSchluessel returns the caller's keys, without the keys.
func (s *Server) ListSchluessel(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, name, anfang, recht, erstellt_am, zuletzt_am, laeuft_ab
		 FROM api_schluessel WHERE user_id=$1 ORDER BY erstellt_am DESC`, middleware.UserID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	liste := []APISchluessel{}
	for rows.Next() {
		var k APISchluessel
		if rows.Scan(&k.ID, &k.Name, &k.Anfang, &k.Recht, &k.ErstelltAm, &k.ZuletztAm, &k.LaeuftAb) == nil {
			liste = append(liste, k)
		}
	}
	writeJSON(w, http.StatusOK, liste)
}

// SchluesselAnlegen makes a key. Days 0 means it does not expire.
func (s *Server) SchluesselAnlegen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req struct {
		Name  string `json:"name"`
		Recht string `json:"scope"`
		Tage  int    `json:"days"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 80 {
		writeErr(w, http.StatusBadRequest, "Der Schlüssel braucht einen Namen.")
		return
	}
	// Anything unknown becomes the weaker right, so a typo cannot open more.
	if req.Recht != "schreiben" {
		req.Recht = "lesen"
	}
	var anzahl int
	_ = s.Pool.QueryRow(r.Context(), `SELECT count(*) FROM api_schluessel WHERE user_id=$1`, uid).Scan(&anzahl)
	if anzahl >= 50 {
		writeErr(w, http.StatusBadRequest, "Zu viele Schlüssel.")
		return
	}
	var ablauf *time.Time
	if req.Tage > 0 {
		t := time.Now().AddDate(0, 0, req.Tage)
		ablauf = &t
	}
	roh := make([]byte, 24)
	if _, err := rand.Read(roh); err != nil {
		writeErr(w, http.StatusInternalServerError, "no randomness")
		return
	}
	schluessel := schluesselVorsatz + hex.EncodeToString(roh)
	k := APISchluessel{Schluessel: schluessel}
	err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO api_schluessel (user_id, name, hash, anfang, recht, laeuft_ab)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, name, anfang, recht, erstellt_am, laeuft_ab`,
		uid, req.Name, schluesselHash(schluessel), schluessel[:10]+"…", req.Recht, ablauf).
		Scan(&k.ID, &k.Name, &k.Anfang, &k.Recht, &k.ErstelltAm, &k.LaeuftAb)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Schlüssel konnte nicht angelegt werden")
		return
	}
	s.spurAusRequest(r, AktSchluesselNeu, "apischluessel", k.ID, k.Name, map[string]interface{}{"scope": k.Recht})
	writeJSON(w, http.StatusCreated, k)
}

// SchluesselWiderrufen deletes a key; it stops working with the next request.
func (s *Server) SchluesselWiderrufen(w http.ResponseWriter, r *http.Request) {
	var name string
	err := s.Pool.QueryRow(r.Context(),
		`DELETE FROM api_schluessel WHERE id=$1 AND user_id=$2 RETURNING name`,
		chi.URLParam(r, "id"), middleware.UserID(r)).Scan(&name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Schlüssel nicht gefunden")
		return
	}
	s.spurAusRequest(r, AktSchluesselWeg, "apischluessel", chi.URLParam(r, "id"), name, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
