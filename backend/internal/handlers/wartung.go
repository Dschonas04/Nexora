// Maintenance functions for administrators: viewing and editing the
// configuration file, restarting the service, emptying the trash of the whole
// instance.
//
// All three are interventions one would otherwise perform on the command line.
// Offering them here does not mean they are harmless, it means they happen on
// the record instead of unnoticed.
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

// Lines whose value is masked in the response.
//
// The file contains credentials. Sending them into a browser window unasked
// would mean carrying a secret over an extra hop and writing it into the history
// of a session somebody leaves open. Whoever wants to change a value writes the
// new one; whoever wants to read it goes to the file.
var geheimeSchluessel = []string{
	"jwt_geheimnis", "datenbank_url", "s3_geheimnis", "s3_zugriffsschluessel",
	"ldap_bind_passwort", "oidc_geheimnis", "lizenz",
}

const versteckt = "********"

// verstecken replaces the values of secret lines with asterisks.
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

// zurueckSetzen puts the masked values back in.
//
// What the browser sends back as asterisks was never at the browser; it still
// stands in the file. At this point the draft therefore takes over the old
// value. Without this step every save would replace the credentials with the
// word "********", and the next start would find no database.
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

// KonfigLesen returns the configuration file as it was loaded.
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
		// A missing file is not an error: the instance then runs from
		// environment variables and defaults. The page should say so rather
		// than show an empty area.
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
			"Die Datei ist für den Dienst nicht beschreibbar. Im Container ist sie meist nur lesend eingehängt.")
	}
	antwort.Hinweise = append(antwort.Hinweise, config.Pruefen(string(roh))...)
	writeJSON(w, http.StatusOK, antwort)
}

// schreibbar checks whether the file can be opened without changing it.
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
	// NurPruefen: judge a draft without writing it.
	NurPruefen bool `json:"nurPruefen"`
}

// KonfigSchreiben speichert einen Entwurf.
//
// The draft is checked before writing and the old version is backed up
// afterwards. Both, because this file decides whether the service comes up at
// all next time: a broken configuration only shows on the next start, and by
// then the old version is gone.
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

	// A timestamped backup. A failure here stops the save: overwriting without
	// a backup removes exactly the way out the backup exists for.
	sicherung, err := sichern(pfad, alt)
	if err != nil {
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
		// The file is only read at startup. That is not a shortcoming to be
		// programmed away: the database address and the session secret are
		// values one must not swap under a running operation.
		"neustartNoetig": true,
	})
}

// sichern makes a copy of the previous version and returns its path.
//
// Into the data directory first, not next to the file. That is not a matter of
// taste: inside a container the configuration usually lives under /etc/nexora,
// that directory belongs to root, and the service runs as its own user, so it
// cannot write beside the file at all. The data directory is the one place it
// may reliably write to.
//
// The fallback next to the file is meant for running without a container, where
// both live together and the backup is expected there.
func sichern(pfad string, inhalt []byte) (string, error) {
	name := fmt.Sprintf("%s.%s.bak", filepath.Base(pfad), time.Now().Format("2006-01-02-1504"))

	speicher.RLock()
	daten := speicher.basis.DatenVerzeich
	speicher.RUnlock()

	if daten != "" {
		ordner := filepath.Join(daten, "konfig-sicherungen")
		if err := os.MkdirAll(ordner, 0o700); err == nil {
			ziel := filepath.Join(ordner, name)
			if err := os.WriteFile(ziel, inhalt, 0o600); err == nil {
				return ziel, nil
			}
		}
	}
	ziel := filepath.Join(filepath.Dir(pfad), name)
	if err := os.WriteFile(ziel, inhalt, 0o600); err != nil {
		return "", err
	}
	return ziel, nil
}

// ersetzen writes the new content in place of the old file.
//
// The clean way first: write beside it, then rename. A crash mid-write then
// leaves no half file behind, and half a configuration is worse than an
// outdated one.
//
// The fallback is for the common case in a container: if the file is mounted
// individually (docker compose with ./config.conf:/etc/nexora/config.conf) the
// path IS the mount point, and a mount point cannot be renamed over. Then the
// only option left is overwriting the existing file. That is unsafe for the
// blink of an eye, which is why a backup was taken beforehand.
func ersetzen(pfad, inhalt string) error {
	vorlaeufig := filepath.Join(filepath.Dir(pfad), "."+filepath.Base(pfad)+".neu")
	if err := os.WriteFile(vorlaeufig, []byte(inhalt), 0o600); err == nil {
		if err := os.Rename(vorlaeufig, pfad); err == nil {
			return nil
		}
		os.Remove(vorlaeufig)
	}
	// Fallback: overwrite in place.
	return os.WriteFile(pfad, []byte(inhalt), 0o600)
}

type neustartReq struct {
	// Bestaetigung has to be the literal word "neustart". An accidental click
	// must not switch the service off.
	Bestaetigung string `json:"bestaetigung"`
}

// Neustart ends the process.
//
// Starting it again is not done here but by whatever runs it: Docker with
// restart: unless-stopped, systemd, Kubernetes. Without something like that the
// service stays down. The interface says so explicitly rather than offer a
// button that might turn out to have been the last one.
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

	// Wait a moment so the answer still goes out. Then exit, orderly enough:
	// open requests finish in milliseconds, and the database holds no state
	// that would have to be wound down here.
	go func() {
		time.Sleep(400 * time.Millisecond)
		log.Printf("beende Prozess für Neustart")
		os.Exit(0)
	}()
}

// PapierkorbLeeren permanently removes deleted pages across the whole instance.
//
// This is a different thing from one account's trash: other people's pages
// disappear here too. Hence administrators only, hence the audit trail, and
// hence the count in the response, so afterwards it is clear what is gone.
func (s *Server) PapierkorbLeeren(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	// The same route as the deadline takes, otherwise the button would one day
	// clean up differently from the clock, and the attachments would be left
	// behind by one of the two.
	anzahl, err := s.PapierkorbAufraeumen(r.Context(), 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Löschen fehlgeschlagen")
		return
	}
	s.spurAusRequest(r, AktPapierkorbLeer, "system", "", "",
		map[string]interface{}{"seiten": anzahl})
	writeJSON(w, http.StatusOK, map[string]int64{"geloescht": anzahl})
}
