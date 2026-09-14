// Package mail sends plain-text mail over SMTP.
//
// Deliberately small: one mail at a time, no queue, no HTML. Nexora sends a
// handful of notifications, not newsletters, and every part more is a part that
// can break at three in the morning.
//
// Unencrypted only when asked for in so many words: with "starttls" (the
// default) a server that does not offer STARTTLS gets nothing, rather than the
// password and the mail in the clear.
package mail

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strings"
	"time"
)

// Einstellungen come from the [Mail] section of config.conf.
type Einstellungen struct {
	// Server is host:port. Without a port the usual one for the encryption is
	// taken: 587 for starttls, 465 for tls, 25 for keine.
	Server   string
	Benutzer string
	Passwort string
	// Absender as it appears in the mail, "Nexora <wiki@example.org>" or just
	// the address.
	Absender string
	// Verschluesselung is starttls (default), tls or keine.
	Verschluesselung string
}

// Versender sends mail with fixed settings.
type Versender struct {
	e       Einstellungen
	host    string
	adresse string
	von     string
	kopfVon string
}

// ErrUnverschluesselt is returned when "starttls" is set and the server does
// not offer it.
var ErrUnverschluesselt = errors.New("der SMTP-Server bietet kein STARTTLS an, unverschlüsselt wird nichts gesendet")

// Neu checks the settings. Without a server it returns nil, nil: no mail, and
// that is not an error but the default.
func Neu(e Einstellungen) (*Versender, error) {
	if strings.TrimSpace(e.Server) == "" {
		return nil, nil
	}
	if strings.TrimSpace(e.Absender) == "" {
		return nil, errors.New("smtp_server ist gesetzt, smtp_absender fehlt")
	}
	art := strings.ToLower(strings.TrimSpace(e.Verschluesselung))
	if art == "" {
		art = "starttls"
	}
	vorgabePort := map[string]string{"starttls": "587", "tls": "465", "keine": "25"}
	if _, ok := vorgabePort[art]; !ok {
		return nil, fmt.Errorf("smtp_verschluesselung=%q, erwartet starttls, tls oder keine", e.Verschluesselung)
	}
	e.Verschluesselung = art

	adresse := strings.TrimSpace(e.Server)
	host, _, err := net.SplitHostPort(adresse)
	if err != nil {
		host = adresse
		adresse = net.JoinHostPort(host, vorgabePort[art])
	}
	a, err := netmail.ParseAddress(e.Absender)
	if err != nil {
		return nil, fmt.Errorf("smtp_absender %q ist keine Adresse: %v", e.Absender, err)
	}
	return &Versender{e: e, host: host, adresse: adresse, von: a.Address, kopfVon: a.String()}, nil
}

// Senden delivers one mail. The deadline comes from ctx, 30 seconds without
// one: a mail server that does not answer must not hold anything up for long.
func (v *Versender) Senden(ctx context.Context, an, betreff, text string) error {
	// Page titles and names are user input and end up in headers. A line
	// break there would be a way to add headers of one's own.
	if strings.ContainsAny(an+betreff, "\r\n") {
		return errors.New("Zeilenumbruch in Empfänger oder Betreff")
	}
	ziel, err := netmail.ParseAddress(an)
	if err != nil {
		return fmt.Errorf("Empfänger %q: %v", an, err)
	}
	frist, ok := ctx.Deadline()
	if !ok {
		frist = time.Now().Add(30 * time.Second)
	}

	waehler := &net.Dialer{Timeout: 15 * time.Second}
	tlsKonf := &tls.Config{ServerName: v.host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	if v.e.Verschluesselung == "tls" {
		conn, err = (&tls.Dialer{NetDialer: waehler, Config: tlsKonf}).DialContext(ctx, "tcp", v.adresse)
	} else {
		conn, err = waehler.DialContext(ctx, "tcp", v.adresse)
	}
	if err != nil {
		return fmt.Errorf("Verbindung zu %s: %w", v.adresse, err)
	}
	_ = conn.SetDeadline(frist)

	c, err := smtp.NewClient(conn, v.host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP-Begrüßung: %w", err)
	}
	defer c.Close()

	if err := c.Hello(domain(v.von)); err != nil {
		return fmt.Errorf("EHLO: %w", err)
	}
	if v.e.Verschluesselung == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return ErrUnverschluesselt
		}
		if err := c.StartTLS(tlsKonf); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}
	if v.e.Benutzer != "" {
		// PlainAuth refuses to send the password over an unencrypted
		// connection to anything but localhost -- exactly right.
		if err := c.Auth(smtp.PlainAuth("", v.e.Benutzer, v.e.Passwort, v.host)); err != nil {
			return fmt.Errorf("Anmeldung am SMTP-Server: %w", err)
		}
	}
	if err := c.Mail(v.von); err != nil {
		return fmt.Errorf("Absender abgelehnt: %w", err)
	}
	if err := c.Rcpt(ziel.Address); err != nil {
		return fmt.Errorf("Empfänger abgelehnt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(v.nachricht(ziel.String(), betreff, text)); err != nil {
		w.Close()
		return fmt.Errorf("Mail schreiben: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("Mail abgelehnt: %w", err)
	}
	return c.Quit()
}

// nachricht builds the message. Quoted-printable and not 8bit: umlauts then
// arrive intact even over a server that does not speak 8BITMIME.
func (v *Versender) nachricht(an, betreff, text string) []byte {
	var b strings.Builder
	kopf := func(k, w string) { b.WriteString(k + ": " + w + "\r\n") }
	kopf("From", v.kopfVon)
	kopf("To", an)
	kopf("Subject", mime.QEncoding.Encode("utf-8", betreff))
	kopf("Date", time.Now().Format(time.RFC1123Z))
	kopf("Message-ID", "<"+zufall()+"@"+domain(v.von)+">")
	kopf("MIME-Version", "1.0")
	kopf("Content-Type", "text/plain; charset=utf-8")
	kopf("Content-Transfer-Encoding", "quoted-printable")
	// RFC 3834: generated automatically, so no out-of-office replies to it.
	kopf("Auto-Submitted", "auto-generated")
	b.WriteString("\r\n")

	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	var rumpf strings.Builder
	qp := quotedprintable.NewWriter(&rumpf)
	_, _ = qp.Write([]byte(strings.ReplaceAll(text, "\n", "\r\n")))
	_ = qp.Close()
	b.WriteString(rumpf.String())
	return []byte(b.String())
}

func domain(adresse string) string {
	if i := strings.LastIndex(adresse, "@"); i >= 0 && i < len(adresse)-1 {
		return adresse[i+1:]
	}
	return "localhost"
}

func zufall() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
