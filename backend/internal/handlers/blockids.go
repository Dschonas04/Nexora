// Block ids for content that did not come from the editor.
//
// BlockNote gives every block an id, and the task list addresses a checklist
// item by it (ticking it off from "My tasks" writes to that block). Content
// written by the importer or through the API usually has none, so its tasks
// would stay invisible until somebody opened and saved the page. Filling the
// ids in on the way into the database closes that gap.
package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
)

// blockIDsErgaenzen gives every block without an id a new one, nested blocks
// included. Content that already has all its ids comes back byte for byte,
// as does anything that is not a block list.
func blockIDsErgaenzen(inhalt []byte) []byte {
	if !bytes.Contains(inhalt, []byte(`"type"`)) {
		return inhalt
	}
	d := json.NewDecoder(bytes.NewReader(inhalt))
	d.UseNumber()
	var bloecke []interface{}
	if d.Decode(&bloecke) != nil {
		return inhalt
	}
	geaendert := false
	var gehe func([]interface{})
	gehe = func(liste []interface{}) {
		for _, b := range liste {
			m, _ := b.(map[string]interface{})
			if m == nil {
				continue
			}
			if id, _ := m["id"].(string); id == "" {
				if _, ok := m["type"].(string); ok {
					m["id"] = neueBlockID()
					geaendert = true
				}
			}
			if kinder, ok := m["children"].([]interface{}); ok {
				gehe(kinder)
			}
		}
	}
	gehe(bloecke)
	if !geaendert {
		return inhalt
	}
	raus, err := json.Marshal(bloecke)
	if err != nil {
		return inhalt
	}
	return raus
}

// neueBlockID makes an id in the shape BlockNote uses: a UUID v4.
func neueBlockID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// BlockIDsNachziehen gives the checklist items of pages imported before the
// importer wrote ids one now, so their tasks show up. Only pages with a
// checklist are read; once they all have ids it changes nothing. Errors are
// logged: a page without ids still opens, it is just missing from "My tasks".
func (s *Server) BlockIDsNachziehen(ctx context.Context) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, content FROM pages WHERE content::text LIKE '%checkListItem%'`)
	if err != nil {
		log.Printf("Block-IDs nachziehen: %v", err)
		return
	}
	type eintrag struct {
		id     string
		inhalt []byte
	}
	var zu []eintrag
	for rows.Next() {
		var id string
		var inhalt []byte
		if rows.Scan(&id, &inhalt) != nil {
			continue
		}
		if neu := blockIDsErgaenzen(inhalt); !bytes.Equal(neu, inhalt) {
			zu = append(zu, eintrag{id, neu})
		}
	}
	rows.Close()
	for _, e := range zu {
		// updated_at stays: nobody edited the page.
		if _, err := s.Pool.Exec(ctx, `UPDATE pages SET content=$2::jsonb WHERE id=$1`, e.id, string(e.inhalt)); err != nil {
			log.Printf("Block-IDs nachziehen (%s): %v", e.id, err)
		}
	}
	if len(zu) > 0 {
		log.Printf("Block-IDs: %d Seiten nachgezogen", len(zu))
	}
}
