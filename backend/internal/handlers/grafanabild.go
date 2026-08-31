// Das Grafana-Bild als Download aus dem Panel.
//
// Es lag als Datei im Quelltextbestand. Das half niemandem, der eine fertige
// Instanz betreibt: er hätte den Bestand auschecken müssen, um an eine Datei zu
// kommen, die seine eigene Instanz kennt. Eingebettet liegt sie im Abbild und
// gehört damit zur Fassung, die tatsächlich läuft — ein Bild, das Kennzahlen
// abfragt, die es in dieser Fassung noch gar nicht gibt, wäre schlimmer als
// keines.
package handlers

import (
	_ "embed"
	"net/http"

	"nexora/internal/middleware"
)

//go:embed bilder/grafana.json
var grafanaBild []byte

func (s *Server) GrafanaBild(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="nexora-grafana.json"`)
	_, _ = w.Write(grafanaBild)
}
