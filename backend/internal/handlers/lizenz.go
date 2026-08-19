// Paid extras are gated here. The rule is deliberately server-side: the browser
// hides locked features to keep the interface honest, but a request that goes
// around the interface has to be refused here, not there.
package handlers

import (
	"net/http"

	"nexora/internal/lizenz"
)

// VerlangeFunktion wraps the routes of one paid extra. Without a valid key the
// request ends in 402 -- the status that exists for exactly this case and that
// the browser can tell apart from "not allowed" (403).
func VerlangeFunktion(f lizenz.Funktion) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !lizenz.Frei(f) {
				z := lizenz.Aktuell()
				grund := z.Grund
				if grund == "" {
					grund = "diese Funktion ist in der vorliegenden Lizenz nicht enthalten"
				}
				writeJSON(w, http.StatusPaymentRequired, map[string]interface{}{
					"error":    "Funktion nicht freigeschaltet",
					"funktion": string(f),
					"grund":    grund,
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// LizenzStatus tells the browser what is unlocked, so the interface can hide
// what would only end in 402 anyway. Readable by every signed-in account: it
// contains no secret, and hiding it would only make the interface lie.
func (s *Server) LizenzStatus(w http.ResponseWriter, r *http.Request) {
	z := lizenz.Aktuell()

	// The full list travels along so the browser does not have to carry its own
	// copy of what extras exist -- that copy would drift.
	alle := make([]string, 0, len(lizenz.Alle))
	for _, f := range lizenz.Alle {
		alle = append(alle, string(f))
	}
	frei := make([]string, 0, len(lizenz.Alle))
	for _, f := range lizenz.Alle {
		if lizenz.Frei(f) {
			frei = append(frei, string(f))
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"gueltig":        z.Gueltig,
		"inhaber":        z.Inhaber,
		"laeuft_ab":      z.LaeuftAb,
		"grund":          z.Grund,
		"alle_extras":    alle,
		"freigeschaltet": frei,
	})
}
