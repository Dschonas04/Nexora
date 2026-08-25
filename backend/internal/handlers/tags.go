// Tags, favorites, search, and the anonymous read view behind a public link.
// These are the small endpoints that did not warrant a file of their own.
package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/lizenz"
	"nexora/internal/middleware"
	"nexora/internal/models"
)

// ListTags returns the caller's tags. Tags are per user, so two people can each
// keep their own vocabulary without seeing the other's.
func (s *Server) ListTags(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	// Die Anzahl kommt mit. Ohne sie ist ein Schlagwort in der Seitenleiste
	// nur ein Wort mit einem Punkt davor, erst die Zahl sagt, ob dahinter
	// etwas steckt, und macht ein verwaistes Schlagwort sichtbar.
	rows, err := s.Pool.Query(r.Context(),
		`SELECT t.id, t.name, t.color,
		        (SELECT count(*) FROM page_tags pt
		         JOIN pages p ON p.id = pt.page_id
		         WHERE pt.tag_id = t.id AND p.deleted_at IS NULL)
		 FROM tags t WHERE t.owner_id=$1 ORDER BY t.name`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	list := []models.Tag{}
	for rows.Next() {
		var t models.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.Anzahl); err == nil {
			list = append(list, t)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// SeitenZuTag liefert die Seiten, die dieses Schlagwort tragen.
//
// Schlagworte gehören einem Konto, die Seiten dahinter nicht zwangsläufig: man
// kann eine geteilte Seite mit einem eigenen Schlagwort versehen. Gefiltert
// wird deshalb nach derselben Regel wie überall, Eigentümer, Admin oder
// ausdrückliche Freigabe.
func (s *Server) SeitenZuTag(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	tagID := chi.URLParam(r, "id")

	// Das Schlagwort selbst muss dem Aufrufer gehören. Sonst ließe sich über
	// eine fremde Kennung erkunden, wie andere ihre Seiten ordnen.
	var gehoert bool
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM tags WHERE id=$1 AND owner_id=$2)`,
		tagID, uid).Scan(&gehoert); err != nil || !gehoert {
		writeErr(w, http.StatusNotFound, "Schlagwort nicht gefunden")
		return
	}

	admin := s.isAdmin(r.Context(), uid)
	rows, err := s.Pool.Query(r.Context(),
		`SELECT p.id, p.parent_id, p.space_id, p.title, p.icon, p.updated_at,
		        (p.owner_id <> $1) AS fremd
		 FROM pages p
		 JOIN page_tags pt ON pt.page_id = p.id
		 WHERE pt.tag_id = $2
		   AND p.deleted_at IS NULL
		   AND (p.owner_id = $1 OR $3
		        OR EXISTS (SELECT 1 FROM page_shares sh
		                   WHERE sh.page_id = p.id AND sh.user_id = $1))
		 ORDER BY p.updated_at DESC`, uid, tagID, admin)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []models.PageMeta{}
	for rows.Next() {
		var p models.PageMeta
		if err := rows.Scan(&p.ID, &p.ParentID, &p.SpaceID, &p.Title, &p.Icon,
			&p.UpdatedAt, &p.Shared); err == nil {
			list = append(list, p)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

type createTagReq struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// CreateTag adds a tag, or recolors it if that name already exists. Treating a
// duplicate as an update rather than an error means the frontend can create a
// tag optimistically without checking first.
func (s *Server) CreateTag(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req createTagReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if req.Color == "" {
		req.Color = "#6b7280"
	}
	var t models.Tag
	err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO tags (owner_id, name, color) VALUES ($1, $2, $3)
		 ON CONFLICT (owner_id, name) DO UPDATE SET color = EXCLUDED.color
		 RETURNING id, name, color`,
		uid, req.Name, req.Color).Scan(&t.ID, &t.Name, &t.Color)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create tag")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// DeleteTag removes a tag. The page_tags rows go with it through the cascade,
// so the tag disappears from every page at once.
func (s *Server) DeleteTag(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	_, err := s.Pool.Exec(r.Context(),
		`DELETE FROM tags WHERE id=$1 AND owner_id=$2`, chi.URLParam(r, "id"), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type attachTagReq struct {
	TagID string `json:"tagId"`
}

// AttachTag puts a tag on a page. Both ownership checks sit inside the INSERT
// ... SELECT, so the row only appears when the caller owns the page and the tag.
// A request that fails those checks writes nothing and still answers 200, since
// from the client's point of view there is nothing to retry.
func (s *Server) AttachTag(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req attachTagReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	_, err := s.Pool.Exec(r.Context(),
		`INSERT INTO page_tags (page_id, tag_id)
		 SELECT $2, $3
		 WHERE EXISTS (SELECT 1 FROM pages WHERE id=$2 AND owner_id=$1)
		   AND EXISTS (SELECT 1 FROM tags  WHERE id=$3 AND owner_id=$1)
		 ON CONFLICT DO NOTHING`,
		uid, chi.URLParam(r, "id"), req.TagID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "attach failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DetachTag removes a tag from a page.
func (s *Server) DetachTag(w http.ResponseWriter, r *http.Request) {
	_, err := s.Pool.Exec(r.Context(),
		`DELETE FROM page_tags WHERE page_id=$1 AND tag_id=$2`,
		chi.URLParam(r, "id"), chi.URLParam(r, "tagId"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "detach failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ListFavorites returns the pages this user has pinned, most recently edited
// first.
func (s *Server) ListFavorites(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	rows, err := s.Pool.Query(r.Context(),
		`SELECT p.id, p.parent_id, p.title, p.icon, p.updated_at FROM pages p
		 JOIN favorites f ON f.page_id = p.id
		 WHERE f.user_id=$1 ORDER BY p.updated_at DESC`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	list := []models.PageMeta{}
	for rows.Next() {
		var p models.PageMeta
		if err := rows.Scan(&p.ID, &p.ParentID, &p.Title, &p.Icon, &p.UpdatedAt); err == nil {
			list = append(list, p)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// Search runs a real full text search over the pages the caller may read.
//
// Two things it gets right that the previous ILIKE version did not.
//
// First, access: the result set is built with the SAME rule pagePerm applies to
// a single page, owner, admin, or an explicit share. A search that reached
// further than the page itself would be a leak, and one that reached less would
// hide a page the user can open by link.
//
// Second, ranking: results come back by relevance, not by modification date. A
// page whose title matches is weighted above one that merely mentions the term,
// which is what setweight in the schema is for.
// kennungMuster ist die Gestalt einer UUID, wie Postgres sie ausgibt.
var kennungMuster = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func istKennung(s string) bool { return kennungMuster.MatchString(s) }

func (s *Server) Search(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []models.SearchHit{})
		return
	}

	// Die Filter kommen als Parameter und nicht als Suchsprache im Feld. Wer
	// "space:technik" tippen muss, tippt es falsch; wer eine Liste aufklappt,
	// sieht auch gleich, was es überhaupt gibt.
	//
	// Jeder Filter ist eine eigene Bedingung mit einem eigenen Platzhalter.
	// Sie in die Zeichenkette zu schreiben wäre kürzer und wäre die Stelle,
	// an der eines Tages eine Eingabe in der Abfrage landet.
	filter := ""
	args := []any{uid, q, false, false, false}
	setz := func(bedingung string, wert any) {
		args = append(args, wert)
		filter += strings.ReplaceAll(bedingung, "$?", "$"+strconv.Itoa(len(args))) + "\n"
	}

	switch space := strings.TrimSpace(r.URL.Query().Get("space")); space {
	case "":
		// no restriction
	case "ohne":
		filter += "AND p.space_id IS NULL\n"
	default:
		// Die Gestalt wird hier geprüft und nicht der Datenbank überlassen:
		// eine Kennung, die keine ist, bricht sonst die ganze Abfrage ab, und
		// die Suche antwortet mit 500 statt mit "so nicht".
		if !istKennung(space) {
			writeErr(w, http.StatusBadRequest, "ungültige Ablage im Filter")
			return
		}
		setz("AND p.space_id = $?::uuid", space)
	}
	if tag := strings.TrimSpace(r.URL.Query().Get("tag")); tag != "" {
		if !istKennung(tag) {
			writeErr(w, http.StatusBadRequest, "ungültiges Schlagwort im Filter")
			return
		}
		setz("AND EXISTS (SELECT 1 FROM page_tags pt WHERE pt.page_id = p.id AND pt.tag_id = $?::uuid)", tag)
	}
	// Ein Zeitraum in Tagen statt zweier Datumsfelder: gesucht wird "was war
	// letzte Woche", nicht "was war zwischen dem 3. und dem 9.".
	if tage, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("tage"))); err == nil && tage > 0 {
		setz("AND p.updated_at > now() - make_interval(days => $?)", tage)
	}
	if strings.TrimSpace(r.URL.Query().Get("wer")) == "ich" {
		filter += "AND p.owner_id = $1\n"
	}

	// websearch_to_tsquery accepts what people actually type: bare words,
	// "quoted phrases", OR, and a leading minus to exclude. Unlike
	// to_tsquery it cannot be made to fail on stray punctuation, so a user
	// typing a colon does not get an error page.
	//
	// ts_headline delivers the snippet with <b> around the hits. The template
	// escapes it before rendering, so this is data, never markup.
	// Zwei Quellen, ein Ergebnis: die Seite selbst und ihre Anhänge.
	//
	// Ein Anhangtreffer erscheint als Treffer der Seite, an der er hängt,
	// alles andere wäre für den Suchenden eine tote Zeile, denn eine Datei ohne
	// ihre Seite ist kein Ort, an den man gehen kann. Woher der Treffer kam,
	// steht daneben.
	//
	// Der Rang eines Anhangtreffers wird gedämpft: eine Seite, die den Begriff
	// selbst enthält, ist fast immer die bessere Antwort als eine, in deren
	// Anhang er irgendwo vorkommt.
	sql := `
		WITH frage AS (SELECT websearch_to_tsquery('german', $2) AS q),
		sichtbar AS (
			SELECT p.* FROM pages p
			WHERE p.deleted_at IS NULL
			  AND (p.owner_id = $1 OR $3
			       OR EXISTS (SELECT 1 FROM page_shares sh
			                  WHERE sh.page_id = p.id AND sh.user_id = $1)
			       OR ` + spaceZugriffSQL("p.space_id", "$1", "$4") + `)
			  ` + filter + `
		),
		treffer AS (
			SELECT p.id, p.parent_id, p.title, p.icon, p.updated_at,
			       ts_headline('german', p.content_text, frage.q,
			         'StartSel=<b>, StopSel=</b>, MaxWords=28, MinWords=12, MaxFragments=1, FragmentDelimiter=" … "') AS ausschnitt,
			       ts_rank_cd(p.such_tsv, frage.q) AS rang,
			       (p.owner_id = $1) AS eigen,
			       '' AS quelle
			FROM sichtbar p, frage
			WHERE p.such_tsv @@ frage.q

			UNION ALL

			SELECT p.id, p.parent_id, p.title, p.icon, p.updated_at,
			       ts_headline('german', a.inhalt_text, frage.q,
			         'StartSel=<b>, StopSel=</b>, MaxWords=28, MinWords=12, MaxFragments=1, FragmentDelimiter=" … "') AS ausschnitt,
			       ts_rank_cd(a.such_tsv, frage.q) * 0.6 AS rang,
			       (p.owner_id = $1) AS eigen,
			       a.filename AS quelle
			FROM attachments a
			JOIN sichtbar p ON p.id = a.page_id, frage
			WHERE $5 AND a.such_tsv @@ frage.q
		)
		SELECT DISTINCT ON (id) id, parent_id, title, icon, updated_at,
		       ausschnitt, rang, eigen, quelle
		FROM treffer
		ORDER BY id, rang DESC
		LIMIT 50`

	// Der Adminstatus wird einmal aufgelöst und als Parameter übergeben, statt
	// die users-Tabelle in die Suchabfrage zu ziehen, das hielte den
	// Abfrageplan sonst unnötig davon ab, den GIN-Index zu benutzen.
	args[2] = s.isAdmin(r.Context(), uid)
	args[3] = lizenz.Frei(lizenz.Gruppen)
	args[4] = lizenz.Frei(lizenz.Anhangsuche)

	rows, err := s.Pool.Query(r.Context(), sql, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()

	list := []models.SearchHit{}
	for rows.Next() {
		var h models.SearchHit
		var rang float32
		if err := rows.Scan(&h.ID, &h.ParentID, &h.Title, &h.Icon, &h.UpdatedAt,
			&h.Ausschnitt, &rang, &h.Eigen, &h.Quelle); err == nil {
			h.Rang = rang
			list = append(list, h)
		}
	}
	// DISTINCT ON zwingt zur Sortierung nach id; die Rangfolge stellen wir
	// danach wieder her. Bei fünfzig Zeilen ist das billiger als eine weitere
	// Abfrageebene.
	sort.SliceStable(list, func(i, j int) bool { return list[i].Rang > list[j].Rang })
	writeJSON(w, http.StatusOK, list)
}

// publicPage is a deliberately narrow view: title, content, icon and the date.
// No owner, no id, no tags, nothing that would leak workspace structure to an
// anonymous visitor.
type publicPage struct {
	Title     string          `json:"title"`
	Content   json.RawMessage `json:"content"`
	Icon      string          `json:"icon"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// GetPublicPage serves a page to anyone holding its token. This is the only
// read endpoint outside the auth middleware. is_public is checked as well as
// the token, so revoking a link takes effect even if the old token were reused.
func (s *Server) GetPublicPage(w http.ResponseWriter, r *http.Request) {
	var p publicPage
	var content []byte
	err := s.Pool.QueryRow(r.Context(),
		`SELECT title, content, icon, updated_at FROM pages
		 WHERE public_token=$1 AND is_public=true`, chi.URLParam(r, "token")).
		Scan(&p.Title, &content, &p.Icon, &p.UpdatedAt)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	p.Content = json.RawMessage(content)
	writeJSON(w, http.StatusOK, p)
}
