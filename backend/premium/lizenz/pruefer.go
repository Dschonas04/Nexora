// Package lizenz verifies Nexora license keys. It is the paid half of the
// project and carries its own license -- see premium/LICENSE.
//
// A key is self-contained: it names the holder, the unlocked extras and an
// optional expiry, and it carries an Ed25519 signature over exactly that data.
// Verifying needs nothing but the public key baked in below, so an installation
// never talks to a license server and works in a network without internet.
//
// The trade-off of that choice: a key once issued cannot be revoked remotely.
// Expiry dates are the only lever, which is why keys for paying customers
// should carry one.
package lizenz

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	kern "nexora/internal/lizenz"
)

// oeffentlicherSchluessel is the counterpart of the private key that signs
// keys. Replacing it invalidates every key ever issued, so it is deliberately a
// constant and not a setting.
//
// Generated with premium/cmd/schluessel -neu.
const oeffentlicherSchluessel = "dsY-UfRNTuGKqTGdjBlJtb5k1rR8FJOarJ-nD9JJRlo"

// Nutzlast is what gets signed. Field names are short because the whole thing
// ends up in a key the customer has to copy by hand.
type Nutzlast struct {
	Inhaber string `json:"i"` // who the key is for
	// Die verkaufte Stufe. Sie ist die übliche Form; die Liste darunter bleibt
	// für Sonderfälle -- ein Kunde, der genau eine Funktion dazukauft.
	Stufe       string   `json:"s,omitempty"`
	Funktionen  []string `json:"f,omitempty"` // einzeln freigeschaltete Zusätze
	Ablauf      string   `json:"a,omitempty"` // ISO date, empty means perpetual
	Ausgestellt string   `json:"d"`           // issue date, for the record
}

// Pruefer implements kern.Pruefer.
type Pruefer struct {
	oeffentlich ed25519.PublicKey
}

// NeuerPruefer builds a verifier for a given public key.
//
// It exists so tests can generate their own key pair instead of shipping a
// working license key in the repository — a key committed once is a key handed
// to everyone who clones it, and offline verification means it can never be
// revoked.
func NeuerPruefer(oeffentlich ed25519.PublicKey) *Pruefer {
	return &Pruefer{oeffentlich: oeffentlich}
}

// init registers this package as the verifier. Importing it is what switches
// the paid extras on -- see backend/premium_an.go.
func init() {
	roh, err := base64.RawURLEncoding.DecodeString(oeffentlicherSchluessel)
	if err != nil || len(roh) != ed25519.PublicKeySize {
		// A build with a broken public key must not silently accept keys.
		// Registering nothing leaves every extra locked.
		return
	}
	kern.Registriere(&Pruefer{oeffentlich: ed25519.PublicKey(roh)})
}

// Pruefe reads a key of the form <nutzlast>.<signatur>, both base64url without
// padding, and reports what it unlocks.
func (p *Pruefer) Pruefe(schluessel string) (kern.Zustand, error) {
	schluessel = strings.TrimSpace(schluessel)
	teile := strings.Split(schluessel, ".")
	if len(teile) != 2 {
		return kern.Zustand{}, errors.New("Schlüssel hat nicht die Form <Daten>.<Signatur>")
	}

	daten, err := base64.RawURLEncoding.DecodeString(teile[0])
	if err != nil {
		return kern.Zustand{}, errors.New("Datenteil des Schlüssels ist unlesbar")
	}
	sig, err := base64.RawURLEncoding.DecodeString(teile[1])
	if err != nil {
		return kern.Zustand{}, errors.New("Signaturteil des Schlüssels ist unlesbar")
	}

	// Signature first. Everything below trusts the payload, so nothing may be
	// read out of it before this check passed.
	if !ed25519.Verify(p.oeffentlich, daten, sig) {
		return kern.Zustand{}, errors.New("Signatur passt nicht -- Schlüssel ungültig oder verändert")
	}

	var n Nutzlast
	if err := json.Unmarshal(daten, &n); err != nil {
		return kern.Zustand{}, errors.New("Daten im Schlüssel sind kein gültiges JSON")
	}

	if n.Ablauf != "" {
		ab, err := time.Parse("2006-01-02", n.Ablauf)
		if err != nil {
			return kern.Zustand{}, fmt.Errorf("Ablaufdatum %q ist unlesbar", n.Ablauf)
		}
		// End of the stated day, so a key valid "until the 31st" still works on
		// the 31st in every timezone west of UTC.
		if time.Now().After(ab.Add(24 * time.Hour)) {
			return kern.Zustand{}, fmt.Errorf("Lizenz ist am %s abgelaufen", n.Ablauf)
		}
	}

	// Aus der Stufe wird die Liste, dann kommen einzeln genannte Zusätze dazu.
	// Ein Schlüssel darf beides tragen: die Stufe für das Übliche, die Liste
	// für das, was jemand darüber hinaus gekauft hat.
	gewuenscht := n.Funktionen
	var stufe kern.Stufe
	if n.Stufe != "" {
		if !kern.StufeGueltig(kern.Stufe(n.Stufe)) {
			return kern.Zustand{}, fmt.Errorf("unbekannte Stufe %q", n.Stufe)
		}
		stufe = kern.Stufe(n.Stufe)
		for _, f := range kern.FunktionenDerStufe(stufe) {
			gewuenscht = append(gewuenscht, string(f))
		}
	}

	// Unknown names in the key are dropped rather than passed through: a key
	// from a newer generator must not unlock anything this build cannot do.
	//
	// Doppelte fallen dabei ebenfalls weg -- Stufe und Liste dürfen sich
	// überschneiden, in der Antwort steht jede Funktion einmal.
	gesehen := map[kern.Funktion]bool{}
	var erlaubt []kern.Funktion
	for _, f := range gewuenscht {
		for _, bekannt := range kern.Alle {
			if kern.Funktion(f) == bekannt && !gesehen[bekannt] {
				gesehen[bekannt] = true
				erlaubt = append(erlaubt, bekannt)
			}
		}
	}
	// Die freie Stufe schaltet nichts frei, und das ist kein Fehler: ein
	// Schlüssel auf "free" ist eine gültige Aussage über den Umfang.
	if len(erlaubt) == 0 && stufe != kern.StufeFrei {
		return kern.Zustand{}, errors.New("Schlüssel schaltet keine Funktion frei, die dieser Build kennt")
	}

	return kern.Zustand{
		Inhaber:    n.Inhaber,
		Stufe:      stufe,
		Funktionen: erlaubt,
		LaeuftAb:   n.Ablauf,
		Gueltig:    true,
	}, nil
}
