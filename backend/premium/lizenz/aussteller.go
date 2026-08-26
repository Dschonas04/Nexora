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

// HoechsteLaufzeit limits how far into the future a key may reach.
//
// One year, and for a reason that has nothing to do with selling: checking
// happens offline, so a key once handed out can no longer be recalled. The
// expiry date is the only lever there is. An unlimited key would be access
// forever, even long after the agreement behind it ends.
//
// 366 days instead of 365: otherwise a key issued on 29 February falls outside
// the limit.
const HoechsteLaufzeit = 366 * 24 * time.Hour

// Ausstellen signs a licence key.
//
// Here and only here a key comes into being; the command line and the admin
// console call the same place. Two paths implementing the same rules each for
// themselves drift apart, and the rule that then goes missing is, in
// experience, the expiry.
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

	// Without a date: one year. That is the rule, not the exception.
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

// hausAussteller connects the core with the signing. It does not hold on to the
// private key but reads it from the environment on every call: that way it can
// be pulled without restarting the service, and it does not sit permanently in
// the memory of a long lived process.
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
