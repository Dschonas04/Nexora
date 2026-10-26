// Block ids for content that did not come from the editor.
//
// BlockNote gives every block an id, and the editor relies on it. Content
// written by the importer or through the API usually has none. Filling the ids
// in on the way into the database means such a page looks to the editor like
// any page written in it.
package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
