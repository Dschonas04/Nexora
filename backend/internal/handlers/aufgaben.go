// Tasks: the checklist items of all pages, gathered in one list.
//
// A task is an ordinary checklist item. Two marks in its text make it more:
//
//	@username       who it is for (any number of them)
//	📅 2026-10-30   when it is due (also 📅 30.10.2026)
//
// Nothing else is stored: the page is the task list, and "My tasks" is a view
// over all pages the account may read. Marks in plain text survive every way a
// page travels -- Markdown export and import, copy and paste, a template --
// where a special block type would be lost on the first round trip.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/lizenz"
	"nexora/internal/middleware"
)

// Aufgabe is one checklist item with what its text says about it.
type Aufgabe struct {
	PageID    string   `json:"pageId"`
	PageTitle string   `json:"pageTitle"`
	BlockID   string   `json:"blockId"`
	Text      string   `json:"text"`
	Erledigt  bool     `json:"checked"`
	Faellig   string   `json:"due,omitempty"` // YYYY-MM-DD
	Personen  []string `json:"assignees"`
	Mir       bool     `json:"mine"`
	CanEdit   bool     `json:"canEdit"`
}

var (
	faelligISO  = regexp.MustCompile(`📅\s*(\d{4})-(\d{2})-(\d{2})`)
	faelligDE   = regexp.MustCompile(`📅\s*(\d{1,2})\.(\d{1,2})\.(\d{4})`)
	personMarke = regexp.MustCompile(`(?:^|[\s(\[,;])@([A-Za-z0-9._-]{3,32})`)
	maxAufgaben = 500
)

// faelligAus reads the due date from a task's text, as YYYY-MM-DD, or "".
func faelligAus(text string) string {
	if m := faelligISO.FindStringSubmatch(text); m != nil {
		if _, err := time.Parse("2006-01-02", m[1]+"-"+m[2]+"-"+m[3]); err == nil {
			return m[1] + "-" + m[2] + "-" + m[3]
		}
	}
	if m := faelligDE.FindStringSubmatch(text); m != nil {
		t, err := time.Parse("2.1.2006", m[1]+"."+m[2]+"."+m[3])
		if err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

// personenAus reads the @names from a task's text, lower case and each once.
// A trailing full stop belongs to the sentence, not to the name.
func personenAus(text string) []string {
	gesehen := map[string]bool{}
	raus := []string{}
	for _, m := range personMarke.FindAllStringSubmatch(text, -1) {
		n := strings.ToLower(strings.TrimRight(m[1], "."))
		if len(n) >= 3 && !gesehen[n] {
			gesehen[n] = true
			raus = append(raus, n)
		}
	}
	return raus
}

// inlineText joins the visible text of a block's inline content; links carry
// theirs one level down.
func inlineText(inhalt interface{}) string {
	teile, _ := inhalt.([]interface{})
	var b strings.Builder
	for _, t := range teile {
		m, _ := t.(map[string]interface{})
		if m == nil {
			continue
		}
		if s, ok := m["text"].(string); ok {
			b.WriteString(s)
		}
		if innen, ok := m["content"]; ok {
			b.WriteString(inlineText(innen))
		}
	}
	return b.String()
}

// aufgabenAus walks a document and returns its checklist items, nested ones
// included.
func aufgabenAus(inhalt json.RawMessage) []Aufgabe {
	var bloecke []interface{}
	if json.Unmarshal(inhalt, &bloecke) != nil {
		return nil
	}
	var raus []Aufgabe
	var gehe func([]interface{})
	gehe = func(liste []interface{}) {
		for _, b := range liste {
			m, _ := b.(map[string]interface{})
			if m == nil {
				continue
			}
			if m["type"] == "checkListItem" {
				id, _ := m["id"].(string)
				props, _ := m["props"].(map[string]interface{})
				erledigt, _ := props["checked"].(bool)
				text := strings.TrimSpace(inlineText(m["content"]))
				if id != "" && text != "" {
					raus = append(raus, Aufgabe{
						BlockID:  id,
						Text:     text,
						Erledigt: erledigt,
						Faellig:  faelligAus(text),
						Personen: personenAus(text),
					})
				}
			}
			if kinder, ok := m["children"].([]interface{}); ok {
				gehe(kinder)
			}
		}
	}
	gehe(bloecke)
	return raus
}

// ListAufgaben gathers the tasks.
//
//	?filter=mine   (default) tasks carrying the caller's @username
//	?filter=own    every task on pages the caller owns
//	&done=1        include ticked ones
//
// Open tasks come first, ordered by due date, those without one last.
func (s *Server) ListAufgaben(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	filter := r.URL.Query().Get("filter")
	mitErledigten := r.URL.Query().Get("done") != ""

	var name *string
	_ = s.Pool.QueryRow(r.Context(), `SELECT benutzername FROM users WHERE id=$1`, uid).Scan(&name)
	ich := ""
	if name != nil {
		ich = strings.ToLower(*name)
	}

	bedingung := `p.owner_id = $1`
	args := []interface{}{uid}
	if filter != "own" {
		if ich == "" {
			writeJSON(w, http.StatusOK, map[string]interface{}{"tasks": []Aufgabe{}, "username": ""})
			return
		}
		// The text column is the cheap first sieve: only pages naming the
		// account at all are opened and walked.
		args = append(args, s.isAdmin(r.Context(), uid), lizenz.Frei(lizenz.Gruppen), "%@"+ich+"%")
		bedingung = `(p.owner_id = $1 OR $2
		       OR EXISTS (SELECT 1 FROM page_shares sh WHERE sh.page_id = p.id AND sh.user_id = $1)
		       OR ` + spaceZugriffSQL("p.space_id", "$1", "$3") + `)
		  AND lower(p.content_text) LIKE $4`
	}
	rows, err := s.Pool.Query(r.Context(),
		`SELECT p.id, p.title, p.content FROM pages p
		 WHERE p.deleted_at IS NULL AND `+bedingung+`
		   AND p.content::text LIKE '%checkListItem%'
		 ORDER BY p.updated_at DESC LIMIT 400`, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	type seite struct {
		id, titel string
		inhalt    []byte
	}
	var seiten []seite
	for rows.Next() {
		var x seite
		if rows.Scan(&x.id, &x.titel, &x.inhalt) == nil {
			seiten = append(seiten, x)
		}
	}
	rows.Close()

	liste := []Aufgabe{}
	for _, x := range seiten {
		var schreiben, geprueft bool
		for _, a := range aufgabenAus(x.inhalt) {
			if a.Erledigt && !mitErledigten {
				continue
			}
			for _, p := range a.Personen {
				if p == ich {
					a.Mir = true
				}
			}
			if filter != "own" && !a.Mir {
				continue
			}
			if !geprueft {
				_, schreiben, _, _ = s.pagePerm(r.Context(), uid, x.id)
				geprueft = true
			}
			a.PageID, a.PageTitle, a.CanEdit = x.id, x.titel, schreiben
			liste = append(liste, a)
			if len(liste) >= maxAufgaben {
				break
			}
		}
	}
	sort.SliceStable(liste, func(i, j int) bool {
		a, b := liste[i], liste[j]
		if a.Erledigt != b.Erledigt {
			return !a.Erledigt
		}
		if (a.Faellig == "") != (b.Faellig == "") {
			return a.Faellig != ""
		}
		return a.Faellig < b.Faellig
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"tasks": liste, "username": ich})
}

// setzeHaken finds the block in the document and sets its checked prop.
// Reports whether the block was there.
func setzeHaken(inhalt json.RawMessage, blockID string, erledigt bool) (json.RawMessage, bool) {
	var bloecke []interface{}
	if json.Unmarshal(inhalt, &bloecke) != nil {
		return nil, false
	}
	gefunden := false
	var gehe func([]interface{})
	gehe = func(liste []interface{}) {
		for _, b := range liste {
			m, _ := b.(map[string]interface{})
			if m == nil || gefunden {
				continue
			}
			if m["id"] == blockID && m["type"] == "checkListItem" {
				props, _ := m["props"].(map[string]interface{})
				if props == nil {
					props = map[string]interface{}{}
					m["props"] = props
				}
				props["checked"] = erledigt
				gefunden = true
				return
			}
			if kinder, ok := m["children"].([]interface{}); ok {
				gehe(kinder)
			}
		}
	}
	gehe(bloecke)
	if !gefunden {
		return nil, false
	}
	neu, err := json.Marshal(bloecke)
	if err != nil {
		return nil, false
	}
	return neu, true
}

// AufgabeAbhaken ticks or unticks a task from the list, without opening the
// page.
//
// Refused while people are writing on the page together: their browsers hold
// the text and would write their own state over this change a moment later.
// The page is the place to tick it then.
func (s *Server) AufgabeAbhaken(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "pageId")
	_, canEdit, _, ok := s.pagePerm(r.Context(), uid, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	if !canEdit {
		writeErr(w, http.StatusForbidden, "read-only access")
		return
	}
	if ImRaum(id) > 0 {
		writeErr(w, http.StatusConflict, "Auf der Seite wird gerade gemeinsam geschrieben – bitte dort abhaken.")
		return
	}
	var req struct {
		Erledigt bool `json:"checked"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	cur, err := s.loadPage(r.Context(), uid, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	neu, gefunden := setzeHaken(cur.Content, chi.URLParam(r, "blockId"), req.Erledigt)
	if !gefunden {
		writeErr(w, http.StatusNotFound, "Aufgabe nicht gefunden")
		return
	}
	s.snapshotVersion(r.Context(), cur, uid)
	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE pages SET content=$2::jsonb, content_text=$3, updated_at=now() WHERE id=$1`,
		id, string(neu), textAusInhalt(neu)); err != nil {
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}
	s.spurAusRequest(r, AktAufgabeAbgehakt, "seite", id, cur.Title, map[string]interface{}{"checked": req.Erledigt})
	s.beobachterBenachrichtigen(id, uid)
	s.webhookSeite(EreignisSeiteGeaendert, id, uid)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// aufgabenZugewiesen tells everybody newly named in a task of this page. Only
// what is NEW counts: a name that already stood in the task before this save
// was told then, and every autosave of a page with ten tasks would otherwise
// repeat ten messages.
func (s *Server) aufgabenZugewiesen(alt, neu json.RawMessage, pageID, akteurID string) {
	vorher := map[string]bool{}
	for _, a := range aufgabenAus(alt) {
		for _, p := range a.Personen {
			vorher[a.BlockID+"\x00"+p] = true
		}
	}
	type zuweisung struct{ person, text string }
	var neue []zuweisung
	for _, a := range aufgabenAus(neu) {
		if a.Erledigt {
			continue
		}
		for _, p := range a.Personen {
			if !vorher[a.BlockID+"\x00"+p] {
				neue = append(neue, zuweisung{p, a.Text})
			}
		}
	}
	if len(neue) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var name string
		_ = s.Pool.QueryRow(ctx, `SELECT name FROM users WHERE id=$1`, akteurID).Scan(&name)
		titel := s.seitenTitel(ctx, pageID)
		for _, z := range neue {
			var empfaenger string
			if s.Pool.QueryRow(ctx, `SELECT id::text FROM users WHERE lower(benutzername) = $1`, z.person).
				Scan(&empfaenger) != nil {
				continue
			}
			// Whoever may not read the page learns nothing about it, not even
			// that a task there carries their name.
			if _, _, _, ok := s.pagePerm(ctx, empfaenger, pageID); !ok {
				continue
			}
			s.zustellen(ctx, empfaenger, PostAufgabe, pageID, "", akteurID, name, titel, z.text)
		}
	}()
}
