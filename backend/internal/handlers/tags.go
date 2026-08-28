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
	// The count travels along. Without it a tag in the sidebar is just a word
	// with a dot in front of it; only the number says whether anything is behind
	// it, and it makes an orphaned tag visible.
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

// SeitenZuTag returns the pages carrying this tag.
//
// Tags belong to an account, the pages behind them not necessarily: one can put
// one's own tag on a shared page. Filtering therefore follows the same rule as
// everywhere else: owner, admin, or an explicit share.
func (s *Server) SeitenZuTag(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	tagID := chi.URLParam(r, "id")

	// The tag itself has to belong to the caller. Otherwise a stranger's id
	// could be used to explore how other people organise their pages.
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
// kennungMuster is the shape of a UUID as Postgres writes it.
var kennungMuster = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func istKennung(s string) bool { return kennungMuster.MatchString(s) }

func (s *Server) Search(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []models.SearchHit{})
		return
	}

	// The filters arrive as parameters rather than as a query language in the
	// field. Whoever has to type "space:technik" types it wrong; whoever opens a
	// list also sees what there is in the first place.
	//
	// Every filter is a condition of its own with a placeholder of its own.
	// Writing them into the string would be shorter, and it would be the spot
	// where one day an input ends up inside the query.
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
		// The shape is checked here rather than left to the database: an id that
		// is none aborts the whole query otherwise, and the search answers with a
		// 500 instead of "not like that".
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
	// A span in days rather than two date fields: people search for "what was
	// last week", not for "what was between the 3rd and the 9th".
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
	// Two sources, one result: the page itself and its attachments.
	//
	// A hit inside an attachment appears as a hit on the page it hangs from.
	// Anything else would be a dead row for the searcher, because a file without
	// its page is not a place one can go to. Where the hit came from is noted
	// beside it.
	//
	// The rank of an attachment hit is damped: a page containing the term itself
	// is almost always the better answer than one where it appears somewhere in
	// an attachment.
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

	// The admin status is resolved once and passed as a parameter instead of
	// pulling the users table into the search query, which would otherwise keep
	// the query plan from using the GIN index for no good reason.
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
	// DISTINCT ON forces sorting by id; the ranking is restored afterwards. At
	// fifty rows that is cheaper than another query level.
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
	Breite    string          `json:"breite"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// GetPublicPage serves a page to anyone holding its token. This is the only
// read endpoint outside the auth middleware. is_public is checked as well as
// the token, so revoking a link takes effect even if the old token were reused.
func (s *Server) GetPublicPage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	var p publicPage
	var seitenID string
	var content []byte
	err := s.Pool.QueryRow(r.Context(),
		`SELECT id::text, title, content, icon, breite, updated_at FROM pages
		 WHERE public_token=$1 AND is_public=true AND deleted_at IS NULL`, token).
		Scan(&seitenID, &p.Title, &content, &p.Icon, &p.Breite, &p.UpdatedAt)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	// Bilder und Anhaenge auf den oeffentlichen Weg umschreiben, sonst zeigt die
	// Seite dem Besucher lauter zerbrochene Bilder.
	p.Content = adressenOeffnen(json.RawMessage(content), seitenID, token)
	writeJSON(w, http.StatusOK, p)
}
