// The trash and its expiry.
//
// Deleting a page moves it aside; this is what finally removes it. Two ways in:
// an admin who empties the whole trash by hand, and the clock. The clock is the
// one that matters, a trash nobody empties is not a safety net, it is a second
// copy of everything that was ever deleted, kept forever, including the things
// that were deleted precisely because they should not be kept.
//
// Both ways go through the same function, so "empty now" and "empty after
// thirty days" cannot drift apart in what they consider deletable and in what
// they leave behind.
package handlers

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"nexora/internal/models"
)

// papierkorbTakt ist der Abstand zwischen zwei Durchgängen. Stündlich, nicht
// minütlich: die Frist wird in Tagen angegeben, und eine Seite eine Stunde
// länger zu behalten als nötig hat noch niemandem geschadet.
const papierkorbTakt = time.Hour

// PapierkorbUhr räumt in einer eigenen Schleife auf, bis der Zusammenhang endet.
//
// Der erste Durchgang läuft sofort. Wer den Dienst neu startet, nachdem die
// Frist verkürzt wurde, soll das Ergebnis sehen und nicht eine Stunde warten.
func (s *Server) PapierkorbUhr(ctx context.Context) {
	uhr := time.NewTicker(papierkorbTakt)
	defer uhr.Stop()
	for {
		if tage := PapierkorbTage(); tage > 0 {
			n, err := s.PapierkorbAufraeumen(ctx, tage)
			if err != nil {
				log.Printf("Papierkorb: %v", err)
			} else if n > 0 {
				tagWort := "Tagen"
				if tage == 1 {
					tagWort = "Tag"
				}
				log.Printf("Papierkorb: %d Seiten nach %d %s endgültig gelöscht", n, tage, tagWort)
				details, _ := json.Marshal(map[string]any{"seiten": n, "tage": tage, "durch": "frist"})
				// Kein Akteur: hier hat niemand geklickt. Der Eintrag nennt
				// die Frist, damit in der Prüfspur nachvollziehbar ist, warum
				// die Seiten verschwunden sind.
				s.spur(ctx, models.Spureintrag{
					AkteurName: "Frist",
					Aktion:     AktPapierkorbLeer,
					ObjektArt:  "system",
					Details:    details,
				})
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-uhr.C:
		}
	}
}

// PapierkorbAufraeumen löscht, was länger als tage im Papierkorb liegt. tage <= 0
// löscht alles, was darin liegt, das ist der Weg des Knopfes in der Wartung.
//
// Die Anhänge werden zuerst gelesen und danach aus der Ablage entfernt. Die
// Reihenfolge ist Absicht: verschwindet die Zeile zuerst und die Datei bleibt
// liegen, weiß niemand mehr, dass es sie gibt, ein Objektspeicher füllt sich
// dann jahrelang mit Dateien, zu denen es keine Seite mehr gibt.
func (s *Server) PapierkorbAufraeumen(ctx context.Context, tage int) (int64, error) {
	bedingung := `deleted_at IS NOT NULL`
	var args []any
	if tage > 0 {
		// make_interval statt ($1 || ' days')::interval: die Verkettung würde
		// eine Zahl als Text verlangen, und pgx schickt eine Zahl als Zahl.
		bedingung += ` AND deleted_at < now() - make_interval(days => $1)`
		args = append(args, tage)
	}

	// Die Anhänge des ganzen Teilbaums, nicht nur der obersten Seite: das
	// Löschen kaskadiert nach unten, und die Dateien der Unterseiten wären
	// sonst die, die liegen bleiben.
	var schluessel []string
	rows, err := s.Pool.Query(ctx, `
		WITH RECURSIVE faellig AS (
			SELECT id FROM pages WHERE `+bedingung+`
			UNION ALL
			SELECT p.id FROM pages p JOIN faellig f ON p.parent_id = f.id
		)
		SELECT a.id::text FROM attachments a JOIN faellig f ON f.id = a.page_id`, args...)
	if err == nil {
		for rows.Next() {
			var k string
			if rows.Scan(&k) == nil {
				schluessel = append(schluessel, k)
			}
		}
		rows.Close()
	} else {
		// Ohne die Liste wird trotzdem gelöscht. Eine Seite im Papierkorb
		// liegen zu lassen, weil ihre Anhänge nicht aufzählbar sind, hieße,
		// die Frist an der schwächsten Stelle scheitern zu lassen.
		log.Printf("Papierkorb: Anhänge nicht ermittelbar: %v", err)
	}

	tag, err := s.Pool.Exec(ctx, `DELETE FROM pages WHERE `+bedingung, args...)
	if err != nil {
		return 0, err
	}

	// Erst jetzt die Bytes. Ein Fehler hier ist kein Grund, das Löschen
	// zurückzunehmen, die Seite ist weg, und eine verwaiste Datei ist ein
	// Platzproblem, kein Datenverlust.
	for _, k := range schluessel {
		if err := s.Ablage.Loeschen(ctx, k); err != nil {
			log.Printf("Papierkorb: Anhang %s blieb liegen: %v", k, err)
		}
	}
	return tag.RowsAffected(), nil
}
