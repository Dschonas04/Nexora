// Webhooks: tell another service when something happens here.
//
// An admin enters an address, n8n for instance, and picks the events. Every
// delivery is a POST with a JSON body and a signature over that body:
//
//	X-Nexora-Event:     page.updated
//	X-Nexora-Delivery:  a fresh id per delivery
//	X-Nexora-Signature: sha256=<hex HMAC-SHA256 of the body with the secret>
//
// The autosave writes every half second while somebody types. page.updated is
// therefore held back until a page has been quiet for a while, and then sent
// once; a hook that fired on every keystroke would be useless to whoever
// receives it.
package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

// The events a hook can ask for.
const (
	EreignisSeiteAngelegt  = "page.created"
	EreignisSeiteGeaendert = "page.updated"
	EreignisSeiteGeloescht = "page.deleted"
	EreignisSeiteWieder    = "page.restored"
	EreignisKommentar      = "comment.created"
	EreignisTest           = "ping"
)

var alleEreignisse = []string{EreignisSeiteAngelegt, EreignisSeiteGeaendert,
	EreignisSeiteGeloescht, EreignisSeiteWieder, EreignisKommentar}

// ruheZeit is how long a page has to stay unchanged before page.updated goes
// out. Long enough to cover a pause for thought while typing, short enough
// that a workflow does not feel stuck.
var ruheZeit = 30 * time.Second

// Webhook is the shape the settings page sees. The secret is shown once, when
// the hook is made; afterwards only its first characters.
type Webhook struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	Ereignisse    []string   `json:"events"`
	Aktiv         bool       `json:"active"`
	Geheimnis     string     `json:"secret,omitempty"`
	GeheimAnfang  string     `json:"secretHint"`
	ErstelltAm    time.Time  `json:"createdAt"`
	ZuletztAm     *time.Time `json:"lastAt"`
	ZuletztStatus int        `json:"lastStatus"`
	ZuletztFehler string     `json:"lastError"`
}

func (s *Server) nurAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "admin only")
		return false
	}
	return true
}

// ListWebhooks returns every hook of the instance.
func (s *Server) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	if !s.nurAdmin(w, r) {
		return
	}
	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, name, url, ereignisse, aktiv, left(geheimnis, 10), erstellt_am,
		        zuletzt_am, zuletzt_status, zuletzt_fehler
		 FROM webhooks ORDER BY erstellt_am`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	liste := []Webhook{}
	for rows.Next() {
		var h Webhook
		if rows.Scan(&h.ID, &h.Name, &h.URL, &h.Ereignisse, &h.Aktiv, &h.GeheimAnfang, &h.ErstelltAm,
			&h.ZuletztAm, &h.ZuletztStatus, &h.ZuletztFehler) == nil {
			h.GeheimAnfang += "…"
			liste = append(liste, h)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"hooks": liste, "events": alleEreignisse})
}

type webhookReq struct {
	Name       *string  `json:"name"`
	URL        *string  `json:"url"`
	Ereignisse []string `json:"events"`
	Aktiv      *bool    `json:"active"`
}

// zielGueltig accepts http and https addresses with a host, nothing else.
func zielGueltig(roh string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(roh))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return u.String(), true
}

// ereignisseBereinigt drops unknown names; an empty list means "all".
func ereignisseBereinigt(liste []string) []string {
	raus := []string{}
	for _, e := range liste {
		for _, g := range alleEreignisse {
			if e == g {
				raus = append(raus, e)
				break
			}
		}
	}
	return raus
}

// WebhookAnlegen makes a hook and answers with its secret, the only time it is
// shown in full.
func (s *Server) WebhookAnlegen(w http.ResponseWriter, r *http.Request) {
	if !s.nurAdmin(w, r) {
		return
	}
	var req webhookReq
	if err := decode(r, &req); err != nil || req.URL == nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	ziel, ok := zielGueltig(*req.URL)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Die Adresse muss mit http:// oder https:// beginnen.")
		return
	}
	name := ""
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	roh := make([]byte, 24)
	if _, err := rand.Read(roh); err != nil {
		writeErr(w, http.StatusInternalServerError, "no randomness")
		return
	}
	geheim := "whsec_" + hex.EncodeToString(roh)
	var h Webhook
	err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO webhooks (name, url, geheimnis, ereignisse) VALUES ($1, $2, $3, $4)
		 RETURNING id, name, url, ereignisse, aktiv, erstellt_am`,
		name, ziel, geheim, ereignisseBereinigt(req.Ereignisse)).
		Scan(&h.ID, &h.Name, &h.URL, &h.Ereignisse, &h.Aktiv, &h.ErstelltAm)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Webhook konnte nicht gespeichert werden")
		return
	}
	h.Geheimnis = geheim
	h.GeheimAnfang = geheim[:10] + "…"
	s.spurAusRequest(r, AktWebhookAngelegt, "webhook", h.ID, ziel, nil)
	writeJSON(w, http.StatusCreated, h)
}

// WebhookAendern changes name, address, events or switches a hook on and off.
func (s *Server) WebhookAendern(w http.ResponseWriter, r *http.Request) {
	if !s.nurAdmin(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	var req webhookReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	var ziel *string
	if req.URL != nil {
		z, ok := zielGueltig(*req.URL)
		if !ok {
			writeErr(w, http.StatusBadRequest, "Die Adresse muss mit http:// oder https:// beginnen.")
			return
		}
		ziel = &z
	}
	var ereignisse []string
	if req.Ereignisse != nil {
		ereignisse = ereignisseBereinigt(req.Ereignisse)
	}
	tag, err := s.Pool.Exec(r.Context(),
		`UPDATE webhooks SET name = coalesce($2, name), url = coalesce($3, url),
		        ereignisse = coalesce($4, ereignisse), aktiv = coalesce($5, aktiv)
		 WHERE id = $1`, id, req.Name, ziel, ereignisse, req.Aktiv)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "Webhook nicht gefunden")
		return
	}
	s.spurAusRequest(r, AktWebhookGeaendert, "webhook", id, "", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// WebhookLoeschen removes a hook.
func (s *Server) WebhookLoeschen(w http.ResponseWriter, r *http.Request) {
	if !s.nurAdmin(w, r) {
		return
	}
	var ziel string
	if err := s.Pool.QueryRow(r.Context(),
		`DELETE FROM webhooks WHERE id=$1 RETURNING url`, chi.URLParam(r, "id")).Scan(&ziel); err != nil {
		writeErr(w, http.StatusNotFound, "Webhook nicht gefunden")
		return
	}
	s.spurAusRequest(r, AktWebhookEntfernt, "webhook", chi.URLParam(r, "id"), ziel, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// WebhookTesten sends a "ping" right away and answers with what came back, so
// the settings page can say whether the address is right.
func (s *Server) WebhookTesten(w http.ResponseWriter, r *http.Request) {
	if !s.nurAdmin(w, r) {
		return
	}
	var ziel, geheim string
	id := chi.URLParam(r, "id")
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT url, geheimnis FROM webhooks WHERE id=$1`, id).Scan(&ziel, &geheim); err != nil {
		writeErr(w, http.StatusNotFound, "Webhook nicht gefunden")
		return
	}
	uid := middleware.UserID(r)
	var name string
	_ = s.Pool.QueryRow(r.Context(), `SELECT name FROM users WHERE id=$1`, uid).Scan(&name)
	koerper := s.webhookKoerper(EreignisTest, map[string]interface{}{
		"actor": map[string]string{"id": uid, "name": name},
	})
	status, fehler := s.zustellenAnHook(r.Context(), id, ziel, geheim, EreignisTest, koerper)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": status, "error": fehler})
}

// webhookKoerper wraps the event's own fields with what every delivery has.
func (s *Server) webhookKoerper(ereignis string, felder map[string]interface{}) []byte {
	felder["event"] = ereignis
	felder["timestamp"] = time.Now().UTC().Format(time.RFC3339)
	if s.SSO.OeffentlicheURL != "" {
		felder["instance"] = strings.TrimRight(s.SSO.OeffentlicheURL, "/")
	}
	b, _ := json.Marshal(felder)
	return b
}

// zustellenAnHook posts one delivery and notes the outcome on the hook.
func (s *Server) zustellenAnHook(ctx context.Context, id, ziel, geheim, ereignis string, koerper []byte) (int, string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ziel, bytes.NewReader(koerper))
	if err != nil {
		return 0, err.Error()
	}
	mac := hmac.New(sha256.New, []byte(geheim))
	mac.Write(koerper)
	kennung := make([]byte, 8)
	_, _ = rand.Read(kennung)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Nexora-Webhook/1")
	req.Header.Set("X-Nexora-Event", ereignis)
	req.Header.Set("X-Nexora-Delivery", hex.EncodeToString(kennung))
	req.Header.Set("X-Nexora-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))

	status, fehler := 0, ""
	antwort, err := http.DefaultClient.Do(req)
	if err != nil {
		fehler = err.Error()
	} else {
		status = antwort.StatusCode
		antwort.Body.Close()
		if status >= 300 {
			fehler = fmt.Sprintf("HTTP %d", status)
		}
	}
	if len(fehler) > 300 {
		fehler = fehler[:300]
	}
	_, _ = s.Pool.Exec(context.Background(),
		`UPDATE webhooks SET zuletzt_am=now(), zuletzt_status=$2, zuletzt_fehler=$3 WHERE id=$1`,
		id, status, fehler)
	return status, fehler
}

// versenden delivers an event to every active hook that asked for it. A
// failed delivery is tried once more after a minute; after that it is noted on
// the hook and given up -- a queue that keeps growing for a receiver that is
// gone helps nobody.
func (s *Server) versenden(ereignis string, felder map[string]interface{}) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rows, err := s.Pool.Query(ctx,
			`SELECT id, url, geheimnis FROM webhooks
			 WHERE aktiv AND (cardinality(ereignisse) = 0 OR $1 = ANY(ereignisse))`, ereignis)
		if err != nil {
			return
		}
		type hook struct{ id, ziel, geheim string }
		var hooks []hook
		for rows.Next() {
			var h hook
			if rows.Scan(&h.id, &h.ziel, &h.geheim) == nil {
				hooks = append(hooks, h)
			}
		}
		rows.Close()
		if len(hooks) == 0 {
			return
		}
		koerper := s.webhookKoerper(ereignis, felder)
		for _, h := range hooks {
			go func(h hook) {
				status, fehler := s.zustellenAnHook(context.Background(), h.id, h.ziel, h.geheim, ereignis, koerper)
				if fehler == "" || (status >= 400 && status < 500) {
					return
				}
				time.Sleep(time.Minute)
				if _, f := s.zustellenAnHook(context.Background(), h.id, h.ziel, h.geheim, ereignis, koerper); f != "" {
					log.Printf("Webhook %s (%s): %s", h.id, ereignis, f)
				}
			}(h)
		}
	}()
}

// seitenFelder describes a page and who acted, for the body of a delivery.
func (s *Server) seitenFelder(ctx context.Context, pageID, akteurID string) map[string]interface{} {
	var titel string
	var space *string
	_ = s.Pool.QueryRow(ctx, `SELECT title, space_id::text FROM pages WHERE id=$1`, pageID).Scan(&titel, &space)
	var name string
	_ = s.Pool.QueryRow(ctx, `SELECT name FROM users WHERE id=$1`, akteurID).Scan(&name)
	seite := map[string]interface{}{"id": pageID, "title": titel, "spaceId": space}
	if s.SSO.OeffentlicheURL != "" {
		seite["url"] = strings.TrimRight(s.SSO.OeffentlicheURL, "/") + "/page/" + pageID
	}
	return map[string]interface{}{
		"page":  seite,
		"actor": map[string]string{"id": akteurID, "name": name},
	}
}

// ruhe holds back page.updated per page until the page has been quiet.
var ruhe = struct {
	sync.Mutex
	uhren map[string]*time.Timer
	wer   map[string]string
}{uhren: map[string]*time.Timer{}, wer: map[string]string{}}

// webhookSeite announces something that happened to a page.
func (s *Server) webhookSeite(ereignis, pageID, akteurID string) {
	if ereignis != EreignisSeiteGeaendert {
		felder := s.seitenFelder(context.Background(), pageID, akteurID)
		s.versenden(ereignis, felder)
		return
	}
	ruhe.Lock()
	defer ruhe.Unlock()
	ruhe.wer[pageID] = akteurID
	if u, ok := ruhe.uhren[pageID]; ok {
		u.Reset(ruheZeit)
		return
	}
	ruhe.uhren[pageID] = time.AfterFunc(ruheZeit, func() {
		ruhe.Lock()
		wer := ruhe.wer[pageID]
		delete(ruhe.uhren, pageID)
		delete(ruhe.wer, pageID)
		ruhe.Unlock()
		s.versenden(EreignisSeiteGeaendert, s.seitenFelder(context.Background(), pageID, wer))
	})
}

// webhookKommentar announces a new comment.
func (s *Server) webhookKommentar(pageID, akteurID, kommentarID, text string) {
	felder := s.seitenFelder(context.Background(), pageID, akteurID)
	felder["comment"] = map[string]string{"id": kommentarID, "text": text}
	s.versenden(EreignisKommentar, felder)
}
