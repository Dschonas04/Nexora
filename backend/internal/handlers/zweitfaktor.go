// The second factor: setting it up, switching it off, and the second step at
// sign-in.
//
// The scheme itself lives in auth/zweitfaktor.go. What lives here is when
// which row changes, and the one rule it all turns on: no session comes into
// being between password and code. Whoever holds only the password gets a
// ticket that is good for five minutes and opens nothing.
package handlers

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	qrcode "github.com/skip2/go-qrcode"

	"nexora/internal/auth"
	"nexora/internal/middleware"
	"nexora/internal/models"
)

// ErsatzcodeAnzahl is the length of the list created when switching on. Ten is
// enough for the cases it exists for, and few enough that one writes them out
// by hand instead of dropping them into a file.
const ErsatzcodeAnzahl = 10

type zweitStand struct {
	Aktiv bool `json:"aktiv"`
	// Seit is a string and not a time because the interface only shows a date,
	// and an empty field would otherwise appear as 1 January of year one.
	Seit string `json:"seit,omitempty"`
	// Offen is the number of recovery codes not yet spent. It shows in the
	// interface so that nobody first notices none are left at the moment they
	// need one.
	Offen   int  `json:"offen"`
	Pflicht bool `json:"pflicht"`
}

// zweitfaktorAktiv says whether an account demands the second step.
func (s *Server) zweitfaktorAktiv(ctx context.Context, userID string) bool {
	var geheim string
	var seit *time.Time
	if s.Pool.QueryRow(ctx, `SELECT totp_geheim, totp_seit FROM users WHERE id=$1`, userID).
		Scan(&geheim, &seit) != nil {
		return false
	}
	return geheim != "" && seit != nil
}

func (s *Server) offeneErsatzcodes(ctx context.Context, userID string) int {
	var n int
	_ = s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM zweitfaktor_codes WHERE user_id=$1 AND benutzt_am IS NULL`,
		userID).Scan(&n)
	return n
}

// ZweitfaktorStand answers the question the settings page asks when it opens:
// is the second factor in place, since when, and how many recovery codes are
// left.
func (s *Server) ZweitfaktorStand(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var geheim string
	var seit *time.Time
	if s.Pool.QueryRow(r.Context(), `SELECT totp_geheim, totp_seit FROM users WHERE id=$1`, uid).
		Scan(&geheim, &seit) != nil {
		writeErr(w, http.StatusInternalServerError, "Konto nicht lesbar")
		return
	}
	st := zweitStand{Aktiv: geheim != "" && seit != nil, Pflicht: ZweitfaktorPflicht()}
	if st.Aktiv {
		st.Seit = seit.Format(time.RFC3339)
		st.Offen = s.offeneErsatzcodes(r.Context(), uid)
	}
	writeJSON(w, http.StatusOK, st)
}

type zweitStartAntwort struct {
	Geheim string `json:"geheim"`
	URI    string `json:"uri"`
	// QR is a finished SVG. The server draws it because the frontend would
	// otherwise have to carry a library around for a single screen, and as SVG
	// rather than PNG because it then stays sharp at any size.
	QR string `json:"qr"`
}

// ZweitfaktorStart creates a secret and returns it together with the QR code.
// It only becomes valid through ZweitfaktorAn.
//
// A second factor already switched on is not touched: whoever wants to renew it
// switches it off first. Otherwise a hijacked session would be enough to pull
// another account's secret onto one's own phone.
func (s *Server) ZweitfaktorStart(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if s.zweitfaktorAktiv(r.Context(), uid) {
		writeErr(w, http.StatusConflict, "Der zweite Faktor steht bereits. Schalte ihn erst ab.")
		return
	}
	var email, name string
	if s.Pool.QueryRow(r.Context(), `SELECT email, name FROM users WHERE id=$1`, uid).
		Scan(&email, &name) != nil {
		writeErr(w, http.StatusInternalServerError, "Konto nicht lesbar")
		return
	}

	geheim, err := auth.NeuesGeheimnis()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Geheimnis konnte nicht erzeugt werden")
		return
	}
	abgelegt, err := auth.GeheimVerschluesseln(s.Secret, geheim)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Geheimnis konnte nicht abgelegt werden")
		return
	}
	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE users SET totp_geheim=$2, totp_seit=NULL WHERE id=$1`, uid, abgelegt); err != nil {
		writeErr(w, http.StatusInternalServerError, "Geheimnis konnte nicht abgelegt werden")
		return
	}

	uri := auth.OtpauthURI(ZweitfaktorAussteller(), email, geheim)
	// Medium error correction: the code is photographed off a screen, not off
	// printed paper, and thus stays small enough to be read without zooming.
	q, err := qrcode.New(uri, qrcode.Medium)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "QR-Code konnte nicht gezeichnet werden")
		return
	}
	writeJSON(w, http.StatusOK, zweitStartAntwort{Geheim: geheim, URI: uri, QR: qrSVG(q)})
}

// qrSVG draws the matrix as SVG: one path made of squares on a white ground. A
// QR code has to stay dark on light, even in the dark base tone -- which is why
// the white area is fixed and does not come from the colour tokens.
func qrSVG(q *qrcode.QRCode) string {
	m := q.Bitmap()
	n := len(m)
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 `)
	b.WriteString(itoa(n))
	b.WriteString(" ")
	b.WriteString(itoa(n))
	b.WriteString(`" shape-rendering="crispEdges"><rect width="100%" height="100%" fill="#ffffff"/><path fill="#000000" d="`)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if m[y][x] {
				b.WriteString("M")
				b.WriteString(itoa(x))
				b.WriteString(" ")
				b.WriteString(itoa(y))
				b.WriteString("h1v1h-1z")
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var ziffern [8]byte
	i := len(ziffern)
	for n > 0 {
		i--
		ziffern[i] = byte('0' + n%10)
		n /= 10
	}
	return string(ziffern[i:])
}

type zweitCodeReq struct {
	Code string `json:"code"`
}

// ZweitfaktorAn switches on what ZweitfaktorStart prepared and returns the
// recovery codes. They appear in exactly one response and never again -- only
// their hashes sit in the database.
func (s *Server) ZweitfaktorAn(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req zweitCodeReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	var abgelegt string
	var seit *time.Time
	if s.Pool.QueryRow(r.Context(), `SELECT totp_geheim, totp_seit FROM users WHERE id=$1`, uid).
		Scan(&abgelegt, &seit) != nil || abgelegt == "" {
		writeErr(w, http.StatusBadRequest, "Es ist nichts eingerichtet.")
		return
	}
	if seit != nil {
		writeErr(w, http.StatusConflict, "Der zweite Faktor steht bereits.")
		return
	}
	geheim, err := auth.GeheimEntschluesseln(s.Secret, abgelegt)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Geheimnis unbrauchbar. Richte es neu ein.")
		return
	}
	if !auth.TOTPPruefen(geheim, req.Code) {
		writeErr(w, http.StatusUnauthorized, "Der Code stimmt nicht. Achte darauf, dass die Uhr des Telefons richtig geht.")
		return
	}

	codes, err := s.ersatzcodesNeu(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Ersatzcodes konnten nicht angelegt werden")
		return
	}
	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE users SET totp_seit=now() WHERE id=$1`, uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht eingeschaltet werden")
		return
	}
	s.spurAusRequest(r, AktZweitfaktorAn, "konto", uid, "", nil)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ersatzcodes": codes})
}

type zweitAusReq struct {
	Passwort string `json:"passwort"`
}

// ZweitfaktorAus switches off, against one's own password. Without that
// question an unattended screen would be enough to remove the second factor --
// and thus exactly what it stands against.
func (s *Server) ZweitfaktorAus(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req zweitAusReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	// The mandatory check comes before the password check: it costs nothing,
	// and computing bcrypt at cost 12 for an answer that reads 403 anyway would
	// be time thrown away.
	if ZweitfaktorPflicht() && !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "Der zweite Faktor ist für diese Instanz vorgeschrieben.")
		return
	}
	if !s.passwortStimmt(r.Context(), uid, req.Passwort) {
		writeErr(w, http.StatusUnauthorized, "Das Passwort stimmt nicht.")
		return
	}
	if err := s.zweitfaktorLoeschen(r.Context(), uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht abgeschaltet werden")
		return
	}
	s.spurAusRequest(r, AktZweitfaktorAus, "konto", uid, "", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ZweitfaktorCodesNeu replaces the recovery codes. The old ones stop working,
// unused ones included -- a list you do not know who has seen is the very
// reason for being here.
func (s *Server) ZweitfaktorCodesNeu(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req zweitAusReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !s.passwortStimmt(r.Context(), uid, req.Passwort) {
		writeErr(w, http.StatusUnauthorized, "Das Passwort stimmt nicht.")
		return
	}
	if !s.zweitfaktorAktiv(r.Context(), uid) {
		writeErr(w, http.StatusBadRequest, "Der zweite Faktor steht nicht.")
		return
	}
	codes, err := s.ersatzcodesNeu(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Ersatzcodes konnten nicht angelegt werden")
		return
	}
	s.spurAusRequest(r, AktZweitfaktorCodes, "konto", uid, "", nil)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ersatzcodes": codes})
}

// ZweitfaktorZuruecksetzen is the administrator's route for the case that
// really happens: the phone is gone and the recovery codes were on it. The
// account then signs in with the password alone again and has to set the second
// factor up anew.
func (s *Server) ZweitfaktorZuruecksetzen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "admin only")
		return
	}
	ziel := chi.URLParam(r, "id")
	var mail string
	if s.Pool.QueryRow(r.Context(), `SELECT email FROM users WHERE id=$1`, ziel).Scan(&mail) != nil {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	if err := s.zweitfaktorLoeschen(r.Context(), ziel); err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht zurückgesetzt werden")
		return
	}
	s.spurAusRequest(r, AktZweitfaktorReset, "konto", ziel, mail, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// The brake on the second step
//
// A code has six digits, and three windows are valid at once -- that is three
// hits in a million per attempt. Without a brake, somebody holding a valid
// ticket and a fast line would not need a whole night for it. After eight
// failures the account is shut out of the second step for ten minutes; the
// first step stays untouched by that, otherwise the brake would be a means of
// locking other people out.
//
// In memory and not in the database: the lock is meant to slow an attack down,
// not to outlive it, and a restart of the service is the greater interruption
// anyway.
const (
	zweitVersucheMax = 8
	zweitSperrdauer  = 10 * time.Minute
)

var zweitBremse = struct {
	sync.Mutex
	stand map[string]*zweitZaehler
}{stand: map[string]*zweitZaehler{}}

type zweitZaehler struct {
	fehler int
	bis    time.Time
}

// zweitGesperrt says whether the account is currently shut out, and clears
// expired entries while it is there.
func zweitGesperrt(userID string) bool {
	zweitBremse.Lock()
	defer zweitBremse.Unlock()
	z := zweitBremse.stand[userID]
	if z == nil {
		return false
	}
	if time.Now().After(z.bis) {
		delete(zweitBremse.stand, userID)
		return false
	}
	return z.fehler >= zweitVersucheMax
}

func zweitFehlschlag(userID string) {
	zweitBremse.Lock()
	defer zweitBremse.Unlock()
	z := zweitBremse.stand[userID]
	if z == nil || time.Now().After(z.bis) {
		z = &zweitZaehler{}
		zweitBremse.stand[userID] = z
	}
	z.fehler++
	z.bis = time.Now().Add(zweitSperrdauer)
}

func zweitFreigeben(userID string) {
	zweitBremse.Lock()
	defer zweitBremse.Unlock()
	delete(zweitBremse.stand, userID)
}

type zweitPruefReq struct {
	Ticket string `json:"ticket"`
	Code   string `json:"code"`
}

// ZweitfaktorPruefen is the second step of signing in. It is public, because
// at this point no session exists yet; the caller identifies itself with the
// ticket from the first step.
//
// Accepted is the current code from the app or one of the recovery codes. A
// spent recovery code stops working.
func (s *Server) ZweitfaktorPruefen(w http.ResponseWriter, r *http.Request) {
	var req zweitPruefReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	uid, err := auth.ZweitTicketLesen(s.Secret, req.Ticket)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	if zweitGesperrt(uid) {
		writeErr(w, http.StatusTooManyRequests,
			"Zu viele Fehlversuche. Warte zehn Minuten und melde dich neu an.")
		return
	}

	var u models.User
	var abgelegt string
	var seit *time.Time
	if s.Pool.QueryRow(r.Context(),
		`SELECT id, email, name, coalesce(benutzername, ''), role, created_at, totp_geheim, totp_seit
		   FROM users WHERE id=$1`, uid,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Benutzername, &u.Role, &u.CreatedAt, &abgelegt, &seit) != nil {
		writeErr(w, http.StatusUnauthorized, "unbekanntes Konto")
		return
	}
	// Between the two steps an administrator may have reset the second factor.
	// The password has already been checked by then and the way is open -- an
	// error message here would only be a dead end.
	if abgelegt == "" || seit == nil {
		s.issueSession(w, r, u.ID)
		s.anmeldeSpur(r, WegPasswort, u.Email, "", &u)
		writeJSON(w, http.StatusOK, u)
		return
	}

	code := strings.TrimSpace(req.Code)
	gut := false
	if geheim, err := auth.GeheimEntschluesseln(s.Secret, abgelegt); err == nil {
		gut = auth.TOTPPruefen(geheim, code)
	}
	if !gut {
		gut = s.ersatzcodeVerbrauchen(r.Context(), u.ID, code)
	}
	if !gut {
		zweitFehlschlag(u.ID)
		s.anmeldeSpur(r, WegPasswort, u.Email, GrundZweitCode, nil)
		writeErr(w, http.StatusUnauthorized, "Der Code stimmt nicht.")
		return
	}

	zweitFreigeben(u.ID)
	s.issueSession(w, r, u.ID)
	s.anmeldeSpur(r, WegPasswort, u.Email, "", &u)
	writeJSON(w, http.StatusOK, u)
}

// ersatzcodesNeu throws the old list away and creates a new one. What comes
// back are the codes in the clear -- the only time they exist that way.
func (s *Server) ersatzcodesNeu(ctx context.Context, userID string) ([]string, error) {
	if _, err := s.Pool.Exec(ctx, `DELETE FROM zweitfaktor_codes WHERE user_id=$1`, userID); err != nil {
		return nil, err
	}
	codes := make([]string, 0, ErsatzcodeAnzahl)
	for i := 0; i < ErsatzcodeAnzahl; i++ {
		c, err := auth.NeuerErsatzcode()
		if err != nil {
			return nil, err
		}
		hash, err := auth.HashPassword(c)
		if err != nil {
			return nil, err
		}
		if _, err := s.Pool.Exec(ctx,
			`INSERT INTO zweitfaktor_codes (user_id, hash) VALUES ($1, $2)`, userID, hash); err != nil {
			return nil, err
		}
		codes = append(codes, c)
	}
	return codes, nil
}

// ersatzcodeVerbrauchen checks the input against the open codes and strikes
// out the one that matches. It compares against each of them in turn because
// bcrypt hashes cannot be looked up -- with ten rows that is no price at all.
func (s *Server) ersatzcodeVerbrauchen(ctx context.Context, userID, eingabe string) bool {
	eingabe = strings.ToLower(strings.TrimSpace(eingabe))
	if eingabe == "" {
		return false
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id, hash FROM zweitfaktor_codes WHERE user_id=$1 AND benutzt_am IS NULL`, userID)
	if err != nil {
		return false
	}
	type eintrag struct{ id, hash string }
	var offen []eintrag
	for rows.Next() {
		var e eintrag
		if rows.Scan(&e.id, &e.hash) == nil {
			offen = append(offen, e)
		}
	}
	rows.Close()

	for _, e := range offen {
		if auth.CheckPassword(e.hash, eingabe) {
			// Only strike it out while the row is still open: two simultaneous
			// sign-ins with the same code must not both get through.
			tag, err := s.Pool.Exec(ctx,
				`UPDATE zweitfaktor_codes SET benutzt_am=now() WHERE id=$1 AND benutzt_am IS NULL`, e.id)
			return err == nil && tag.RowsAffected() == 1
		}
	}
	return false
}

func (s *Server) zweitfaktorLoeschen(ctx context.Context, userID string) error {
	if _, err := s.Pool.Exec(ctx,
		`UPDATE users SET totp_geheim='', totp_seit=NULL WHERE id=$1`, userID); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `DELETE FROM zweitfaktor_codes WHERE user_id=$1`, userID)
	return err
}

func (s *Server) passwortStimmt(ctx context.Context, userID, passwort string) bool {
	var hash string
	if s.Pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, userID).Scan(&hash) != nil {
		return false
	}
	return hash != "" && auth.CheckPassword(hash, passwort)
}
