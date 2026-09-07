// Der zweite Faktor: Einrichten, Abschalten und der zweite Schritt an der
// Anmeldung.
//
// Das Verfahren selbst steht in auth/zweitfaktor.go. Hier steht, wann was in
// der Datenbank passiert, und die eine Regel, um die es dabei geht: zwischen
// Passwort und Code entsteht keine Sitzung. Wer nur das Passwort hat, bekommt
// ein Ticket, das fünf Minuten gilt und nichts öffnet.
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

// ErsatzcodeAnzahl ist die Länge der Liste, die beim Einschalten entsteht. Zehn
// ist genug für die Fälle, für die sie da ist, und wenig genug, dass man sie
// abschreibt statt sie in eine Datei zu legen.
const ErsatzcodeAnzahl = 10

type zweitStand struct {
	Aktiv bool `json:"aktiv"`
	// Seit steht als Text und nicht als Zeit, weil die Oberfläche nur ein Datum
	// zeigt und ein leeres Feld sonst als 1. Januar Jahr eins erschiene.
	Seit string `json:"seit,omitempty"`
	// Offen ist die Zahl der Ersatzcodes, die noch nicht verbraucht sind. Sie
	// steht in der Oberfläche, damit niemand erst merkt, dass keiner mehr da
	// ist, wenn er einen braucht.
	Offen   int  `json:"offen"`
	Pflicht bool `json:"pflicht"`
}

// zweitfaktorAktiv sagt, ob ein Konto den zweiten Schritt verlangt.
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

// ZweitfaktorStand beantwortet die Frage, die die Einstellungsseite beim Öffnen
// stellt: steht der zweite Faktor, seit wann, wie viele Ersatzcodes sind übrig.
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
	// QR ist ein fertiges SVG. Der Server zeichnet es, weil das Frontend sonst
	// eine Bibliothek für einen einzigen Bildschirm mitschleppen müsste, und als
	// SVG statt als PNG, weil es dann in jeder Größe scharf bleibt.
	QR string `json:"qr"`
}

// ZweitfaktorStart legt ein Geheimnis an und gibt es zusammen mit dem QR-Code
// zurück. Gültig wird es erst durch ZweitfaktorAn.
//
// Ein bereits eingeschalteter zweiter Faktor wird dabei nicht angerührt: wer ihn
// erneuern will, schaltet ihn erst ab. Sonst genügte eine übernommene Sitzung,
// um das Geheimnis eines fremden Kontos auf das eigene Telefon zu holen.
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
	// Fehlerkorrektur auf mittlerer Stufe: der Code wird von einem Bildschirm
	// abfotografiert, nicht von bedrucktem Papier, und bleibt so klein genug,
	// um ohne Zoom lesbar zu sein.
	q, err := qrcode.New(uri, qrcode.Medium)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "QR-Code konnte nicht gezeichnet werden")
		return
	}
	writeJSON(w, http.StatusOK, zweitStartAntwort{Geheim: geheim, URI: uri, QR: qrSVG(q)})
}

// qrSVG zeichnet die Matrix als SVG: ein Pfad aus lauter Quadraten auf weißem
// Grund. Ein QR-Code muss dunkel auf hell bleiben, auch im dunklen Grundton --
// deshalb steht die weiße Fläche fest und kommt nicht aus den Farbmarken.
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

// ZweitfaktorAn schaltet ein, was ZweitfaktorStart vorbereitet hat, und gibt die
// Ersatzcodes zurück. Sie stehen genau einmal in einer Antwort und danach nie
// wieder -- in der Datenbank liegen nur ihre Hashes.
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

// ZweitfaktorAus schaltet ab, gegen das eigene Passwort. Ohne diese Rückfrage
// genügte ein unbeaufsichtigter Bildschirm, um den zweiten Faktor zu entfernen
// -- und damit genau das, wogegen er steht.
func (s *Server) ZweitfaktorAus(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req zweitAusReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	// Die Pflicht steht vor der Passwortpruefung: sie kostet nichts, und bcrypt
	// mit Kostenfaktor 12 fuer eine Antwort zu rechnen, die ohnehin 403 lautet,
	// waere verschenkte Zeit.
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

// ZweitfaktorCodesNeu ersetzt die Ersatzcodes. Die alten gelten danach nicht
// mehr, auch die unbenutzten -- eine Liste, von der man nicht weiß, wer sie
// gesehen hat, ist der Grund, hier zu sein.
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

// ZweitfaktorZuruecksetzen ist der Weg der Verwaltung für den Fall, den es
// wirklich gibt: das Telefon ist weg und die Ersatzcodes liegen darauf. Das
// Konto meldet sich danach wieder allein mit dem Passwort an und muss den
// zweiten Faktor neu einrichten.
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
// Die Bremse am zweiten Schritt
//
// Ein Code hat sechs Stellen, und drei Fenster gelten gleichzeitig -- das sind
// drei Treffer in einer Million je Versuch. Ohne Bremse braeuchte jemand mit
// einem gueltigen Ticket und einer schnellen Leitung dafuer keine Nacht. Nach
// acht Fehlversuchen ist das Konto fuer den zweiten Schritt zehn Minuten dicht;
// der erste Schritt bleibt davon unberuehrt, sonst waere die Bremse ein Mittel,
// fremde Konten auszusperren.
//
// Im Arbeitsspeicher und nicht in der Datenbank: die Sperre soll einen Angriff
// ausbremsen, nicht ihn ueberdauern, und ein Neustart des Dienstes ist ohnehin
// die groessere Unterbrechung.
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

// zweitGesperrt sagt, ob das Konto gerade nicht drankommt, und raeumt dabei
// abgelaufene Eintraege weg.
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

// ZweitfaktorPruefen ist der zweite Schritt der Anmeldung. Er steht öffentlich,
// denn an dieser Stelle gibt es noch keine Sitzung; ausgewiesen wird sich mit
// dem Ticket aus dem ersten Schritt.
//
// Angenommen wird der laufende Code aus der App oder einer der Ersatzcodes. Ein
// verbrauchter Ersatzcode gilt danach nicht mehr.
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
	// Zwischen den beiden Schritten kann die Verwaltung den zweiten Faktor
	// zurückgesetzt haben. Dann ist das Passwort bereits geprüft und der Weg
	// steht offen -- eine Fehlermeldung wäre hier nur eine Sackgasse.
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

// ersatzcodesNeu wirft die alte Liste weg und legt eine neue an. Zurück kommen
// die Codes im Klartext -- das einzige Mal, dass es sie so gibt.
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

// ersatzcodeVerbrauchen prüft die Eingabe gegen die offenen Codes und streicht
// den, der passt. Verglichen wird gegen jeden einzeln, weil bcrypt-Hashes sich
// nicht nachschlagen lassen -- bei zehn Zeilen ist das kein Preis.
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
			// Nur streichen, wenn die Zeile noch offen ist: zwei gleichzeitige
			// Anmeldungen mit demselben Code sollen nicht beide durchkommen.
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
