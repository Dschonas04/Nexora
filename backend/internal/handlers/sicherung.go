// A complete backup streamed to the browser.
//
// Rationale: the database is not the entire state. Attachments live as
// separate files, on disk or in an object store, and a SQL dump alone leaves
// rows in `attachments` that point to nothing. Discovering that only during
// restore is too late.
//
// Four decisions shape this implementation.
//
// IT IS STREAMED, not buffered. The archive is written piece by piece to the
// response. Building the archive fully in memory would require keeping the
// whole dataset twice and fails exactly at the sizes where backups matter.
//
// THE DUMP COMES FROM `pg_dump`, not from a custom SQL export. What is
// backed up must be restorable; `pg_dump` produces the exact form that
// `psql` expects, including foreign keys, defaults and correct ordering.
//
// ATTACHMENTS ARE READ THROUGH THE STORAGE INTERFACE. This makes the
// procedure agnostic to whether files live on disk or in an object store: the
// same backup works in both cases without special-casing here.
//
// A MARKER IS WRITTEN AT THE END. If the stream is cut in the middle because
// the connection drops or pg_dump fails, the file remains a valid ZIP: a
// half backup looks like a full one. The final entry is named FERTIG; if it
// is missing the archive is incomplete.
package handlers

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// The search index is excluded automatically: `such_tsv` is a GENERATED
// column and `pg_dump` does not include its contents, only the expression that
// defines it. PostgreSQL recomputes it on restore. The same applies to the
// GIN index over it.

// `SicherungUmfang` reports what a backup would contain.
//
// A separate endpoint is used because the UI needs to know BEFORE starting a
// download: for a dataset of several gigabytes the user should not press the
// button and then wonder whether the process hung or is simply slow.
func (s *Server) SicherungUmfang(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	ctx := r.Context()

	var dbBytes, anhangBytes int64
	var anhaenge int
	_ = s.Pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&dbBytes)
	_ = s.Pool.QueryRow(ctx,
		`SELECT count(*), coalesce(sum(size), 0) FROM attachments`).Scan(&anhaenge, &anhangBytes)

	fehler := ""
	if _, err := exec.LookPath("pg_dump"); err != nil {
		fehler = "pg_dump fehlt im Abbild. Ohne das Werkzeug lässt sich die Datenbank nicht sichern."
	} else if s.DatenbankURL == "" {
		fehler = "Die Adresse der Datenbank ist nicht bekannt."
	}

	// The complete command for a retrieval script. The target URL comes from
	// configuration if present; otherwise a placeholder that is clearly
	// identifiable. Anyone who has to retype the command will likely make
	// mistakes copying it.
	ziel := "https://NEXORA-HOST"
	if u := speicherOeffentlicheURL(); u != "" {
		ziel = strings.TrimSuffix(u, "/")
	}
	token := SicherungToken()
	wort := token
	if wort == "" {
		wort = "<erst ein Losungswort erzeugen>"
	}
	befehl := fmt.Sprintf(`#!/bin/sh
# Nexora sichern. Taeglich per cron, etwa:  30 2 * * *  /pfad/zu/diesem/skript
set -eu
ZIEL=/var/backups/nexora
WORT='%s'
mkdir -p "$ZIEL"
NAME="$ZIEL/nexora-$(date +%%Y-%%m-%%d_%%H%%M).zip"

curl -fsS --max-time 3600      -H "Authorization: Bearer $WORT"      -o "$NAME"      %s/api/system/sicherung

# Die Marke am Ende beweist, dass das Archiv vollstaendig ist. Ein halbes ZIP
# ist ein gueltiges ZIP, ohne diese Pruefung faellt der Abbruch nicht auf.
if unzip -l "$NAME" | grep -q '/FERTIG$'; then
    echo "vollstaendig: $NAME"
    find "$ZIEL" -name 'nexora-*.zip' -mtime +14 -delete
else
    echo "UNVOLLSTAENDIG, nicht verwenden: $NAME" >&2
    exit 1
fi
`, wort, ziel)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tokenGesetzt":   token != "",
		"token":          token,
		"skript":         befehl,
		"datenbankBytes": dbBytes,
		"anhaenge":       anhaenge,
		"anhaengeBytes":  anhangBytes,
		"ablage":         s.Ablage.Name(),
		// The dump is textual and compresses well; attachments are usually
		// already compressed. This is an estimate and is labelled as such.
		"geschaetztBytes": dbBytes/4 + anhangBytes,
		"bereit":          fehler == "",
		"fehler":          fehler,
	})
}

// `SicherungTokenNeu` generates the passphrase for retrieval without login.
func (s *Server) SicherungTokenNeu(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	roh := make([]byte, 32)
	if _, err := rand.Read(roh); err != nil {
		writeErr(w, http.StatusInternalServerError, "Zufallsquelle nicht verfügbar")
		return
	}
	if err := s.einstellungSchreiben(r.Context(), "sicherung_token", hex.EncodeToString(roh)); err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht gespeichert werden")
		return
	}
	s.spurAusRequest(r, AktEinstellung, "einstellung", "sicherung_token", "Sicherung",
		map[string]interface{}{"aktion": "Losungswort erzeugt"})
	s.SicherungUmfang(w, r)
}

// `SicherungTokenWeg` removes it. After this the backup is only available
// from the admin panel.
func (s *Server) SicherungTokenWeg(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	if err := s.einstellungSchreiben(r.Context(), "sicherung_token", ""); err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht gespeichert werden")
		return
	}
	s.spurAusRequest(r, AktEinstellung, "einstellung", "sicherung_token", "Sicherung",
		map[string]interface{}{"aktion": "Losungswort entfernt"})
	s.SicherungUmfang(w, r)
}

// Sicherung schreibt das Archiv in die Antwort.
func (s *Server) Sicherung(w http.ResponseWriter, r *http.Request) {
	// Two ways in, and only two: a logged-in admin, or a valid passphrase.
	// A middleware has already determined which was used; this is the check
	// for the admin case.
	uid := middleware.UserID(r)
	ueberToken := perToken(r)
	if !ueberToken && !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	// All checks that can fail BEFORE the first byte is written.
	//
	// Once the response headers are sent a proper error status cannot be
	// returned; the user would receive a corrupted archive with a 200 code.
	// Perform the checks here so failures produce a clean error.
	if s.DatenbankURL == "" {
		writeErr(w, http.StatusPreconditionFailed, "Die Adresse der Datenbank ist nicht bekannt")
		return
	}
	if _, err := exec.LookPath("pg_dump"); err != nil {
		writeErr(w, http.StatusPreconditionFailed,
			"pg_dump fehlt im Abbild. Ohne das Werkzeug lässt sich die Datenbank nicht sichern.")
		return
	}

	// Detached from the request timeout.
	//
	// The router applies a 30 second timeout to every request, which is far
	// too short for a backup. Watching r.Context() here would be wrong even
	// though it seems appropriate: that context signals both a gone browser
	// AND the timeout, and they cannot be distinguished. The backup would
	// then always abort after 30 seconds.
	//
	// A gone browser still stops the process, but via a different mechanism:
	// writing to a closed connection fails, pg_dump receives a broken pipe and
	// returns an error.
	ctx, abbruch := context.WithCancel(context.WithoutCancel(r.Context()))
	defer abbruch()

	stempel := time.Now().Format("2006-01-02_1504")
	wurzel := "nexora-sicherung-" + stempel
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s.zip"`, wurzel))
	// No buffering on the way: the response is a stream and a proxy that
	// fully buffers it would make an ongoing backup appear stalled for
	// minutes.
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Cache-Control", "no-store")

	archiv := zip.NewWriter(w)
	fertig := false
	defer func() {
		if err := archiv.Close(); err != nil {
			log.Printf("Sicherung: closing archive: %v", err)
		}
		if !fertig {
			log.Printf("Sicherung: aborted before completion")
		}
	}()

	// ── Instructions first ──────────────────────────────────────────────
	//
	// Placed as the first entry so it appears at the top when opened. In an
	// emergency the person inspecting an archive should not have to hunt for
	// documentation.
	if err := s.schreibeEintrag(archiv, wurzel+"/LIESMICH.md",
		[]byte(liesmich(stempel, s.Ablage.Name()))); err != nil {
		return
	}

	// ── The database ───────────────────────────────────────────────────
	datei, err := archiv.Create(wurzel + "/datenbank.sql")
	if err != nil {
		return
	}
	if err := s.pgDump(ctx, datei); err != nil {
		log.Printf("Sicherung: pg_dump: %v", err)
		// The headers have already been sent. Instead of an HTTP status the
		// note remains inside the archive itself: the FERTIG marker will be
		// missing and the README explains what that means.
		return
	}

	// ── Attachments ───────────────────────────────────────────────────
	anzahl, uebersprungen, err := s.anhaengeSichern(ctx, archiv, wurzel)
	if err != nil {
		log.Printf("Sicherung: Anhänge: %v", err)
		return
	}

	// ── The marker ─────────────────────────────────────────────────────
	inhalt := fmt.Sprintf("Sicherung vollstaendig.\nErstellt: %s\nAnhaenge: %d\n",
		time.Now().Format(time.RFC3339), anzahl)
	if uebersprungen > 0 {
		inhalt += fmt.Sprintf("Nicht lesbar und darum uebersprungen: %d\n", uebersprungen)
	}
	if err := s.schreibeEintrag(archiv, wurzel+"/FERTIG", []byte(inhalt)); err != nil {
		return
	}
	fertig = true

	// Every retrieval is recorded in the audit trail with its source
	// address. For a script this is the only trace it leaves: it has no
	// account to later identify who fetched the whole dataset.
	if ueberToken {
		s.spur(ctx, models.Spureintrag{
			Aktion: AktSicherung, ObjektArt: "system", ObjektTitel: "Sicherung",
			AkteurName: "Skript mit Losungswort", IP: absenderIP(r),
			Details: []byte(fmt.Sprintf(`{"anhaenge":%d,"uebersprungen":%d,"weg":"losungswort"}`,
				anzahl, uebersprungen)),
		})
	} else {
		s.spurAusRequest(r, AktSicherung, "system", "", "Sicherung",
			map[string]interface{}{"anhaenge": anzahl, "uebersprungen": uebersprungen, "weg": "panel"})
	}
}

func (s *Server) schreibeEintrag(archiv *zip.Writer, name string, inhalt []byte) error {
	f, err := archiv.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write(inhalt)
	return err
}

// `pgDump` invokes the tool and streams its output.
func (s *Server) pgDump(ctx context.Context, ziel io.Writer) error {
	befehl := exec.CommandContext(ctx, "pg_dump",
		// Do not include ownership and privileges: on restore the role names
		// often differ, and a dump that insists on a foreign role name fails
		// for reasons unrelated to the data.
		"--no-owner", "--no-privileges",
		// Clean before restore so the same dump can be applied into a
		// database that already contains objects.
		"--clean", "--if-exists",
		s.DatenbankURL)
	// Das Losungswort steht in der Adresse und hat in keiner Fehlermeldung
	// etwas zu suchen; deshalb wird stderr eingesammelt und nicht
	// weitergereicht.
	var meldung strings.Builder
	befehl.Stdout = ziel
	befehl.Stderr = &meldung
	if err := befehl.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, letzteZeile(meldung.String()))
	}
	return nil
}

func letzteZeile(s string) string {
	zeilen := strings.Split(strings.TrimSpace(s), "\n")
	return zeilen[len(zeilen)-1]
}

// anhaengeSichern places every attachment into the archive, read via the
// storage interface.
//
// A missing file does not abort the backup but is counted as skipped. That is
// the purpose of a backup: to rescue what exists rather than fail on what is
// already missing. The count is recorded in the final marker.
func (s *Server) anhaengeSichern(ctx context.Context, archiv *zip.Writer, wurzel string) (int, int, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id::text, filename FROM attachments ORDER BY created_at`)
	if err != nil {
		return 0, 0, err
	}
	type anhang struct{ id, name string }
	liste := []anhang{}
	for rows.Next() {
		var a anhang
		if rows.Scan(&a.id, &a.name) == nil {
			liste = append(liste, a)
		}
	}
	rows.Close()

	anzahl, uebersprungen := 0, 0
	for _, a := range liste {
		quelle, err := s.Ablage.Lesen(ctx, a.id)
		if err != nil {
			uebersprungen++
			continue
		}
		// The filename in the archive is the ID, not the original name: many
		// attachments may share the same display name and the database finds
		// them by ID. The human-readable name is stored in the database.
		ziel, err := archiv.Create(fmt.Sprintf("%s/anhaenge/%s", wurzel, a.id))
		if err != nil {
			quelle.Close()
			return anzahl, uebersprungen, err
		}
		_, err = io.Copy(ziel, quelle)
		quelle.Close()
		if err != nil {
			return anzahl, uebersprungen, err
		}
		anzahl++
	}
	return anzahl, uebersprungen, nil
}

func liesmich(stempel, ablage string) string {
	return `# Nexora-Sicherung ` + stempel + `

Erstellt aus der laufenden Instanz, Ablage der Anhaenge: ` + ablage + `

## Zuerst nachsehen

Liegt neben dieser Datei eine Datei **FERTIG**, ist das Archiv vollstaendig.
Fehlt sie, ist die Sicherung mittendrin abgebrochen. Ein ZIP bleibt dabei ein
gueltiges ZIP und laesst sich oeffnen, es ist nur nicht alles darin. Eine
Sicherung ohne FERTIG nicht verwenden.

## Was drin ist

    datenbank.sql   Der vollstaendige Dump, erzeugt mit pg_dump
    anhaenge/       Jede Datei unter ihrer Kennung aus der Tabelle attachments

Der Suchindex ist NICHT enthalten und muss es nicht sein: die Spalte such_tsv
wird von PostgreSQL aus Titel und Text berechnet und entsteht beim Einspielen
neu. Dasselbe gilt fuer den Index darueber.

## Zurueckspielen

Datenbank, in eine leere oder eine bestehende:

    gunzip -c datenbank.sql | psql "postgres://nexora@HOST:5432/nexora"

Der Dump raeumt vorher auf (--clean --if-exists), laesst sich also auch in eine
Datenbank legen, in der schon etwas steht. Eigentuemer und Rechte stehen nicht
darin: das Konto darf beim Einspielen anders heissen.

Anhaenge: den Inhalt von anhaenge/ in das Datenverzeichnis der neuen Instanz
legen (Vorgabe /data/attachments) oder in den Eimer des Objektspeichers, jeweils
unter demselben Namen. Die Datenbank findet sie ueber die Kennung wieder.

## Was NICHT drin ist

    config.conf     Steht auf dem Wirt und enthaelt Geheimnisse
    Der Lizenzschluessel, falls er nur in der Datei steht

## Vorsicht

Dieses Archiv enthaelt alles: Passwort-Hashes, Sitzungen, Freigabe-Tokens, den
gesamten Inhalt. Es ist die empfindlichste Datei, die diese Instanz herausgibt.
`
}
