// Importing and issuing license keys, both from the administration pages.
//
// Importing works on any installation: the key is verified, stored in the
// database and takes effect at once. Without that, renewing a license would
// mean editing the configuration file and restarting the service.
//
// Issuing works only where the private key is present. That is the licensor and
// nobody else: verification happens offline, so the private key is the only
// thing separating "has paid" from "has written a key". It comes from the
// environment and lives nowhere in the repository.
package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nexora/internal/lizenz"
	"nexora/internal/middleware"
)

// lizenzSchluessel is the name the imported key is stored under. Deliberately
// not part of the ordinary settings list: it does not belong in a form next to
// colours and deadlines, and it must not turn up in an overview by accident.
const lizenzSchluessel = "lizenz"

type lizenzEinlesenReq struct {
	Schluessel string `json:"schluessel"`
}

// LizenzEinlesen takes a key, verifies it and puts it into effect.
func (s *Server) LizenzEinlesen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	var req lizenzEinlesenReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	schluessel := strings.TrimSpace(req.Schluessel)

	// An empty key withdraws the license. That is not an oversight, it is the
	// way back to the free feature set.
	if schluessel == "" {
		if _, err := s.Pool.Exec(r.Context(),
			`DELETE FROM einstellungen WHERE schluessel=$1`, lizenzSchluessel); err != nil {
			writeErr(w, http.StatusInternalServerError, "konnte nicht gespeichert werden")
			return
		}
		lizenz.Laden("")
		s.spurAusRequest(r, AktLizenzGeladen, "system", "", "", map[string]any{"aktion": "entfernt"})
		writeJSON(w, http.StatusOK, lizenz.Aktuell())
		return
	}

	// Verify first, store second. An invalid key in the database would mean the
	// service reports an error on its next start that nobody connects with this
	// click any more.
	z, err := lizenz.Pruefe(schluessel)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := s.Pool.Exec(r.Context(),
		`INSERT INTO einstellungen (schluessel, wert, geaendert_von)
		 VALUES ($1, $2, (SELECT name FROM users WHERE id=$3))
		 ON CONFLICT (schluessel) DO UPDATE SET wert=EXCLUDED.wert,
		   geaendert_am=now(), geaendert_von=EXCLUDED.geaendert_von`,
		lizenzSchluessel, schluessel, uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht gespeichert werden")
		return
	}

	lizenz.Laden(schluessel)
	s.spurAusRequest(r, AktLizenzGeladen, "system", "", z.Inhaber,
		map[string]any{"stufe": string(z.Stufe), "laeuft_ab": z.LaeuftAb})
	writeJSON(w, http.StatusOK, lizenz.Aktuell())
}

type lizenzAusstellenReq struct {
	Inhaber    string   `json:"inhaber"`
	Stufe      string   `json:"stufe"`
	Funktionen []string `json:"funktionen"` // single extras on top of the tier
	Ablauf     string   `json:"ablauf"`     // YYYY-MM-DD, empty means one year
}

// LizenzAusstellen signs a new key.
func (s *Server) LizenzAusstellen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	if !lizenz.Ausstellbar() {
		writeErr(w, http.StatusNotImplemented,
			"diese Installation kann keine Schlüssel ausstellen. Dafür braucht es den privaten Signierschlüssel.")
		return
	}
	var req lizenzAusstellenReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	var zusatz []lizenz.Funktion
	for _, n := range req.Funktionen {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		bekannt := false
		for _, f := range lizenz.Alle {
			if lizenz.Funktion(n) == f {
				bekannt = true
				zusatz = append(zusatz, f)
			}
		}
		if !bekannt {
			writeErr(w, http.StatusBadRequest, "unbekannte Funktion: "+n)
			return
		}
	}

	var ablauf time.Time
	if strings.TrimSpace(req.Ablauf) != "" {
		var err error
		ablauf, err = time.Parse("2006-01-02", strings.TrimSpace(req.Ablauf))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Ablaufdatum ist nicht JJJJ-MM-TT")
			return
		}
	}

	schluessel, err := lizenz.Ausstellen(req.Inhaber, lizenz.Stufe(strings.TrimSpace(req.Stufe)),
		zusatz, ablauf)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// The audit trail records holder and tier of the issued key, but never the
	// key itself: whoever may read the trail should not be able to collect
	// other people's licenses along the way.
	s.spurAusRequest(r, AktLizenzAusgestellt, "system", "", req.Inhaber,
		map[string]any{"stufe": req.Stufe, "ablauf": req.Ablauf})

	writeJSON(w, http.StatusOK, map[string]string{"schluessel": schluessel})
}

// LizenzAusDatenbank fetches an imported key at startup. It takes precedence
// over the configuration file: whatever was imported through the administration
// pages last is the more recent intent.
//
// Errors are swallowed and treated as "no key". At startup the table may have
// been created moments ago, and a license is no reason to keep the service from
// coming up.
func LizenzAusDatenbank(ctx context.Context, pool *pgxpool.Pool) string {
	var wert string
	if err := pool.QueryRow(ctx,
		`SELECT wert FROM einstellungen WHERE schluessel=$1`, lizenzSchluessel).Scan(&wert); err != nil {
		return ""
	}
	return strings.TrimSpace(wert)
}
