// Die Verwaltung der Kennzahlen: an- und ausschalten, und nachsehen, ob es
// ankommt.
//
// Das Losungswort ließe sich auch über den gewöhnlichen Weg der Einstellungen
// setzen, es steht in derselben Karte wie die übrigen. Ein eigener Abschnitt
// lohnt trotzdem, weil das Einschalten allein nichts nützt: es gehört ein
// Abschnitt in die prometheus.yml, und der enthält genau dieses Wort. Wer es
// hier erzeugt, soll den fertigen Abschnitt daneben stehen haben und ihn
// kopieren können, statt ihn aus einer Anleitung abzuschreiben.
//
// Dazu die Gegenprobe: wann wurde zuletzt abgeholt. Ohne die weiß niemand, ob
// die Verdrahtung sitzt, und man sucht den Fehler abwechselnd auf beiden
// Seiten.
package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"nexora/internal/middleware"
)

// Wann zuletzt jemand die Kennzahlen abgeholt hat, als Unix-Sekunde.
//
// Eine Zahl und keine Zeit mit Sperre: geschrieben wird sie bei jedem Abholen,
// gelesen selten, und ein atomarer Wert kostet nichts. Null heißt: noch nie.
var zuletztAbgeholt atomic.Int64

// abholungen zählt mit, damit sich ein einmaliger Versuch von einem laufenden
// Sammler unterscheiden lässt.
var abholungen atomic.Int64

func metrikenAbgeholt() {
	zuletztAbgeholt.Store(time.Now().Unix())
	abholungen.Add(1)
}

// MetrikenZustand sagt, ob die Kennzahlen an sind, und liefert den fertigen
// Abschnitt für prometheus.yml mit.
func (s *Server) MetrikenZustand(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	writeJSON(w, http.StatusOK, s.metrikenAntwort())
}

func (s *Server) metrikenAntwort() map[string]interface{} {
	token := MetrikenToken()
	antwort := map[string]interface{}{
		"aktiv":       token != "",
		"token":       token,
		"abholungen":  abholungen.Load(),
		"ausDerDatei": speicherBasisToken() != "" && speicherWertFehlt("metriken_token"),
	}
	if t := zuletztAbgeholt.Load(); t > 0 {
		antwort["zuletztAbgeholt"] = time.Unix(t, 0)
		antwort["vorSekunden"] = time.Now().Unix() - t
	}

	// Der fertige Abschnitt. Die Adresse kann nur der Betreiber wissen, deshalb
	// steht die öffentliche Adresse darin, wenn eine gesetzt ist, und sonst ein
	// Platzhalter, der als solcher zu erkennen ist.
	ziel := "NEXORA-HOST:3000"
	if u := speicherOeffentlicheURL(); u != "" {
		ziel = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://"), "/")
	}
	wort := token
	if wort == "" {
		wort = "<erst ein Losungswort erzeugen>"
	}
	antwort["prometheus"] = fmt.Sprintf(`  - job_name: 'nexora'
    metrics_path: /metrics
    scrape_interval: 15s
    authorization:
      credentials: '%s'
    static_configs:
      - targets: ['%s']
        labels:
          instance_name: 'nexora'`, wort, ziel)
	return antwort
}

// MetrikenTokenNeu erzeugt ein Losungswort und legt es ab.
//
// Erzeugt und nicht eingetippt: ein Wort, das sich jemand ausdenkt, ist kurz
// und wiederverwendet. Vierundzwanzig Byte aus der Zufallsquelle des Systems
// sind lang genug, dass Raten ausscheidet, und niemand muss sie sich merken,
// weil sie in die prometheus.yml kopiert werden.
func (s *Server) MetrikenTokenNeu(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	roh := make([]byte, 24)
	if _, err := rand.Read(roh); err != nil {
		writeErr(w, http.StatusInternalServerError, "Zufallsquelle nicht verfügbar")
		return
	}
	neu := hex.EncodeToString(roh)

	if err := s.einstellungSchreiben(r.Context(), "metriken_token", neu); err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht gespeichert werden")
		return
	}
	// Das Wort selbst steht NICHT in der Prüfspur. Dass es gewechselt wurde,
	// gehört hinein; was es ist, wäre ein Geheimnis in einem Protokoll, das
	// mehr Leute lesen als die Einstellungen.
	s.spurAusRequest(r, AktEinstellung, "einstellung", "metriken_token", "Kennzahlen",
		map[string]interface{}{"aktion": "Losungswort erzeugt"})

	writeJSON(w, http.StatusOK, s.metrikenAntwort())
}

// MetrikenAus nimmt das Losungswort zurück. Danach gibt es den Weg nicht mehr.
func (s *Server) MetrikenAus(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	if err := s.einstellungSchreiben(r.Context(), "metriken_token", ""); err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht gespeichert werden")
		return
	}
	s.spurAusRequest(r, AktEinstellung, "einstellung", "metriken_token", "Kennzahlen",
		map[string]interface{}{"aktion": "abgeschaltet"})
	writeJSON(w, http.StatusOK, s.metrikenAntwort())
}
