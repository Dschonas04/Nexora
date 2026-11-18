// MCP server (beta): Nexora as a tool for AI assistants.
//
// Claude, an IDE agent or a local model connect to
//
//	POST /api/mcp     Authorization: Bearer nxr_<key>
//
// and get tools to search, read and write pages. It speaks the Model Context
// Protocol over "Streamable HTTP" in its plainest form: every JSON-RPC request
// is answered with one JSON response, no event stream, no server state.
//
// Every tool is carried out as an ordinary request against the instance's own
// API, with the caller's key. That is the whole security design: a tool can do
// exactly what the key can do over the REST API, the same permission checks,
// licence switches, audit entries and webhooks apply, and a read-only key
// cannot write through the back door.
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"nexora/internal/einlesen"
	"nexora/internal/middleware"
)

const mcpProtokoll = "2025-06-18"

type mcpAnfrage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpFehler struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpAntwort struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpFehler      `json:"error,omitempty"`
}

type mcpWerkzeug struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

func schema(pflicht []string, felder map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": felder}
	if len(pflicht) > 0 {
		s["required"] = pflicht
	}
	return s
}

func feld(typ, beschreibung string) map[string]any {
	return map[string]any{"type": typ, "description": beschreibung}
}

var lesend = map[string]any{"readOnlyHint": true}

var mcpWerkzeuge = []mcpWerkzeug{
	{
		Name: "suchen", Title: "Search pages",
		Description: "Full text search over all pages the account can read. Returns id, title, space and a snippet per hit.",
		InputSchema: schema([]string{"query"}, map[string]any{
			"query":    feld("string", "Search words"),
			"space_id": feld("string", "Only search in this space (optional)"),
		}),
		Annotations: lesend,
	},
	{
		Name: "ablagen", Title: "List spaces",
		Description: "Lists the spaces (top-level areas of the wiki) the account can reach, with id and name.",
		InputSchema: schema(nil, map[string]any{}),
		Annotations: lesend,
	},
	{
		Name: "seiten", Title: "List pages",
		Description: "Lists pages with id, title, parent and space, newest first. Optionally only those of one space.",
		InputSchema: schema(nil, map[string]any{
			"space_id": feld("string", "Only pages of this space (optional)"),
		}),
		Annotations: lesend,
	},
	{
		Name: "seite_lesen", Title: "Read page",
		Description: "Returns one page as Markdown.",
		InputSchema: schema([]string{"id"}, map[string]any{
			"id": feld("string", "Page id"),
		}),
		Annotations: lesend,
	},
	{
		Name: "seite_anlegen", Title: "Create page",
		Description: "Creates a page from Markdown. Needs a key with write access.",
		InputSchema: schema([]string{"title"}, map[string]any{
			"title":     feld("string", "Title of the page"),
			"markdown":  feld("string", "Content as Markdown (optional)"),
			"space_id":  feld("string", "Space to create it in (optional)"),
			"parent_id": feld("string", "Parent page, makes it a subpage (optional)"),
		}),
	},
	{
		Name: "seite_aendern", Title: "Change page",
		Description: "Changes title and/or content of a page. mode 'replace' (default) replaces the content with the Markdown, 'append' adds it at the end. Needs a key with write access.",
		InputSchema: schema([]string{"id"}, map[string]any{
			"id":       feld("string", "Page id"),
			"title":    feld("string", "New title (optional)"),
			"markdown": feld("string", "New content as Markdown (optional)"),
			"mode":     map[string]any{"type": "string", "enum": []string{"replace", "append"}, "description": "replace or append"},
		}),
		Annotations: map[string]any{"destructiveHint": true},
	},
}

// MCP answers the JSON-RPC calls of an MCP client.
func (s *Server) MCP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent) // no sessions to end
		return
	default:
		// No server-initiated stream: the client then just uses POST.
		w.Header().Set("Allow", "POST, DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	schluessel := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if middleware.SchluesselPruefer == nil || schluessel == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="nexora"`)
		writeErr(w, http.StatusUnauthorized, "API-Schlüssel fehlt")
		return
	}
	if _, _, ok := middleware.SchluesselPruefer(r.Context(), schluessel); !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="nexora", error="invalid_token"`)
		writeErr(w, http.StatusUnauthorized, "API-Schlüssel ungültig")
		return
	}

	roh, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "zu groß")
		return
	}
	// A batch is allowed by older revisions of the protocol; answer it as one.
	if t := bytes.TrimSpace(roh); len(t) > 0 && t[0] == '[' {
		var liste []mcpAnfrage
		if json.Unmarshal(t, &liste) != nil {
			writeJSON(w, http.StatusOK, mcpAntwort{JSONRPC: "2.0", ID: json.RawMessage("null"),
				Error: &mcpFehler{-32700, "parse error"}})
			return
		}
		var raus []mcpAntwort
		for _, a := range liste {
			if antw, ok := s.mcpEine(r, schluessel, a); ok {
				raus = append(raus, antw)
			}
		}
		if len(raus) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeJSON(w, http.StatusOK, raus)
		return
	}
	var a mcpAnfrage
	if json.Unmarshal(roh, &a) != nil {
		writeJSON(w, http.StatusOK, mcpAntwort{JSONRPC: "2.0", ID: json.RawMessage("null"),
			Error: &mcpFehler{-32700, "parse error"}})
		return
	}
	antw, ok := s.mcpEine(r, schluessel, a)
	if !ok {
		w.WriteHeader(http.StatusAccepted) // a notification gets no answer
		return
	}
	writeJSON(w, http.StatusOK, antw)
}

// mcpEine handles one message. ok is false for notifications.
func (s *Server) mcpEine(r *http.Request, schluessel string, a mcpAnfrage) (mcpAntwort, bool) {
	if len(a.ID) == 0 {
		return mcpAntwort{}, false
	}
	antw := mcpAntwort{JSONRPC: "2.0", ID: a.ID}
	switch a.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(a.Params, &p)
		version := mcpProtokoll
		switch p.ProtocolVersion {
		case "2024-11-05", "2025-03-26", "2025-06-18":
			version = p.ProtocolVersion
		}
		antw.Result = map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "nexora", "title": "Nexora (beta)", "version": "beta"},
			"instructions": "Nexora is a wiki. Pages live in spaces and can have subpages. " +
				"Search before creating a page, read a page before changing it, and write content as Markdown.",
		}
	case "ping":
		antw.Result = map[string]any{}
	case "tools/list":
		antw.Result = map[string]any{"tools": mcpWerkzeuge}
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(a.Params, &p) != nil {
			antw.Error = &mcpFehler{-32602, "invalid params"}
			break
		}
		text, fehler := s.mcpWerkzeug(r.Context(), r, schluessel, p.Name, p.Arguments)
		if fehler != nil {
			antw.Result = map[string]any{
				"content": []any{map[string]any{"type": "text", "text": fehler.Error()}},
				"isError": true,
			}
			break
		}
		antw.Result = map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}
	default:
		antw.Error = &mcpFehler{-32601, "method not found: " + a.Method}
	}
	return antw, true
}

// intern runs one request against the instance's own API with the caller's
// key, through the full router: same checks as from outside.
func (s *Server) intern(ctx context.Context, aussen *http.Request, schluessel, methode, pfad string, rumpf any) (int, []byte) {
	if s.Intern == nil {
		return http.StatusServiceUnavailable, []byte(`{"error":"intern nicht verfügbar"}`)
	}
	var body io.Reader
	if rumpf != nil {
		b, _ := json.Marshal(rumpf)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequestWithContext(ctx, methode, pfad, body)
	req.Header.Set("Authorization", "Bearer "+schluessel)
	if rumpf != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "nexora-mcp")
	req.RemoteAddr = aussen.RemoteAddr
	if xff := aussen.Header.Get("X-Forwarded-For"); xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	rec := httptest.NewRecorder()
	s.Intern.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func internFehler(code int, b []byte) error {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(b, &e)
	if e.Error == "" {
		e.Error = strings.TrimSpace(string(b))
	}
	switch code {
	case http.StatusForbidden:
		return fmt.Errorf("nicht erlaubt (%s)", e.Error)
	case http.StatusNotFound:
		return fmt.Errorf("nicht gefunden")
	}
	return fmt.Errorf("Fehler %d: %s", code, e.Error)
}

func (s *Server) mcpWerkzeug(ctx context.Context, r *http.Request, k, name string, roh json.RawMessage) (string, error) {
	var arg struct {
		Query    string `json:"query"`
		SpaceID  string `json:"space_id"`
		ParentID string `json:"parent_id"`
		ID       string `json:"id"`
		Title    string `json:"title"`
		Markdown string `json:"markdown"`
		Mode     string `json:"mode"`
	}
	if len(roh) > 0 {
		if err := json.Unmarshal(roh, &arg); err != nil {
			return "", fmt.Errorf("Argumente unlesbar: %v", err)
		}
	}
	idSicher := func(id string) error {
		if id == "" || strings.ContainsAny(id, "/?#%") {
			return fmt.Errorf("ungültige id")
		}
		return nil
	}
	ok := func(code int) bool { return code >= 200 && code < 300 }

	switch name {
	case "suchen":
		if strings.TrimSpace(arg.Query) == "" {
			return "", fmt.Errorf("query fehlt")
		}
		q := url.Values{"q": {arg.Query}}
		if arg.SpaceID != "" {
			q.Set("space", arg.SpaceID)
		}
		code, b := s.intern(ctx, r, k, "GET", "/api/search?"+q.Encode(), nil)
		if !ok(code) {
			return "", internFehler(code, b)
		}
		return kuerzeJSON(b, 30), nil

	case "ablagen":
		code, b := s.intern(ctx, r, k, "GET", "/api/spaces", nil)
		if !ok(code) {
			return "", internFehler(code, b)
		}
		var liste []map[string]any
		_ = json.Unmarshal(b, &liste)
		raus := []map[string]any{}
		for _, x := range liste {
			raus = append(raus, map[string]any{"id": x["id"], "name": x["name"], "publicAccess": x["publicAccess"]})
		}
		return jsonText(raus), nil

	case "seiten":
		code, b := s.intern(ctx, r, k, "GET", "/api/pages", nil)
		if !ok(code) {
			return "", internFehler(code, b)
		}
		var liste []map[string]any
		_ = json.Unmarshal(b, &liste)
		raus := []map[string]any{}
		for _, x := range liste {
			if arg.SpaceID != "" && fmt.Sprint(x["spaceId"]) != arg.SpaceID {
				continue
			}
			raus = append(raus, map[string]any{"id": x["id"], "title": x["title"], "parentId": x["parentId"],
				"spaceId": x["spaceId"], "updatedAt": x["updatedAt"]})
			if len(raus) >= 500 {
				break
			}
		}
		return jsonText(raus), nil

	case "seite_lesen":
		if err := idSicher(arg.ID); err != nil {
			return "", err
		}
		code, b := s.intern(ctx, r, k, "GET", "/api/pages/"+arg.ID+"/markdown", nil)
		if !ok(code) {
			return "", internFehler(code, b)
		}
		return string(b), nil

	case "seite_anlegen":
		if strings.TrimSpace(arg.Title) == "" {
			return "", fmt.Errorf("title fehlt")
		}
		neu := map[string]any{"title": arg.Title}
		if arg.SpaceID != "" {
			neu["spaceId"] = arg.SpaceID
		}
		if arg.ParentID != "" {
			neu["parentId"] = arg.ParentID
		}
		code, b := s.intern(ctx, r, k, "POST", "/api/pages", neu)
		if !ok(code) {
			return "", internFehler(code, b)
		}
		var seite struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(b, &seite)
		if strings.TrimSpace(arg.Markdown) != "" {
			code, b = s.intern(ctx, r, k, "PUT", "/api/pages/"+seite.ID,
				map[string]any{"content": bloeckeAus(arg.Markdown, arg.Title)})
			if !ok(code) {
				return "", internFehler(code, b)
			}
		}
		return jsonText(map[string]any{"id": seite.ID, "title": arg.Title, "created": true}), nil

	case "seite_aendern":
		if err := idSicher(arg.ID); err != nil {
			return "", err
		}
		patch := map[string]any{}
		if arg.Title != "" {
			patch["title"] = arg.Title
		}
		if arg.Markdown != "" {
			bloecke := bloeckeAus(arg.Markdown, "")
			if arg.Mode == "append" {
				code, b := s.intern(ctx, r, k, "GET", "/api/pages/"+arg.ID, nil)
				if !ok(code) {
					return "", internFehler(code, b)
				}
				var alt struct {
					Content []json.RawMessage `json:"content"`
				}
				_ = json.Unmarshal(b, &alt)
				neu := make([]any, 0, len(alt.Content)+len(bloecke))
				for _, c := range alt.Content {
					neu = append(neu, c)
				}
				for _, c := range bloecke {
					neu = append(neu, c)
				}
				patch["content"] = neu
			} else {
				patch["content"] = bloecke
			}
		}
		if len(patch) == 0 {
			return "", fmt.Errorf("nichts zu ändern: title oder markdown angeben")
		}
		code, b := s.intern(ctx, r, k, "PUT", "/api/pages/"+arg.ID, patch)
		if !ok(code) {
			return "", internFehler(code, b)
		}
		return jsonText(map[string]any{"id": arg.ID, "changed": true}), nil
	}
	return "", fmt.Errorf("unbekanntes Werkzeug %q", name)
}

// bloeckeAus turns Markdown into editor blocks. A first heading equal to the
// title is dropped: the title already stands above the page.
func bloeckeAus(md, titel string) []einlesen.Block {
	t, _, bloecke := einlesen.Lies(md)
	_ = t
	if titel != "" && len(bloecke) > 0 && bloecke[0].Type == "heading" {
		if strings.EqualFold(strings.TrimSpace(rohBlockText(bloecke[0])), strings.TrimSpace(titel)) {
			bloecke = bloecke[1:]
		}
	}
	if bloecke == nil {
		bloecke = []einlesen.Block{}
	}
	return bloecke
}

func rohBlockText(b einlesen.Block) string {
	roh, _ := json.Marshal(b.Content)
	return rohText(roh)
}

func jsonText(v any) string {
	b, _ := json.MarshalIndent(v, "", " ")
	return string(b)
}

// kuerzeJSON keeps at most n entries of a JSON list.
func kuerzeJSON(b []byte, n int) string {
	var liste []json.RawMessage
	if json.Unmarshal(b, &liste) != nil {
		return string(b)
	}
	if len(liste) > n {
		liste = liste[:n]
	}
	return jsonText(liste)
}
