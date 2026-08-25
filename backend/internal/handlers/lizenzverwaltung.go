// Lizenzschlüssel einlesen und ausstellen, beides aus der Verwaltung heraus.
//
// Einlesen kann jede Installation: der Schlüssel wird geprüft, in der Datenbank
// abgelegt und sofort wirksam. Ohne das müsste man an die Konfigurationsdatei
// und den Dienst neu starten, um eine verlängerte Lizenz einzuspielen.
//
// Ausstellen kann nur, wer den privaten Schlüssel hat. Das ist der Herausgeber
// und sonst niemand: geprüft wird offline, also ist der private Schlüssel die
// einzige Grenze zwischen "hat bezahlt" und "hat sich einen Schlüssel gebaut".
// Er kommt aus der Umgebung und steht nirgends im Verzeichnis.
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

// lizenzSchluessel ist der Name, unter dem der eingelesene Schlüssel liegt.
// Bewusst nicht in der Liste der gewöhnlichen Einstellungen: er gehört nicht in
// eine Maske neben Farben und Fristen, und er soll nicht versehentlich in einer
// Übersicht auftauchen.
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

	// Ein leerer Schlüssel nimmt die Lizenz zurück. Das ist kein Versehen,
	// sondern der Weg zurück auf den freien Umfang.
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

	// Erst prüfen, dann speichern. Ein ungültiger Schlüssel in der Datenbank
	// hieße: beim nächsten Start meldet der Dienst einen Fehler, den niemand
	// mehr mit diesem Klick in Verbindung bringt.
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

// LizenzAusstellen signiert einen neuen Schlüssel.
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

	// Der ausgestellte Schlüssel steht in der Prüfspur mit Inhaber und Stufe,
	// aber ohne den Schlüssel selbst: wer die Spur lesen darf, soll nicht
	// nebenbei fremde Lizenzen einsammeln können.
	s.spurAusRequest(r, AktLizenzAusgestellt, "system", "", req.Inhaber,
		map[string]any{"stufe": req.Stufe, "ablauf": req.Ablauf})

	writeJSON(w, http.StatusOK, map[string]string{"schluessel": schluessel})
}

// LizenzAusDatenbank holt einen eingelesenen Schlüssel beim Start. Er hat
// Vorrang vor der Konfigurationsdatei: was zuletzt über die Verwaltung
// eingespielt wurde, ist der jüngere Wille.
//
// Fehler werden verschluckt und als "kein Schlüssel" behandelt. Beim Start ist
// die Tabelle unter Umständen gerade erst angelegt worden, und eine Lizenz ist
// kein Grund, den Dienst nicht hochkommen zu lassen.
func LizenzAusDatenbank(ctx context.Context, pool *pgxpool.Pool) string {
	var wert string
	if err := pool.QueryRow(ctx,
		`SELECT wert FROM einstellungen WHERE schluessel=$1`, lizenzSchluessel).Scan(&wert); err != nil {
		return ""
	}
	return strings.TrimSpace(wert)
}
