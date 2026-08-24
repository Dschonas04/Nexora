// Gespeicherte Sitzungen.
//
// Vorher war eine Sitzung nichts als ein unterschriebenes Token: gültig, bis
// die Zeit ablief, und durch nichts aufzuhalten. Abmelden löschte nur das
// Plätzchen im Browser -- wer das Token vorher kopiert hatte, blieb angemeldet.
// Ein verlorenes Notebook konnte man nicht aussperren, ohne allen anderen den
// Zugang zu nehmen.
//
// Jetzt steht jede Sitzung als Zeile in der Datenbank, und das Token verweist
// nur darauf. Damit lässt sich eine einzelne beenden, und die Liste beantwortet
// die Frage, wer gerade angemeldet ist.
//
// Der Preis ist eine Abfrage je Anfrage. Sie ist billig -- ein Zugriff über den
// Primärschlüssel -- und wird zusätzlich zwischengespeichert, siehe
// sitzungsspeicher.go.
package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/auth"
	"nexora/internal/middleware"
)

// sitzungsTakt ist der Abstand zwischen zwei Aufräumdurchgängen.
const sitzungsTakt = 6 * time.Hour

// aufgefrischtAb: erst wenn eine Sitzung so weit fortgeschritten ist, wird sie
// verlängert. Bei jeder Anfrage zu verlängern hieße, bei jeder Anfrage zu
// schreiben -- viel Last für nichts.
const aufgefrischtAb = 0.5

// benutztSpanne begrenzt, wie oft "zuletzt benutzt" nachgeführt wird. Ohne das
// schriebe jede einzelne Anfrage in die Zeile.
const benutztSpanne = 5 * time.Minute

// Sitzung ist eine Zeile, wie die Oberfläche sie zeigt.
type Sitzung struct {
	ID         string    `json:"id"`
	AngelegtAm time.Time `json:"angelegtAm"`
	ZuletztAm  time.Time `json:"zuletztAm"`
	LaeuftAb   time.Time `json:"laeuftAb"`
	IP         string    `json:"ip"`
	Browser    string    `json:"browser"`
	// Wahr für die Sitzung, mit der gerade gefragt wird -- damit die Liste
	// sagen kann "dieses Gerät" und nicht versehentlich das eigene beendet.
	Diese bool `json:"diese"`
}

// sitzungAnlegen schreibt die Zeile und gibt ihre Kennung zurück.
func (s *Server) sitzungAnlegen(ctx context.Context, r *http.Request, userID string) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO sitzungen (user_id, laeuft_ab, ip, browser)
		 VALUES ($1, now() + make_interval(days => $2), $3, $4)
		 RETURNING id`,
		userID, SitzungTage(), absenderIP(r), kurzerBrowser(r.UserAgent())).Scan(&id)
	return id, err
}

// SitzungGilt prüft eine Sitzung und frischt sie bei Bedarf auf. Diese Funktion
// hängt in der Middleware.
func (s *Server) SitzungGilt(r *http.Request, w http.ResponseWriter, uid, sid string) bool {
	// Ein Token ohne Sitzungskennung stammt aus der Zeit davor. Es gilt bis zum
	// Ablauf weiter -- alle auf einmal abzumelden wäre eine unnötige Härte für
	// eine Änderung, von der niemand etwas mitbekommen soll.
	if sid == "" {
		return true
	}

	if gilt, ok := s.sitzungAusSpeicher(sid); ok {
		if !gilt {
			return false
		}
		// Aus dem Zwischenspeicher heraus wird nicht aufgefrischt: das
		// passiert beim nächsten Durchgang durch die Datenbank.
		return true
	}

	var besitzer string
	var angelegt, laeuftAb, zuletzt time.Time
	var widerrufen *time.Time
	err := s.Pool.QueryRow(r.Context(),
		`SELECT user_id, angelegt_am, zuletzt_am, laeuft_ab, widerrufen_am
		 FROM sitzungen WHERE id=$1`, sid).
		Scan(&besitzer, &angelegt, &zuletzt, &laeuftAb, &widerrufen)
	if err != nil || besitzer != uid || widerrufen != nil || time.Now().After(laeuftAb) {
		s.sitzungMerken(sid, false)
		return false
	}

	jetzt := time.Now()

	// Auffrischen: hat die Sitzung mehr als die Hälfte ihrer Zeit hinter sich,
	// bekommt sie neue -- samt neuem Plätzchen. Wer täglich arbeitet, wird so
	// nie abgemeldet; wer wochenlang nicht kommt, schon.
	gesamt := laeuftAb.Sub(angelegt)
	if gesamt > 0 && jetzt.Sub(angelegt) > time.Duration(float64(gesamt)*aufgefrischtAb) {
		neuAb := jetzt.Add(SitzungDauer())
		if _, err := s.Pool.Exec(r.Context(),
			`UPDATE sitzungen SET laeuft_ab=$2, angelegt_am=now(), zuletzt_am=now() WHERE id=$1`,
			sid, neuAb); err == nil {
			if token, err := auth.GenerateToken(s.Secret, uid, sid, SitzungDauer()); err == nil {
				s.setAuthCookieFuer(w, r, token)
			}
		}
	} else if jetzt.Sub(zuletzt) > benutztSpanne {
		s.Pool.Exec(r.Context(), `UPDATE sitzungen SET zuletzt_am=now() WHERE id=$1`, sid)
	}

	s.sitzungMerken(sid, true)
	return true
}

// ListSitzungen zeigt die eigenen Sitzungen.
func (s *Server) ListSitzungen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	diese := middleware.SitzungID(r)

	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, angelegt_am, zuletzt_am, laeuft_ab, ip, browser
		 FROM sitzungen
		 WHERE user_id=$1 AND widerrufen_am IS NULL AND laeuft_ab > now()
		 ORDER BY zuletzt_am DESC`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	liste := []Sitzung{}
	for rows.Next() {
		var si Sitzung
		if err := rows.Scan(&si.ID, &si.AngelegtAm, &si.ZuletztAm, &si.LaeuftAb,
			&si.IP, &si.Browser); err == nil {
			si.Diese = si.ID == diese
			liste = append(liste, si)
		}
	}
	writeJSON(w, http.StatusOK, liste)
}

// SitzungBeenden widerruft eine einzelne Sitzung.
func (s *Server) SitzungBeenden(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	// user_id in der Bedingung: sonst könnte man mit einer fremden Kennung
	// jemand anderen abmelden.
	tag, err := s.Pool.Exec(r.Context(),
		`UPDATE sitzungen SET widerrufen_am=now()
		 WHERE id=$1 AND user_id=$2 AND widerrufen_am IS NULL`, id, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht beendet werden")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "Sitzung nicht gefunden")
		return
	}
	s.sitzungMerken(id, false)
	s.spurAusRequest(r, AktSitzungBeendet, "sitzung", id, "", nil)

	// Die eigene zu beenden ist erlaubt -- dann muss auch das Plätzchen weg,
	// sonst schickt der Browser weiter ein Token, das nirgends mehr gilt.
	if id == middleware.SitzungID(r) {
		s.clearAuthCookie(w)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// SitzungenBeenden widerruft alle außer der gerade benutzten.
func (s *Server) SitzungenBeenden(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	diese := middleware.SitzungID(r)

	rows, err := s.Pool.Query(r.Context(),
		`UPDATE sitzungen SET widerrufen_am=now()
		 WHERE user_id=$1 AND widerrufen_am IS NULL AND id <> $2
		 RETURNING id`, uid, diese)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht beendet werden")
		return
	}
	defer rows.Close()
	anzahl := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			s.sitzungMerken(id, false)
			anzahl++
		}
	}
	s.spurAusRequest(r, AktSitzungBeendet, "konto", uid, "",
		map[string]any{"beendet": anzahl, "umfang": "alle anderen"})
	writeJSON(w, http.StatusOK, map[string]int{"beendet": anzahl})
}

// SitzungenUhr räumt abgelaufene und widerrufene Sitzungen weg.
//
// Nicht sofort nach dem Ablauf: eine Zeile, die noch ein paar Tage steht,
// beantwortet die Frage "wann war ich zuletzt von wo angemeldet" -- und genau
// die stellt man nach einem Vorfall.
func (s *Server) SitzungenUhr(ctx context.Context) {
	uhr := time.NewTicker(sitzungsTakt)
	defer uhr.Stop()
	for {
		tag, err := s.Pool.Exec(ctx,
			`DELETE FROM sitzungen
			 WHERE laeuft_ab < now() - interval '30 days'
			    OR (widerrufen_am IS NOT NULL AND widerrufen_am < now() - interval '30 days')`)
		if err != nil {
			log.Printf("Sitzungen aufräumen: %v", err)
		} else if n := tag.RowsAffected(); n > 0 {
			log.Printf("Sitzungen: %d alte Zeilen entfernt", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-uhr.C:
		}
	}
}

// SitzungenEinesKontos widerruft alles, was einem Konto gehört. Gebraucht beim
// Passwortwechsel und wenn ein Administrator jemanden aussperrt.
func (s *Server) SitzungenEinesKontos(ctx context.Context, uid string) {
	rows, err := s.Pool.Query(ctx,
		`UPDATE sitzungen SET widerrufen_am=now()
		 WHERE user_id=$1 AND widerrufen_am IS NULL RETURNING id`, uid)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			s.sitzungMerken(id, false)
		}
	}
}

// kurzerBrowser macht aus einer User-Agent-Zeile etwas Lesbares.
//
// Die vollständige Zeile ist ein Absatz voller Altlasten ("Mozilla/5.0" steht
// in jedem Browser, auch in denen, die nie Mozilla waren). Für die Frage "war
// ich das?" reichen Programm und Betriebssystem.
func kurzerBrowser(ua string) string {
	if ua == "" {
		return "unbekannt"
	}
	var programm, system string
	switch {
	case strings.Contains(ua, "Firefox/"):
		programm = "Firefox"
	case strings.Contains(ua, "Edg/"):
		programm = "Edge"
	case strings.Contains(ua, "Chrome/"):
		programm = "Chrome"
	case strings.Contains(ua, "Safari/"):
		programm = "Safari"
	default:
		programm = "anderer Browser"
	}
	switch {
	case strings.Contains(ua, "Windows"):
		system = "Windows"
	case strings.Contains(ua, "Android"):
		system = "Android"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"):
		system = "iOS"
	case strings.Contains(ua, "Mac OS"):
		system = "macOS"
	case strings.Contains(ua, "Linux"):
		system = "Linux"
	}
	if system == "" {
		return programm
	}
	return programm + " auf " + system
}
