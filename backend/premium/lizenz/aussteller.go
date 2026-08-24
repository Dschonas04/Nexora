package lizenz

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	kern "nexora/internal/lizenz"
)

// HoechsteLaufzeit begrenzt, wie weit ein Schlüssel in die Zukunft reichen darf.
//
// Ein Jahr, und zwar aus einem Grund, der nichts mit dem Verkauf zu tun hat:
// geprüft wird offline, also lässt sich ein ausgegebener Schlüssel nicht mehr
// zurückrufen. Das Ablaufdatum ist der einzige Hebel, den es gibt. Ein
// unbefristeter Schlüssel wäre ein Zugang für immer, auch wenn die Abmachung
// dahinter längst endet.
//
// 366 Tage statt 365: sonst fällt ein am 29. Februar ausgestellter Schlüssel
// aus der Grenze.
const HoechsteLaufzeit = 366 * 24 * time.Hour

// Ausstellen signiert einen Lizenzschlüssel.
//
// Hier und nur hier entsteht ein Schlüssel -- die Befehlszeile und die
// Verwaltungsoberfläche rufen dieselbe Stelle auf. Zwei Wege, die dieselben
// Regeln je für sich umsetzen, driften auseinander, und die Regel, die dann
// fehlt, ist erfahrungsgemäß die Frist.
func Ausstellen(privat ed25519.PrivateKey, inhaber string, stufe kern.Stufe,
	zusaetzlich []kern.Funktion, ablauf time.Time) (string, error) {

	inhaber = strings.TrimSpace(inhaber)
	if inhaber == "" {
		return "", errors.New("ohne Inhaber wird kein Schlüssel ausgestellt")
	}
	if len(privat) != ed25519.PrivateKeySize {
		return "", errors.New("kein gültiger privater Schlüssel")
	}
	if stufe == "" && len(zusaetzlich) == 0 {
		return "", errors.New("weder eine Stufe noch einzelne Funktionen angegeben")
	}
	if stufe != "" && !kern.StufeGueltig(stufe) {
		return "", fmt.Errorf("unbekannte Stufe %q", stufe)
	}

	// Ohne Angabe: ein Jahr. Das ist die Regel, nicht die Ausnahme.
	if ablauf.IsZero() {
		ablauf = time.Now().Add(HoechsteLaufzeit)
	}
	if ablauf.Before(time.Now()) {
		return "", errors.New("das Ablaufdatum liegt in der Vergangenheit")
	}
	if ablauf.After(time.Now().Add(HoechsteLaufzeit)) {
		return "", fmt.Errorf("länger als ein Jahr wird nicht ausgestellt (höchstens %s)",
			time.Now().Add(HoechsteLaufzeit).Format("2006-01-02"))
	}

	var namen []string
	for _, f := range zusaetzlich {
		namen = append(namen, string(f))
	}

	n := Nutzlast{
		Inhaber:     inhaber,
		Stufe:       string(stufe),
		Funktionen:  namen,
		Ablauf:      ablauf.Format("2006-01-02"),
		Ausgestellt: time.Now().Format("2006-01-02"),
	}
	daten, err := json.Marshal(n)
	if err != nil {
		return "", fmt.Errorf("Daten konnten nicht verpackt werden: %w", err)
	}
	sig := ed25519.Sign(privat, daten)
	return base64.RawURLEncoding.EncodeToString(daten) + "." +
		base64.RawURLEncoding.EncodeToString(sig), nil
}

// hausAussteller verbindet den Kern mit dem Signieren. Er hält den privaten
// Schlüssel nicht fest, sondern liest ihn bei jedem Aufruf aus der Umgebung:
// so lässt er sich abziehen, ohne den Dienst neu zu starten, und er steht
// nicht dauerhaft im Speicher eines langlebigen Prozesses.
type hausAussteller struct{}

func (hausAussteller) Moeglich() bool { return privatAusUmgebung() != nil }

func (hausAussteller) Stelle(inhaber string, stufe kern.Stufe,
	zusaetzlich []kern.Funktion, ablauf time.Time) (string, error) {
	priv := privatAusUmgebung()
	if priv == nil {
		return "", errors.New("kein privater Schlüssel hinterlegt")
	}
	return Ausstellen(priv, inhaber, stufe, zusaetzlich, ablauf)
}

func privatAusUmgebung() ed25519.PrivateKey {
	roh := strings.TrimSpace(os.Getenv("NEXORA_SIGNIERSCHLUESSEL"))
	if roh == "" {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(roh)
	if err != nil || len(b) != ed25519.PrivateKeySize {
		return nil
	}
	return ed25519.PrivateKey(b)
}

func init() { kern.RegistriereAussteller(hausAussteller{}) }
