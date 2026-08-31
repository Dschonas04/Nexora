// Eine Sicherung wieder einspielen.
//
// Der zerstörerischste Weg, den diese Anwendung hat: er ersetzt den gesamten
// Bestand. Alles hier ist darauf ausgelegt, dass ein Irrtum überlebbar bleibt.
//
// ES WIRD ZUERST GESICHERT. Bevor irgendetwas überschrieben wird, entsteht ein
// Dump des jetzigen Standes im Datenverzeichnis. Wer die falsche Datei
// erwischt, hat damit einen Weg zurück; ohne diesen Schritt wäre ein Fehlgriff
// endgültig, und Fehlgriffe passieren genau hier, weil zwei Archive gleich
// heißen und sich nur im Zeitstempel unterscheiden.
//
// OHNE DIE MARKE FERTIG WIRD ABGELEHNT. Ein mittendrin abgebrochenes Archiv ist
// ein gültiges ZIP und lässt sich öffnen. Es einzuspielen hieße, einen halben
// Bestand über einen ganzen zu legen. Genau dafür schreibt die Sicherung diese
// Marke als letzten Eintrag.
//
// DANACH WIRD NEU GESTARTET. Der Vorrat an Verbindungen hält vorbereitete
// Abfragen auf Tabellen, die es nach dem Einspielen so nicht mehr gibt, und der
// Zwischenspeicher der Einstellungen steht auf den alten Werten. Beides ließe
// sich einzeln nachziehen; ein Neustart macht es vollständig und ist an dieser
// Stelle ohnehin niemandem zu viel.
package handlers

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"nexora/internal/middleware"
)

// So groß darf ein hochgeladenes Archiv höchstens sein. Nicht als Schutz vor
// einer großen Sicherung, sondern damit ein versehentlich gewähltes Abbild von
// zwanzig Gigabyte nicht erst die Platte füllt und dann abgelehnt wird.
const maxSicherungBytes = 8 << 30 // 8 GiB

// Wiederherstellen nimmt ein Archiv entgegen und spielt es ein.
func (s *Server) Wiederherstellen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	if _, err := exec.LookPath("psql"); err != nil {
		writeErr(w, http.StatusPreconditionFailed, "psql fehlt im Abbild")
		return
	}
	if s.DatenbankURL == "" {
		writeErr(w, http.StatusPreconditionFailed, "Die Adresse der Datenbank ist nicht bekannt")
		return
	}

	// Das Archiv landet auf der Platte, nicht im Speicher: für ein ZIP braucht
	// es wahlfreien Zugriff, und eine Sicherung von mehreren Gigabyte im
	// Arbeitsspeicher wäre der Grund, aus dem das Einspielen scheitert.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "Archiv konnte nicht gelesen werden")
		return
	}
	if r.FormValue("bestaetigung") != "wiederherstellen" {
		writeErr(w, http.StatusBadRequest, "Bestätigung fehlt")
		return
	}
	datei, kopf, err := r.FormFile("datei")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Es wurde keine Datei gewählt")
		return
	}
	defer datei.Close()
	if kopf.Size > maxSicherungBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "Das Archiv ist größer als 8 GB")
		return
	}

	ablage := s.datenVerzeichnis()
	// Anlegen, falls es das Verzeichnis noch nicht gibt. Bei einer Instanz, die
	// noch nie einen Anhang geschrieben hat, entsteht es sonst nirgends — und
	// dann scheiterte ausgerechnet das Einspielen daran, dass noch nichts da
	// ist. Genau der Zustand einer frischen Instanz, in die jemand eine
	// Sicherung legen will.
	if err := os.MkdirAll(ablage, 0o700); err != nil {
		writeErr(w, http.StatusInternalServerError,
			"Das Datenverzeichnis "+ablage+" ließ sich nicht anlegen")
		return
	}
	zwischen, err := os.CreateTemp(ablage, "wiederherstellung-*.zip")
	if err != nil {
		writeErr(w, http.StatusInternalServerError,
			"kein Platz für das Archiv in "+ablage)
		return
	}
	defer os.Remove(zwischen.Name())
	defer zwischen.Close()
	if _, err := io.Copy(zwischen, datei); err != nil {
		writeErr(w, http.StatusBadRequest, "Das Archiv kam nicht vollständig an")
		return
	}

	archiv, err := zip.OpenReader(zwischen.Name())
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Das ist kein lesbares ZIP-Archiv")
		return
	}
	defer archiv.Close()

	// ── Prüfen, bevor irgendetwas geschieht ─────────────────────────────
	var dump *zip.File
	anhaenge := map[string]*zip.File{}
	marke := false
	for _, f := range archiv.File {
		name := f.Name
		switch {
		case strings.HasSuffix(name, "/datenbank.sql"):
			dump = f
		case strings.HasSuffix(name, "/FERTIG"):
			marke = true
		default:
			// Anhänge liegen unter anhaenge/<kennung>. Der Name wird auf seinen
			// letzten Teil verkürzt: ein Eintrag wie "../../etc/passwd" wäre
			// sonst ein Weg, außerhalb der Ablage zu schreiben.
			if i := strings.Index(name, "/anhaenge/"); i >= 0 {
				kennung := filepath.Base(name)
				if kennung != "" && kennung != "." && kennung != ".." {
					anhaenge[kennung] = f
				}
			}
		}
	}
	if dump == nil {
		writeErr(w, http.StatusBadRequest,
			"In dem Archiv liegt keine datenbank.sql. Ist das eine Nexora-Sicherung?")
		return
	}
	if !marke {
		writeErr(w, http.StatusBadRequest,
			"Dem Archiv fehlt die Marke FERTIG. Es wurde mittendrin abgebrochen und wird nicht eingespielt.")
		return
	}

	// Die Spur wird VOR dem Einspielen geschrieben, denn danach gibt es diese
	// Datenbank nicht mehr. Sie wird vom eingespielten Stand überschrieben und
	// ist damit weg; der dauerhafte Beleg ist der Rückweg auf der Platte und
	// das Protokoll des Prozesses. Trotzdem hier: wer den Rückweg einspielt,
	// findet darin, dass und wann jemand etwas ersetzt hat.
	s.spurAusRequest(r, AktWiederherstellung, "system", "", kopf.Filename,
		map[string]interface{}{"archiv": kopf.Filename, "bytes": kopf.Size})

	// ── Erst den jetzigen Stand retten ──────────────────────────────────
	stempel := time.Now().Format("2006-01-02_1504")
	rettung := filepath.Join(ablage, "vor-wiederherstellung-"+stempel+".sql")
	if err := s.rettungsDump(r.Context(), rettung); err != nil {
		// Kein Einspielen ohne Rückweg. Lieber gar nicht als unumkehrbar.
		writeErr(w, http.StatusInternalServerError,
			"Der jetzige Stand ließ sich nicht sichern, deshalb wird nichts eingespielt: "+err.Error())
		return
	}
	log.Printf("Wiederherstellung: Stand vorher gesichert nach %s", rettung)

	// ── Einspielen ──────────────────────────────────────────────────────
	quelle, err := dump.Open()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Der Dump ließ sich nicht lesen")
		return
	}
	meldung, err := s.psqlEinspielen(quelle, s.DatenbankURL)
	quelle.Close()
	if err != nil {
		log.Printf("Wiederherstellung gescheitert: %v", err)
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf(
			"Das Einspielen ist gescheitert: %s. Der Stand von vorher liegt als %s im Datenverzeichnis.",
			meldung, filepath.Base(rettung)))
		return
	}

	// ── Anhänge ─────────────────────────────────────────────────────────
	//
	// Vorhandene werden überschrieben, überzählige NICHT gelöscht: eine Datei
	// zu viel in der Ablage kostet Platz, eine zu wenig kostet Inhalt.
	geschrieben, misslungen := 0, 0
	for kennung, f := range anhaenge {
		quelle, err := f.Open()
		if err != nil {
			misslungen++
			continue
		}
		_, err = s.Ablage.Schreiben(context.WithoutCancel(r.Context()), kennung, quelle,
			int64(f.UncompressedSize64), "application/octet-stream")
		quelle.Close()
		if err != nil {
			misslungen++
			continue
		}
		geschrieben++
	}

	log.Printf("Wiederherstellung: eingespielt, %d Anhänge geschrieben, %d misslungen, Rückweg %s",
		geschrieben, misslungen, filepath.Base(rettung))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"anhaenge":   geschrieben,
		"misslungen": misslungen,
		"rueckweg":   filepath.Base(rettung),
		"neustartIn": 1,
		"hinweis":    "Der Dienst startet neu. Die Anmeldung stammt jetzt aus der Sicherung, du wirst dich neu anmelden müssen.",
	})

	// ── Neu starten ─────────────────────────────────────────────────────
	go func() {
		time.Sleep(time.Second)
		log.Printf("beende Prozess nach Wiederherstellung")
		os.Exit(0)
	}()
}

// rettungsDump schreibt den jetzigen Stand als SQL neben die Anhänge.
func (s *Server) rettungsDump(ctx context.Context, pfad string) error {
	ziel, err := os.Create(pfad)
	if err != nil {
		return err
	}
	defer ziel.Close()
	return s.pgDump(ctx, ziel)
}

// psqlEinspielen schiebt den Dump durch psql.
//
// ON_ERROR_STOP=1 mit Absicht: ein Einspielen, das auf halbem Weg stolpert und
// weitermacht, hinterlässt eine Datenbank, die weder der alte noch der neue
// Stand ist. Lieber am ersten Fehler halten, solange der Rückweg noch frisch
// daneben liegt.
func (s *Server) psqlEinspielen(quelle io.Reader, url string) (string, error) {
	befehl := exec.Command("psql", "--quiet", "-v", "ON_ERROR_STOP=1", url)
	befehl.Stdin = quelle
	var meldung strings.Builder
	befehl.Stdout = &meldung
	befehl.Stderr = &meldung
	err := befehl.Run()
	return letzteZeile(meldung.String()), err
}

// datenVerzeichnis ist der Ort für die Zwischendatei und den Rückweg. Dieselbe
// Platte, auf der auch die Anhänge liegen: sie ist die einzige, von der der
// Dienst weiß, dass er dort schreiben darf.
func (s *Server) datenVerzeichnis() string {
	speicher.RLock()
	defer speicher.RUnlock()
	if v := speicher.basis.DatenVerzeich; v != "" {
		return v
	}
	return os.TempDir()
}
