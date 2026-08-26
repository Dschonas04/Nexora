package dok

import (
	"encoding/json"
	"os"
	"testing"
)

// Writes the example file out on request so that it can be checked with a real
// viewer. Without the environment variable nothing happens.
func TestDateienSchreiben(t *testing.T) {
	ziel := os.Getenv("DOK_SCHREIBEN")
	if ziel == "" {
		t.Skip("DOK_SCHREIBEN nicht gesetzt")
	}
	d := AusInhalt(json.RawMessage(beispiel), "Testseite")
	if err := os.WriteFile(ziel+".pdf", PDF(d), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := Word(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ziel+".docx", w, 0o644); err != nil {
		t.Fatal(err)
	}
}
