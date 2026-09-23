package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"nexora/internal/middleware"
)

// The appearance belongs to the account.
//
// Base tone and accent used to sit in the settings table as design_grundton and
// design_akzent, that is once for the whole instance and changeable only by
// administrators. That was the wrong level: which colour somebody can bear and
// whether they work light or dark is nobody else's business, and whoever
// switched to dark at night switched everybody else over too.
//
// Now both values live in the account's row. Empty means "nothing chosen": then
// the default applies, and it is the same one the signed-out visitor sees on
// the login page.
const (
	grundtonVorgabe = "grau"
	akzentVorgabe   = "#1b6ec2"
)

type aussehenAntwort struct {
	Grundton string `json:"grundton"`
	Akzent   string `json:"akzent"`
	// Sprache is the interface language, "de" or "en". Empty: nothing chosen,
	// the browser decides.
	Sprache string `json:"sprache"`
}

// sprachen are the interface languages there is a word list for.
var sprachen = map[string]bool{"de": true, "en": true}

// aussehenLesen fetches the account's choice and fills missing values from the
// default. A read error is not one that may hold up the interface: the account
// simply sees the default then.
func (s *Server) aussehenLesen(r *http.Request) aussehenAntwort {
	a := aussehenAntwort{Grundton: grundtonVorgabe, Akzent: akzentVorgabe}
	uid := middleware.UserID(r)
	if uid == "" {
		return a
	}
	var grundton, akzent, sprache string
	if s.Pool.QueryRow(r.Context(),
		`SELECT design_grundton, design_akzent, sprache FROM users WHERE id=$1`, uid).
		Scan(&grundton, &akzent, &sprache) != nil {
		return a
	}
	if grundtoene[grundton] {
		a.Grundton = grundton
	}
	if istHexFarbe(akzent) {
		a.Akzent = akzent
	}
	if sprachen[sprache] {
		a.Sprache = sprache
	}
	return a
}

// AussehenSpeichern takes in the signed-in account's choice.
//
// Both are checked although the interface only ever sends valid values: the
// accent ends up unchanged in a CSS variable, and a string with braces in it
// would be a way inside.
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

	// Empty is allowed and means "back to the default".
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

// SpracheSpeichern stores the interface language on the signed-in account. A
// route of its own and not part of AussehenSpeichern: switching the language
// must not send base tone and accent along, or a second tab with an older
// choice would write it back.
func (s *Server) SpracheSpeichern(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sprache string `json:"sprache"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeErr(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	req.Sprache = strings.TrimSpace(req.Sprache)
	if req.Sprache != "" && !sprachen[req.Sprache] {
		writeErr(w, http.StatusBadRequest, "erwartet de oder en")
		return
	}
	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE users SET sprache=$2 WHERE id=$1`, middleware.UserID(r), req.Sprache); err != nil {
		writeErr(w, http.StatusInternalServerError, "nicht gespeichert")
		return
	}
	writeJSON(w, http.StatusOK, s.aussehenLesen(r))
}
