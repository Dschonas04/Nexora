// Package lizenz is the gate between the freely licensed core and the paid
// extras. It lives on the Apache side on purpose and knows nothing about how a
// key is verified, it only asks whoever registered itself as the verifier.
//
// Without a registered verifier every extra stays locked. That is what makes a
// build without the premium directory possible: the core compiles and runs, the
// paid features simply report themselves as unavailable.
package lizenz

import (
	"errors"
	"sync"
	"time"
)

// Funktion names one paid extra. The strings travel to the browser and into
// license keys, so they are part of the interface and must not be renamed.
type Funktion string

const (
	Versionen Funktion = "versionen" // page history: snapshots, browsing, restoring
	Anhaenge  Funktion = "anhaenge"  // per-page file uploads
	Freigeben Funktion = "freigeben" // sharing with accounts and public read-only links

	// Business extras. Kept as separate names rather than one "enterprise"
	// switch so a key can grant exactly what was paid for, an audit trail
	// without groups is a perfectly sensible purchase.
	Pruefspur   Funktion = "pruefspur"   // audit trail of who did what
	Gruppen     Funktion = "gruppen"     // groups, space permissions, space-manager role
	SSO         Funktion = "sso"         // sign-in through an OIDC provider
	LDAP        Funktion = "ldap"        // sign-in against LDAP or Active Directory
	Anhangsuche Funktion = "anhangsuche" // full text search inside attachments
	Export      Funktion = "export"      // exporting a whole space
	Kommentare  Funktion = "kommentare"  // comments on pages
	Konflikte   Funktion = "konflikte"   // detecting concurrent edits
	Echtzeit    Funktion = "echtzeit"    // several accounts writing on one page at the same time
)

// Alle lists every paid extra. Used for the status endpoint and by the key
// generator, so that both sides always agree on what exists.
var Alle = []Funktion{
	Versionen, Anhaenge, Freigeben,
	Pruefspur, Gruppen, SSO, LDAP, Anhangsuche, Export, Kommentare, Konflikte,
	Echtzeit,
}

// Stufe is a bundle of features, the thing that is sold.
//
// Individual features remain the measure in the code nonetheless: checks always
// run against a feature, never against a tier. Otherwise every query would have
// to know which tier contains what, and rearranging the offer would mean
// touching half the code.
type Stufe string

const (
	StufeFrei     Stufe = "free"
	StufeAdvanced Stufe = "advanced"
	StufePro      Stufe = "pro"
	StufeBusiness Stufe = "business"
)

// StufenReihe is the order from the smallest tier to the largest.
var StufenReihe = []Stufe{StufeFrei, StufeAdvanced, StufePro, StufeBusiness}

// stufenZusatz names what a tier adds COMPARED TO THE PREVIOUS ONE. Written
// cumulatively the same list would appear three times, and at the next
// rearrangement one of them would no longer be right.
var stufenZusatz = map[Stufe][]Funktion{
	StufeFrei:     {},
	StufeAdvanced: {Versionen, Anhaenge, Kommentare},
	StufePro:      {Freigeben, Konflikte, Echtzeit, Export, Anhangsuche},
	StufeBusiness: {Gruppen, Pruefspur, SSO, LDAP},
}

// FunktionenDerStufe returns everything a tier contains, including what the
// smaller ones already contained.
func FunktionenDerStufe(st Stufe) []Funktion {
	var raus []Funktion
	for _, s := range StufenReihe {
		raus = append(raus, stufenZusatz[s]...)
		if s == st {
			return raus
		}
	}
	// Unknown tier: unlock nothing. A typo in the key must not accidentally lead
	// to Business.
	return nil
}

// StufeGueltig reports whether the name is a known tier.
func StufeGueltig(st Stufe) bool {
	for _, s := range StufenReihe {
		if s == st {
			return true
		}
	}
	return false
}

// Aussteller signs new keys. This too is implemented by the premium package;
// the free core knows neither the private key nor the procedure and can
// therefore issue no keys, however it is called.
type Aussteller interface {
	// Moeglich says whether issuing is possible at all. On an ordinary
	// installation the answer is no: no private key lies there, and it should stay
	// that way.
	Moeglich() bool
	Stelle(inhaber string, stufe Stufe, zusaetzlich []Funktion, ablauf time.Time) (string, error)
}

// Pruefer verifies a key and answers what it unlocks. The premium package
// implements it; the core never sees the signature logic.
type Pruefer interface {
	// Pruefe reads a key and returns the unlocked extras. An invalid, expired
	// or forged key must yield an error, never a partial result.
	Pruefe(schluessel string) (Zustand, error)
}

// Zustand is what a valid key grants. It is also the shape the browser gets,
// minus the key itself.
type Zustand struct {
	Inhaber    string     `json:"inhaber"`             // who the key was issued to
	Stufe      Stufe      `json:"stufe,omitempty"`     // the bundle that was sold, when the key names one
	Funktionen []Funktion `json:"funktionen"`          // what it unlocks
	LaeuftAb   string     `json:"laeuft_ab,omitempty"` // ISO date, empty means perpetual
	Gueltig    bool       `json:"gueltig"`
	Grund      string     `json:"grund,omitempty"` // why an invalid key was rejected
}

var (
	mu         sync.RWMutex
	pruefer    Pruefer
	aussteller Aussteller
	aktuell    Zustand
)

// Registriere installs the verifier. The premium package calls it from init(),
// so importing that package is what turns the paid extras on.
func Registriere(p Pruefer) {
	mu.Lock()
	defer mu.Unlock()
	pruefer = p
}

// RegistriereAussteller installs the issuer.
func RegistriereAussteller(a Aussteller) {
	mu.Lock()
	defer mu.Unlock()
	aussteller = a
}

// Ausstellbar says whether this installation can generate keys.
func Ausstellbar() bool {
	mu.RLock()
	defer mu.RUnlock()
	return aussteller != nil && aussteller.Moeglich()
}

// Ausstellen creates a key, provided this installation is allowed to.
func Ausstellen(inhaber string, stufe Stufe, zusaetzlich []Funktion, ablauf time.Time) (string, error) {
	mu.RLock()
	a := aussteller
	mu.RUnlock()
	if a == nil || !a.Moeglich() {
		return "", errors.New("diese Installation kann keine Schlüssel ausstellen")
	}
	return a.Stelle(inhaber, stufe, zusaetzlich, ablauf)
}

// Pruefe reads a key without replacing the one in force. For the admin console:
// first see what a key contains, then decide.
func Pruefe(schluessel string) (Zustand, error) {
	mu.RLock()
	p := pruefer
	mu.RUnlock()
	if p == nil {
		return Zustand{}, errors.New("dieser Build enthält die Lizenzprüfung nicht")
	}
	return p.Pruefe(schluessel)
}

// Laden takes the configured key and remembers what it unlocks. It is called
// once at startup; a bad key is not fatal, the server simply runs with the free
// feature set and says why.
func Laden(schluessel string) {
	mu.Lock()
	defer mu.Unlock()

	aktuell = Zustand{}
	if schluessel == "" {
		aktuell.Grund = "kein Lizenzschlüssel hinterlegt"
		return
	}
	if pruefer == nil {
		// A build without the premium directory. A key cannot be checked here,
		// and claiming it were valid would unlock code that is not present.
		aktuell.Grund = "dieser Build enthält die Lizenzprüfung nicht"
		return
	}

	z, err := pruefer.Pruefe(schluessel)
	if err != nil {
		aktuell.Grund = err.Error()
		return
	}
	aktuell = z
}

// Frei reports whether one extra is unlocked. Everything the core does without
// a key never asks this.
func Frei(f Funktion) bool {
	mu.RLock()
	defer mu.RUnlock()
	if !aktuell.Gueltig {
		return false
	}
	for _, x := range aktuell.Funktionen {
		if x == f {
			return true
		}
	}
	return false
}

// Aktuell returns the current state for the status endpoint.
func Aktuell() Zustand {
	mu.RLock()
	defer mu.RUnlock()
	return aktuell
}
