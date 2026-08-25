package handlers

import (
	"context"
	"encoding/json"
	"log"
)

// IndexNachziehen fills content_text for pages written before the search index
// existed. Without it the full text search would silently return nothing for
// every older page, the worst kind of failure, because it looks like an empty
// result rather than a broken index.
//
// It runs at startup and is cheap on a warm database: the WHERE clause matches
// nothing once every page has been through it. Work happens in batches so a
// large workspace does not hold one long transaction.
//
// Errors are logged, not fatal. A server that cannot backfill still serves
// pages; refusing to start would turn a degraded search into an outage.
func (s *Server) IndexNachziehen(ctx context.Context) {
	const stapel = 200
	gesamt := 0

	for {
		rows, err := s.Pool.Query(ctx,
			`SELECT id, content FROM pages
			 WHERE content_text = '' AND content IS NOT NULL
			 LIMIT $1`, stapel)
		if err != nil {
			log.Printf("Suchindex nachziehen: %v", err)
			return
		}

		type eintrag struct {
			id   string
			text string
		}
		var zu []eintrag
		for rows.Next() {
			var id string
			var inhalt []byte
			if err := rows.Scan(&id, &inhalt); err != nil {
				continue
			}
			zu = append(zu, eintrag{id: id, text: textAusInhalt(json.RawMessage(inhalt))})
		}
		rows.Close()

		if len(zu) == 0 {
			break
		}

		for _, e := range zu {
			// Eine leere Seite ergibt leeren Text und würde beim nächsten Durchlauf
			// erneut gelesen. Ein einzelnes Leerzeichen bricht die Schleife und
			// stört den tsvector nicht.
			text := e.text
			if text == "" {
				text = " "
			}
			if _, err := s.Pool.Exec(ctx,
				`UPDATE pages SET content_text=$2 WHERE id=$1`, e.id, text); err != nil {
				log.Printf("Suchindex nachziehen (%s): %v", e.id, err)
			}
		}
		gesamt += len(zu)

		if len(zu) < stapel {
			break
		}
	}

	if gesamt > 0 {
		log.Printf("Suchindex: %d Seiten nachgezogen", gesamt)
	}
}
