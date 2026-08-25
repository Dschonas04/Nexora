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

// papierkorbTakt is the interval between two sweeps. Hourly, not by the minute:
// the deadline is given in days, and keeping a page one hour longer than
// necessary has never hurt anyone.
const papierkorbTakt = time.Hour

// PapierkorbUhr sweeps in a loop of its own until the context ends.
//
// The first pass runs immediately. Someone who restarts the service after
// shortening the deadline should see the result, not wait an hour for it.
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
				// No actor: nobody clicked here. The entry names the deadline
				// so the audit trail explains why the pages disappeared.
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

// PapierkorbAufraeumen deletes whatever has been in the trash longer than tage.
// tage <= 0 deletes everything in it, which is the route the button on the
// maintenance page takes.
//
// The attachments are read first and removed from storage afterwards. The order
// is deliberate: if the row went first and the file stayed behind, nobody would
// know the file exists any more, and an object store would fill up for years
// with files that no page points to.
func (s *Server) PapierkorbAufraeumen(ctx context.Context, tage int) (int64, error) {
	bedingung := `deleted_at IS NOT NULL`
	var args []any
	if tage > 0 {
		// make_interval rather than ($1 || ' days')::interval: the concatenation
		// would want the number as text, and pgx sends a number as a number.
		bedingung += ` AND deleted_at < now() - make_interval(days => $1)`
		args = append(args, tage)
	}

	// The attachments of the whole subtree, not just of the top page: the delete
	// cascades downwards, and the files of the subpages would otherwise be the
	// ones left behind.
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
		// The delete goes ahead even without the list. Leaving a page in the
		// trash because its attachments cannot be enumerated would let the
		// deadline fail at its weakest point.
		log.Printf("Papierkorb: Anhänge nicht ermittelbar: %v", err)
	}

	tag, err := s.Pool.Exec(ctx, `DELETE FROM pages WHERE `+bedingung, args...)
	if err != nil {
		return 0, err
	}

	// Only now the bytes. A failure here is no reason to undo the delete: the
	// page is gone, and an orphaned file is a smaller problem than a page that
	// keeps coming back.
	for _, k := range schluessel {
		if err := s.Ablage.Loeschen(ctx, k); err != nil {
			log.Printf("Papierkorb: Anhang %s blieb liegen: %v", k, err)
		}
	}
	return tag.RowsAffected(), nil
}
