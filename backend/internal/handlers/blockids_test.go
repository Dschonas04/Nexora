package handlers

import (
	"encoding/json"
	"testing"
)

func TestBlockIDsErgaenzen(t *testing.T) {
	roh := []byte(`[{"type":"checkListItem","props":{"checked":false},"content":[{"type":"text","text":"Do it"}],` +
		`"children":[{"type":"paragraph","props":{"level":2}}]}]`)
	neu := blockIDsErgaenzen(roh)
	var b []map[string]any
	if err := json.Unmarshal(neu, &b); err != nil {
		t.Fatal(err)
	}
	if id, _ := b[0]["id"].(string); len(id) != 36 {
		t.Fatalf("top-level block got no id: %v", b[0]["id"])
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
