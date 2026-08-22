// Wartungsfunktionen für Administratoren: die Konfigurationsdatei ansehen und
// ändern, den Dienst neu starten, den Papierkorb der ganzen Instanz leeren.
//
// Alle drei sind Eingriffe, die man sonst auf der Kommandozeile machen würde.
// Sie hier anzubieten heisst nicht, dass sie harmlos wären -- es heisst, dass
// sie protokolliert stattfinden statt unbemerkt.
package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nexora/internal/config"
	"nexora/internal/middleware"
)

// Zeilen, deren Wert in der Antwort unkenntlich gemacht wird.
//
// Die Datei enthält Zugangsdaten. Sie ungefragt in ein Browserfenster zu
// schicken hiesse, ein Geheimnis über eine zusätzliche Strecke zu tragen und in
// den Verlauf einer Sitzung zu schreiben, die jemand offen stehen lässt. Wer
// den Wert ändern will, schreibt den neuen hin; wer ihn lesen will, geht an die
// Datei.
var geheimeSchluessel = []string{
	"jwt_geheimnis", "datenbank_url", "s3_geheimnis", "s3_zugriffsschluessel",
	"ldap_bind_passwort", "oidc_geheimnis", "lizenz",
}

const versteckt = "********"

// verstecken ersetzt die Werte der Geheimniszeilen durch Sterne.
func verstecken(inhalt string) string {
	zeilen := strings.Split(inhalt, "\n")
	for i, z := range zeilen {
		blank := strings.TrimSpace(z)
		if blank == "" || strings.HasPrefix(blank, "#") || strings.HasPrefix(blank, ";") ||
			strings.HasPrefix(blank, "[") {
			continue
		}
		g := strings.Index(z, "=")
		if g < 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(z[:g]))
		for _, geheim := range geheimeSchluessel {
			if name == geheim && strings.TrimSpace(z[g+1:]) != "" {
				zeilen[i] = z[:g+1] + " " + versteckt
				break
			}
		}
	}
	return strings.Join(zeilen, "\n")
}

// zurueckSetzen bringt die versteckten Werte wieder ein.
//
// Was der Browser als Sterne zurückschickt, war nie beim Browser -- es steht
// weiterhin in der Datei. Der Entwurf übernimmt an dieser Stelle also den alten
// Wert. Ohne diesen Schritt würde jedes Speichern die Zugangsdaten durch das
// Wort "********" ersetzen, und beim nächsten Start käme keine Datenbank mehr.
func zurueckSetzen(entwurf, alt string) string {
	alteWerte := map[string]string{}
	for _, z := range strings.Split(alt, "\n") {
		g := strings.Index(z, "=")
		if g < 0 {
			continue
		}
		alteWerte[strings.ToLower(strings.TrimSpace(z[:g]))] = strings.TrimSpace(z[g+1:])
	}

	zeilen := strings.Split(entwurf, "\n")
	for i, z := range zeilen {
		g := strings.Index(z, "=")
		if g < 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(z[:g]))
		if strings.TrimSpace(z[g+1:]) != versteckt {
			continue
		}
		if v, da := alteWerte[name]; da {
			zeilen[i] = z[:g+1] + " " + v
		}
	}
	return strings.Join(zeilen, "\n")
}

type konfigAntwort struct {
	Pfad        string   `json:"pfad"`
	Inhalt      string   `json:"inhalt"`
	Gefunden    bool     `json:"gefunden"`
	Schreibbar  bool     `json:"schreibbar"`
	Hinweise    []string `json:"hinweise"`
	Schluessel  []string `json:"schluessel"`
	Geheimnisse []string `json:"geheimnisse"`
}

// KonfigLesen liefert die geladene Konfigurationsdatei.
func (s *Server) KonfigLesen(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	speicher.RLock()
	pfad := speicher.basis.Pfad
	speicher.RUnlock()

	antwort := konfigAntwort{
		Pfad:        pfad,
		Hinweise:    []string{},
		Schluessel:  config.BekannteSchluessel(),
		Geheimnisse: geheimeSchluessel,
	}
	if pfad == "" {
		// Ohne Datei ist das kein Fehler: die Instanz läuft dann aus
		// Umgebungsvariablen und Vorgaben. Die Seite soll das sagen, statt
		// eine leere Fläche zu zeigen.
		antwort.Hinweise = append(antwort.Hinweise,
			"Es wurde keine config.conf gefunden. Diese Instanz läuft aus Umgebungsvariablen und Vorgaben.")
		writeJSON(w, http.StatusOK, antwort)
		return
	}
	roh, err := os.ReadFile(pfad)
	if err != nil {
		antwort.Hinweise = append(antwort.Hinweise, "Datei nicht lesbar: "+err.Error())
		writeJSON(w, http.StatusOK, antwort)
		return
	}
	antwort.Gefunden = true
	antwort.Inhalt = verstecken(string(roh))
	antwort.Schreibbar = schreibbar(pfad)
	if !antwort.Schreibbar {
		antwort.Hinweise = append(antwort.Hinweise,
			"Die Datei ist für den Dienst nicht beschreibbar -- im Container ist sie meist nur lesend eingehängt.")
	}
	antwort.Hinweise = append(antwort.Hinweise, config.Pruefen(string(roh))...)
	writeJSON(w, http.StatusOK, antwort)
}

// schreibbar prüft, ob sich die Datei öffnen lässt, ohne sie zu verändern.
func schreibbar(pfad string) bool {
	f, err := os.OpenFile(pfad, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

type konfigSchreibReq struct {
	Inhalt string `json:"inhalt"`
	// NurPruefen: Entwurf beurteilen, ohne ihn zu schreiben.
	NurPruefen bool `json:"nurPruefen"`
}

// KonfigSchreiben speichert einen Entwurf.
//
// Vor dem Schreiben wird geprüft, danach eine Sicherung der alten Fassung
// angelegt. Beides, weil diese Datei bestimmt, ob der Dienst beim nächsten Mal
// überhaupt hochkommt: eine kaputte Konfiguration merkt man erst beim Start,
// und dann ist die alte Fassung weg.
func (s *Server) KonfigSchreiben(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	var req konfigSchreibReq
	_ = decode(r, &req)

	speicher.RLock()
	pfad := speicher.basis.Pfad
	speicher.RUnlock()
	if pfad == "" {
		writeErr(w, http.StatusBadRequest, "diese Instanz hat keine config.conf")
		return
	}
	alt, err := os.ReadFile(pfad)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "alte Fassung nicht lesbar")
		return
	}

	entwurf := strings.ReplaceAll(req.Inhalt, "\r\n", "\n")
	entwurf = zurueckSetzen(entwurf, string(alt))
	hinweise := config.Pruefen(entwurf)

	if req.NurPruefen {
		writeJSON(w, http.StatusOK, map[string]interface{}{"hinweise": hinweise})
		return
	}
	if strings.TrimSpace(entwurf) == "" {
		writeErr(w, http.StatusBadRequest, "eine leere Konfiguration wird nicht gespeichert")
		return
	}

	// Sicherung neben die Datei, mit Zeitstempel im Namen. Ein Fehler dabei
	// hält das Speichern auf: ohne Sicherung zu überschreiben nimmt genau den
	// Ausweg weg, für den sie da ist.
	sicherung := fmt.Sprintf("%s.%s.bak", pfad, time.Now().Format("2006-01-02-1504"))
	if err := os.WriteFile(sicherung, alt, 0o600); err != nil {
		writeErr(w, http.StatusInternalServerError,
			"Sicherung konnte nicht angelegt werden: "+err.Error())
		return
	}

	if err := ersetzen(pfad, entwurf); err != nil {
		writeErr(w, http.StatusInternalServerError, "nicht schreibbar: "+err.Error())
		return
	}

	s.spurAusRequest(r, AktKonfigGeaendert, "system", pfad, filepath.Base(pfad),
		map[string]interface{}{"sicherung": filepath.Base(sicherung), "zeichen": len(entwurf)})
	log.Printf("Konfiguration geändert, Sicherung: %s", sicherung)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"hinweise":  hinweise,
		"sicherung": filepath.Base(sicherung),
		// Gelesen wird die Datei nur beim Start. Das ist kein Mangel, den man
		// wegprogrammieren sollte: die Datenbankadresse und das
		// Sitzungsgeheimnis im laufenden Betrieb zu wechseln hiesse, jeden
		// offenen Vorgang unter der Hand umzuhängen.
		"neustartNoetig": true,
	})
}

// ersetzen schreibt den neuen Inhalt an die Stelle der alten Datei.
//
// Zuerst der saubere Weg: daneben schreiben, dann umbenennen. Ein Absturz
// mitten im Schreiben hinterlässt so keine halbe Datei -- und eine halbe
// Konfiguration ist schlimmer als eine veraltete.
//
// Der Weg scheitert aber genau dort, wo diese Anwendung meistens läuft: hängt
// die Datei einzeln in den Container (docker-compose mit
// ./config.conf:/etc/nexora/config.conf), dann IST der Pfad der Einhängepunkt,
// und über einen Einhängepunkt lässt sich nicht umbenennen. Dann bleibt nur,
// die vorhandene Datei zu überschreiben. Das ist einen Wimpernschlag lang
// unsicher -- deshalb wurde vorher eine Sicherung angelegt.
func ersetzen(pfad, inhalt string) error {
	vorlaeufig := filepath.Join(filepath.Dir(pfad), "."+filepath.Base(pfad)+".neu")
	if err := os.WriteFile(vorlaeufig, []byte(inhalt), 0o600); err == nil {
		if err := os.Rename(vorlaeufig, pfad); err == nil {
			return nil
		}
		os.Remove(vorlaeufig)
	}
	// Rückfall: an Ort und Stelle überschreiben.
	return os.WriteFile(pfad, []byte(inhalt), 0o600)
}

type neustartReq struct {
	// Bestaetigung muss wörtlich "neustart" sein. Ein versehentlicher Klick
	// soll den Dienst nicht abschalten.
	Bestaetigung string `json:"bestaetigung"`
}

// Neustart beendet den Prozess.
//
// Neu gestartet wird er nicht von hier, sondern von dem, was ihn betreibt --
// Docker mit restart: unless-stopped, systemd, Kubernetes. Ohne so etwas bleibt
// der Dienst aus. Deshalb sagt die Oberfläche das ausdrücklich dazu, statt
// einen Knopf anzubieten, der im schlechtesten Fall der letzte war.
func (s *Server) Neustart(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	var req neustartReq
	_ = decode(r, &req)
	if req.Bestaetigung != "neustart" {
		writeErr(w, http.StatusBadRequest, "Bestätigung fehlt")
		return
	}
	s.spurAusRequest(r, AktNeustart, "system", "", "", nil)
	log.Printf("Neustart durch Administrator %s angefordert", uid)

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})

	// Kurz warten, damit die Antwort noch hinausgeht. Danach beenden --
	// geordnet genug: offene Anfragen laufen in Millisekunden, und die
	// Datenbank hält keinen Zustand, der hier abgeschlossen werden müsste.
	go func() {
		time.Sleep(400 * time.Millisecond)
		log.Printf("beende Prozess für Neustart")
		os.Exit(0)
	}()
}

// PapierkorbLeeren entfernt gelöschte Seiten der ganzen Instanz endgültig.
//
// Das ist etwas anderes als der Papierkorb eines Kontos: hier verschwinden auch
// fremde Seiten. Deshalb Admin, deshalb Prüfspur, und deshalb steht die Anzahl
// in der Antwort -- damit hinterher feststeht, was weg ist.
func (s *Server) PapierkorbLeeren(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	tag, err := s.Pool.Exec(r.Context(), `DELETE FROM pages WHERE deleted_at IS NOT NULL`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Löschen fehlgeschlagen")
		return
	}
	anzahl := tag.RowsAffected()
	s.spurAusRequest(r, AktPapierkorbLeer, "system", "", "",
		map[string]interface{}{"seiten": anzahl})
	writeJSON(w, http.StatusOK, map[string]int64{"geloescht": anzahl})
}
