package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nexora/internal/middleware"
)

func mcpRufen(t *testing.T, s *Server, schluessel, rumpf string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/mcp", strings.NewReader(rumpf))
	if schluessel != "" {
		req.Header.Set("Authorization", "Bearer "+schluessel)
	}
	rec := httptest.NewRecorder()
	s.MCP(rec, req)
	return rec
}

func TestMCP(t *testing.T) {
	alt := middleware.SchluesselPruefer
	defer func() { middleware.SchluesselPruefer = alt }()
	middleware.SchluesselPruefer = func(_ context.Context, k string) (string, bool, bool) {
		return "u1", true, k == "nxr_gut"
	}
	var gesehen string
	s := &Server{Intern: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gesehen = r.Method + " " + r.URL.RequestURI() + " " + r.Header.Get("Authorization")
		w.Write([]byte(`[{"id":"a"},{"id":"b"}]`))
	})}

	if rec := mcpRufen(t, s, "", `{}`); rec.Code != 401 {
		t.Fatalf("ohne Schlüssel %d", rec.Code)
	}
	if rec := mcpRufen(t, s, "nxr_falsch", `{}`); rec.Code != 401 {
		t.Fatalf("falscher Schlüssel %d", rec.Code)
	}
	rec := mcpRufen(t, s, "nxr_gut", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`)
	if !strings.Contains(rec.Body.String(), `"protocolVersion":"2025-03-26"`) {
		t.Fatalf("initialize: %s", rec.Body.String())
	}
	if rec := mcpRufen(t, s, "nxr_gut", `{"jsonrpc":"2.0","method":"notifications/initialized"}`); rec.Code != 202 {
		t.Fatalf("Benachrichtigung %d", rec.Code)
	}
	rec = mcpRufen(t, s, "nxr_gut", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	var liste struct {
		Result struct{ Tools []mcpWerkzeug } `json:"result"`
	}
	json.Unmarshal(rec.Body.Bytes(), &liste)
	if len(liste.Result.Tools) != len(mcpWerkzeuge) {
		t.Fatalf("tools/list: %s", rec.Body.String())
	}
	rec = mcpRufen(t, s, "nxr_gut", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"suchen","arguments":{"query":"a&b"}}}`)
	if gesehen != "GET /api/search?q=a%26b Bearer nxr_gut" || strings.Contains(rec.Body.String(), "isError") {
		t.Fatalf("suchen: %q %s", gesehen, rec.Body.String())
	}
	rec = mcpRufen(t, s, "nxr_gut", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"seite_lesen","arguments":{"id":"../users"}}}`)
	if !strings.Contains(rec.Body.String(), `"isError":true`) {
		t.Fatalf("Pfad in id: %s", rec.Body.String())
	}
}

func TestBloeckeAusOhneTitelDoppelt(t *testing.T) {
	b := bloeckeAus("# Plan\n\nText", "Plan")
	for _, x := range b {
		if x.Type == "heading" {
			t.Fatalf("Titel doppelt: %+v", b)
		}
	}
	if len(b) == 0 {
		t.Fatal("Text fehlt")
	}
}
