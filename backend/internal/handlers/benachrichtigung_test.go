package handlers

import (
	"strings"
	"testing"
)

func TestNachrichtMailDeutschMitLink(t *testing.T) {
	betreff, text := nachrichtMail("", PostKommentar, "abc", "Anna", "Ofenplan", "Läuft der Ofen?", "https://wiki.example.org/")
	if betreff != "Nexora: Anna hat „Ofenplan“ kommentiert" {
		t.Errorf("Betreff: %q", betreff)
	}
	for _, muss := range []string{"> Läuft der Ofen?", "https://wiki.example.org/page/abc", "Mein Konto → Benachrichtigungen"} {
		if !strings.Contains(text, muss) {
			t.Errorf("Text ohne %q:\n%s", muss, text)
		}
	}
}

func TestNachrichtMailEnglischOhneLink(t *testing.T) {
	betreff, text := nachrichtMail("en", PostFreigabe, "abc", "", "", "", "")
	if betreff != "Nexora: Somebody shared “Untitled” with you" {
		t.Errorf("Betreff: %q", betreff)
	}
	if strings.Contains(text, "/page/") {
		t.Errorf("Link ohne oeffentliche_url:\n%s", text)
	}
}

func TestNachrichtMailBetreffEinzeilig(t *testing.T) {
	betreff, _ := nachrichtMail("", PostErwaehnt, "abc", "Eve\r\nBcc: x@example.org", "Titel", "", "")
	if strings.ContainsAny(betreff, "\r\n") {
		t.Errorf("Zeilenumbruch im Betreff: %q", betreff)
	}
}
