package mail

import (
	"bufio"
	"context"
	"errors"
	"io"
	"mime/quotedprintable"
	"net"
	"strings"
	"testing"
	"time"
)

// scheinServer speaks just enough SMTP to accept one mail and hands over what
// arrived after DATA. It offers no STARTTLS, which is what the tests need.
func scheinServer(t *testing.T) (string, <-chan string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan string, 1)
	go func() {
		defer l.Close()
		conn, err := l.Accept()
		if err != nil {
			ch <- ""
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		r := bufio.NewReader(conn)
		schreib := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		schreib("220 schein ESMTP")
		var daten strings.Builder
		for {
			zeile, err := r.ReadString('\n')
			if err != nil {
				ch <- daten.String()
				return
			}
			befehl := strings.ToUpper(strings.TrimSpace(zeile))
			switch {
			case strings.HasPrefix(befehl, "EHLO"), strings.HasPrefix(befehl, "HELO"):
				schreib("250-schein")
				schreib("250 8BITMIME")
			case befehl == "DATA":
				schreib("354 los")
				for {
					z, err := r.ReadString('\n')
					if err != nil || z == ".\r\n" {
						break
					}
					daten.WriteString(z)
				}
				schreib("250 angenommen")
			case befehl == "QUIT":
				schreib("221 tschüss")
				ch <- daten.String()
				return
			default:
				schreib("250 ok")
			}
		}
	}()
	return l.Addr().String(), ch
}

func TestOhneServerKeinVersender(t *testing.T) {
	v, err := Neu(Einstellungen{})
	if v != nil || err != nil {
		t.Fatalf("ohne smtp_server: %v, %v", v, err)
	}
}

func TestFalscheAngabenWerdenAbgewiesen(t *testing.T) {
	if _, err := Neu(Einstellungen{Server: "mail.example.org"}); err == nil {
		t.Error("ohne Absender angenommen")
	}
	if _, err := Neu(Einstellungen{Server: "mail.example.org", Absender: "a@example.org", Verschluesselung: "vielleicht"}); err == nil {
		t.Error("unbekannte Verschlüsselung angenommen")
	}
	v, err := Neu(Einstellungen{Server: "mail.example.org", Absender: "Nexora <wiki@example.org>"})
	if err != nil {
		t.Fatal(err)
	}
	if v.adresse != "mail.example.org:587" || v.e.Verschluesselung != "starttls" {
		t.Errorf("Vorgaben: %s %s", v.adresse, v.e.Verschluesselung)
	}
}

func TestMailKommtAn(t *testing.T) {
	adresse, erhalten := scheinServer(t)
	v, err := Neu(Einstellungen{Server: adresse, Absender: "Nexora <wiki@example.org>", Verschluesselung: "keine"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := v.Senden(ctx, "anna@example.org", "Anna hat „Grüße“ kommentiert", "Schöne Grüße\nzweite Zeile"); err != nil {
		t.Fatal(err)
	}
	roh := <-erhalten
	kopf, rumpf, ok := strings.Cut(roh, "\r\n\r\n")
	if !ok {
		t.Fatalf("keine Trennung von Kopf und Rumpf:\n%s", roh)
	}
	for _, muss := range []string{"From: \"Nexora\" <wiki@example.org>", "To: <anna@example.org>", "Subject: =?utf-8?q?", "Auto-Submitted: auto-generated", "Content-Transfer-Encoding: quoted-printable"} {
		if !strings.Contains(kopf, muss) {
			t.Errorf("Kopf ohne %q:\n%s", muss, kopf)
		}
	}
	text, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(rumpf)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "Schöne Grüße\r\nzweite Zeile") {
		t.Errorf("Rumpf: %q", text)
	}
}

func TestOhneStartTLSWirdNichtsGesendet(t *testing.T) {
	adresse, erhalten := scheinServer(t)
	v, err := Neu(Einstellungen{Server: adresse, Absender: "wiki@example.org", Benutzer: "wiki", Passwort: "geheim"})
	if err != nil {
		t.Fatal(err)
	}
	err = v.Senden(context.Background(), "anna@example.org", "Test", "Text")
	if !errors.Is(err, ErrUnverschluesselt) {
		t.Fatalf("erwartet ErrUnverschluesselt, bekam %v", err)
	}
	if d := <-erhalten; d != "" {
		t.Errorf("trotzdem Daten gesendet: %q", d)
	}
}

func TestKeineKopfzeilenEinschleusen(t *testing.T) {
	v, _ := Neu(Einstellungen{Server: "127.0.0.1:1", Absender: "wiki@example.org", Verschluesselung: "keine"})
	if err := v.Senden(context.Background(), "anna@example.org", "Titel\r\nBcc: mallory@example.org", "x"); err == nil {
		t.Error("Zeilenumbruch im Betreff angenommen")
	}
}
