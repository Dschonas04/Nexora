// Binding an object store from the settings page.
//
// The credentials do not live in the settings table with everything else. A
// secret access key belongs in the environment or in config.conf, not in a row
// that a database dump carries off — the same reason the license key cannot be
// set here either.
//
// What this file does provide is the part that genuinely needs a browser:
// checking whether a given endpoint actually answers, before someone writes it
// into a config file and restarts a running instance on a guess.
package handlers

import (
	"context"
	"net/http"
	"time"

	"nexora/internal/ablage"
	"nexora/internal/middleware"
)

type s3TestReq struct {
	Endpunkt  string `json:"endpunkt"`
	Bucket    string `json:"bucket"`
	Zugriff   string `json:"zugriff"`
	Geheimnis string `json:"geheimnis"`
	Region    string `json:"region"`
	TLS       bool   `json:"tls"`
	Pfadstil  bool   `json:"pfadstil"`
}

// S3Testen tries a connection and writes a tiny object as a test.
//
// Merely connecting would check too little: the most common faults, a wrong key,
// a missing write permission, a bucket in another region, only show up on the
// first write. The test object is removed again right away.
func (s *Server) S3Testen(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	var req s3TestReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Endpunkt == "" {
		writeErr(w, http.StatusBadRequest, "kein Endpunkt angegeben")
		return
	}
	if req.Bucket == "" {
		req.Bucket = "nexora"
	}
	if req.Region == "" {
		req.Region = "us-east-1"
	}

	// A deadline of its own: an endpoint pointing nowhere would otherwise leave
	// the call hanging until the request times out, and whoever is testing sees a
	// spinning button for minutes.
	ctx, abbrechen := context.WithTimeout(r.Context(), 12*time.Second)
	defer abbrechen()

	a, err := ablage.NeuS3(ctx, ablage.Einstellungen{
		Endpunkt:  req.Endpunkt,
		Bucket:    req.Bucket,
		Zugriff:   req.Zugriff,
		Geheimnis: req.Geheimnis,
		Region:    req.Region,
		TLS:       req.TLS,
		Pfadstil:  req.Pfadstil,
	})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"schritt": "verbinden",
			"grund":   err.Error(),
		})
		return
	}

	schluessel := "nexora-verbindungstest"
	inhalt := "Verbindungstest von Nexora. Dieses Objekt darf gelöscht werden."
	if _, err := a.Schreiben(ctx, schluessel, stringLeser(inhalt), int64(len(inhalt)), "text/plain"); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"schritt": "schreiben",
			"grund":   err.Error(),
		})
		return
	}
	if _, err := a.Lesen(ctx, schluessel); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"schritt": "lesen",
			"grund":   err.Error(),
		})
		return
	}
	if err := a.Loeschen(ctx, schluessel); err != nil {
		// Writing and reading worked, that is enough to run on. That the cleanup
		// fails is a note, not a failure.
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"ablage":    a.Name(),
			"anmerkung": "Testobjekt konnte nicht gelöscht werden: " + err.Error(),
		})
		return
	}

	s.spurAusRequest(r, AktS3Test, "system", "objektspeicher", req.Endpunkt, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ablage": a.Name()})
}

// AblageZustand says where attachments currently live.
func (s *Server) AblageZustand(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ablage": s.Ablage.Name()})
}
