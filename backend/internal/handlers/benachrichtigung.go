// Inbox messages by e-mail as well.
//
// The inbox stays the place where things are read; the mail only says that
// something is waiting there. That is why it carries the same few kinds and no
// more -- a page changed is still no reason to write to somebody -- and why
// every account switches it on for itself. Mail nobody asked for is the fastest
// way into the spam folder, and then the one mail that matters lands there too.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"nexora/internal/middleware"
)

// Mailer sends one plain-text mail. internal/mail implements it; the interface
// keeps the handlers free of SMTP and testable without a mail server.
type Mailer interface {
	Senden(ctx context.Context, an, betreff, text string) error
}

type benachrichtigungAntwort struct {
	// Email is the account's choice.
	Email bool `json:"email"`
	// Verfuegbar says whether this instance can send mail at all. Without it
	// the switch stays greyed out.
	Verfuegbar bool   `json:"verfuegbar"`
	Adresse    string `json:"adresse"`
}

func (s *Server) benachrichtigungLesen(ctx context.Context, uid string) (benachrichtigungAntwort, error) {
	a := benachrichtigungAntwort{Verfuegbar: s.Mail != nil}
	err := s.Pool.QueryRow(ctx,
		`SELECT mail_benachrichtigung, email FROM users WHERE id=$1`, uid).Scan(&a.Email, &a.Adresse)
	return a, err
}

// Benachrichtigung returns the signed-in account's choice.
func (s *Server) Benachrichtigung(w http.ResponseWriter, r *http.Request) {
	a, err := s.benachrichtigungLesen(r.Context(), middleware.UserID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "nicht lesbar")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// BenachrichtigungSpeichern switches the mail on or off for the signed-in
// account.
func (s *Server) BenachrichtigungSpeichern(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email bool `json:"email"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeErr(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	// Switching on without a way to send would be a promise nobody keeps.
	if req.Email && s.Mail == nil {
		writeErr(w, http.StatusConflict, "E-Mail ist auf dieser Instanz nicht eingerichtet.")
		return
	}
	uid := middleware.UserID(r)
	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE users SET mail_benachrichtigung=$2 WHERE id=$1`, uid, req.Email); err != nil {
		writeErr(w, http.StatusInternalServerError, "nicht gespeichert")
		return
	}
	a, err := s.benachrichtigungLesen(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "nicht lesbar")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// BenachrichtigungTesten sends a mail to the signed-in account right away, so a
// wrong server, port or password shows up now and not as the first comment
// that never arrives.
func (s *Server) BenachrichtigungTesten(w http.ResponseWriter, r *http.Request) {
	if s.Mail == nil {
		writeErr(w, http.StatusConflict, "E-Mail ist auf dieser Instanz nicht eingerichtet.")
		return
	}
	uid := middleware.UserID(r)
	var an, sprache string
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT email, sprache FROM users WHERE id=$1`, uid).Scan(&an, &sprache); err != nil {
		writeErr(w, http.StatusInternalServerError, "nicht lesbar")
		return
	}
	if strings.TrimSpace(an) == "" {
		writeErr(w, http.StatusBadRequest, "Dein Konto hat keine E-Mail-Adresse.")
		return
	}
	betreff, text := testMail(sprache, s.SSO.OeffentlicheURL)
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	if err := s.Mail.Senden(ctx, an, betreff, text); err != nil {
		log.Printf("Testmail für Konto %s: %v", uid, err)
		writeErr(w, http.StatusBadGateway, "Senden gescheitert: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "an": an})
}

// mailZuNachricht sends an inbox message by e-mail to an account that switched
// it on. In a goroutine of its own and with its own deadline: the comment that
// triggered it is saved already, and a slow mail server must not keep the
// person who wrote it waiting.
func (s *Server) mailZuNachricht(empfaenger, art, pageID, ausloeserName, seitenTitel, text string) {
	if s.Mail == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var an, sprache string
		var ein bool
		if err := s.Pool.QueryRow(ctx,
			`SELECT email, sprache, mail_benachrichtigung FROM users WHERE id=$1`, empfaenger).
			Scan(&an, &sprache, &ein); err != nil || !ein || strings.TrimSpace(an) == "" {
			return
		}
		betreff, inhalt := nachrichtMail(sprache, art, pageID, ausloeserName, seitenTitel, text, s.SSO.OeffentlicheURL)
		if err := s.Mail.Senden(ctx, an, betreff, inhalt); err != nil {
			// The account id and not the address: the log is read by more
			// people than the inbox.
			log.Printf("Mail (%s an Konto %s): %v", art, empfaenger, err)
		}
	}()
}

func je(en bool, de, eng string) string {
	if en {
		return eng
	}
	return de
}

// nachrichtMail phrases subject and text in the account's language. Without a
// choice German, the language the instance speaks first.
func nachrichtMail(sprache, art, pageID, wer, titel, auszug, adresse string) (string, string) {
	en := sprache == "en"
	if strings.TrimSpace(wer) == "" {
		wer = je(en, "Jemand", "Somebody")
	}
	if strings.TrimSpace(titel) == "" {
		titel = je(en, "Ohne Titel", "Untitled")
	}
	var betreff string
	switch art {
	case PostKommentar:
		betreff = je(en, fmt.Sprintf("%s hat „%s“ kommentiert", wer, titel), fmt.Sprintf("%s commented on “%s”", wer, titel))
	case PostAntwort:
		betreff = je(en, fmt.Sprintf("%s hat auf deinen Kommentar zu „%s“ geantwortet", wer, titel), fmt.Sprintf("%s replied to your comment on “%s”", wer, titel))
	case PostErwaehnt:
		betreff = je(en, fmt.Sprintf("%s hat dich auf „%s“ erwähnt", wer, titel), fmt.Sprintf("%s mentioned you on “%s”", wer, titel))
	case PostFreigabe:
		betreff = je(en, fmt.Sprintf("%s hat „%s“ mit dir geteilt", wer, titel), fmt.Sprintf("%s shared “%s” with you", wer, titel))
	default:
		betreff = je(en, "Neue Nachricht in deinem Postfach", "New message in your inbox")
	}
	// Names and titles are user input; one line, whatever they contain.
	betreff = strings.Join(strings.Fields(betreff), " ")

	var b strings.Builder
	b.WriteString(betreff + ".\n\n")
	if a := strings.TrimSpace(auszug); a != "" {
		for _, z := range strings.Split(a, "\n") {
			b.WriteString("> " + z + "\n")
		}
		b.WriteString("\n")
	}
	if adresse != "" && pageID != "" {
		b.WriteString(je(en, "Zur Seite:", "Open the page:") + "\n")
		b.WriteString(strings.TrimRight(adresse, "/") + "/page/" + pageID + "\n\n")
	}
	b.WriteString("-- \n")
	b.WriteString(je(en,
		"Diese Mail kommt, weil in deinem Nexora-Konto E-Mail-Benachrichtigungen eingeschaltet sind. Abschalten: Mein Konto → Benachrichtigungen.\n",
		"You get this mail because e-mail notifications are switched on in your Nexora account. Switch them off under My account → Notifications.\n"))
	return "Nexora: " + betreff, b.String()
}

func testMail(sprache, adresse string) (string, string) {
	en := sprache == "en"
	text := je(en,
		"Wenn diese Mail angekommen ist, ist der Versand richtig eingerichtet.\n",
		"If this mail arrived, sending is set up correctly.\n")
	if adresse == "" {
		text += je(en,
			"\nHinweis: oeffentliche_url ist nicht gesetzt, Mails enthalten deshalb keinen Link zur Seite.\n",
			"\nNote: oeffentliche_url is not set, so mails carry no link to the page.\n")
	}
	return je(en, "Nexora: Testmail", "Nexora: test mail"), text
}
