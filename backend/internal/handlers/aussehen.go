package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"nexora/internal/middleware"
)

// Das Aussehen gehoert dem Konto.
//
// Grundton und Akzent standen als design_grundton und design_akzent in der
// Einstellungstabelle, also einmal fuer die ganze Instanz und nur fuer
// Administratoren aenderbar. Das war die falsche Ebene: welche Farbe jemand
// ertraegt und ob er hell oder dunkel arbeitet, geht niemanden sonst etwas an,
// und wer nachts dunkel stellte, stellte alle anderen mit um.
//
// Jetzt liegen beide Werte in der Zeile des Kontos. Leer heisst "nichts
// gewaehlt": dann gilt die Vorgabe, und zwar dieselbe, die auch der
// Abgemeldete auf der Anmeldeseite sieht.
const (
	grundtonVorgabe = "grau"
	akzentVorgabe   = "#2383e2"
)

type aussehenAntwort struct {
	Grundton string `json:"grundton"`
	Akzent   string `json:"akzent"`
}

// aussehenLesen holt die Wahl des Kontos und ergaenzt fehlende Werte durch die
// Vorgabe. Ein Fehler beim Lesen ist keiner, der die Oberflaeche aufhalten
// darf: dann sieht das Konto eben die Vorgabe.
func (s *Server) aussehenLesen(r *http.Request) aussehenAntwort {
	a := aussehenAntwort{Grundton: grundtonVorgabe, Akzent: akzentVorgabe}
	uid := middleware.UserID(r)
	if uid == "" {
		return a
	}
	var grundton, akzent string
	if s.Pool.QueryRow(r.Context(),
		`SELECT design_grundton, design_akzent FROM users WHERE id=$1`, uid).
		Scan(&grundton, &akzent) != nil {
		return a
	}
	if grundtoene[grundton] {
		a.Grundton = grundton
	}
	if istHexFarbe(akzent) {
		a.Akzent = akzent
	}
	return a
}

// AussehenSpeichern nimmt die Wahl des angemeldeten Kontos entgegen.
//
// Geprueft wird beides, obwohl die Oberflaeche nur gueltige Werte schickt: der
// Akzent landet unveraendert in einer CSS-Variablen, und eine Zeichenkette mit
// Klammern waere ein Weg hinein.
func (s *Server) AussehenSpeichern(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Grundton string `json:"grundton"`
		Akzent   string `json:"akzent"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeErr(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	req.Grundton = strings.TrimSpace(req.Grundton)
	req.Akzent = strings.ToLower(strings.TrimSpace(req.Akzent))

	// Leer ist erlaubt und heisst "zurueck auf die Vorgabe".
	if req.Grundton != "" && !grundtoene[req.Grundton] {
		writeErr(w, http.StatusBadRequest, "erwartet weiss, grau oder dunkel")
		return
	}
	if req.Akzent != "" && !istHexFarbe(req.Akzent) {
		writeErr(w, http.StatusBadRequest, "erwartet eine Farbe wie #2383e2")
		return
	}

	uid := middleware.UserID(r)
	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE users SET design_grundton=$2, design_akzent=$3 WHERE id=$1`,
		uid, req.Grundton, req.Akzent); err != nil {
		writeErr(w, http.StatusInternalServerError, "nicht gespeichert")
		return
	}
	writeJSON(w, http.StatusOK, s.aussehenLesen(r))
}
