package handlers

import (
	"encoding/json"
	"testing"
)

func TestBlockIDsErgaenzen(t *testing.T) {
	roh := []byte(`[{"type":"checkListItem","props":{"checked":false},"content":[{"type":"text","text":"Do it @gast"}],` +
		`"children":[{"type":"paragraph","props":{"level":2}}]}]`)
	neu := blockIDsErgaenzen(roh)
	aufgaben := aufgabenAus(neu)
	if len(aufgaben) != 1 || len(aufgaben[0].BlockID) != 36 || aufgaben[0].Personen[0] != "gast" {
		t.Fatalf("task not found after filling ids: %+v", aufgaben)
	}
	var b []map[string]any
	if err := json.Unmarshal(neu, &b); err != nil {
		t.Fatal(err)
	}
	kind := b[0]["children"].([]any)[0].(map[string]any)
	if kind["id"] == "" || kind["id"] == nil {
		t.Fatal("nested block got no id")
	}
	if string(blockIDsErgaenzen(neu)) != string(neu) {
		t.Fatal("content with all ids must come back unchanged")
	}
	if string(blockIDsErgaenzen([]byte(`[]`))) != `[]` {
		t.Fatal("empty content changed")
	}
}
