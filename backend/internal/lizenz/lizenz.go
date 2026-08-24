// Package lizenz is the gate between the freely licensed core and the paid
// extras. It lives on the Apache side on purpose and knows nothing about how a
// key is verified -- it only asks whoever registered itself as the verifier.
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
	// switch so a key can grant exactly what was paid for -- an audit trail
	// without groups is a perfectly sensible purchase.
	Pruefspur   Funktion = "pruefspur"   // audit trail of who did what
	Gruppen     Funktion = "gruppen"     // groups, space permissions, space-manager role
	SSO         Funktion = "sso"         // sign-in through an OIDC provider
	LDAP        Funktion = "ldap"        // sign-in against LDAP or Active Directory
	Anhangsuche Funktion = "anhangsuche" // full text search inside attachments
	Export      Funktion = "export"      // exporting a whole space
	Vorlagen    Funktion = "vorlagen"    // page templates
	Kommentare  Funktion = "kommentare"  // comments on pages
	Konflikte   Funktion = "konflikte"   // detecting concurrent edits
)

// Alle lists every paid extra. Used for the status endpoint and by the key
// generator, so that both sides always agree on what exists.
var Alle = []Funktion{
	Versionen, Anhaenge, Freigeben,
	Pruefspur, Gruppen, SSO, LDAP, Anhangsuche, Export, Vorlagen, Kommentare, Konflikte,
}

// Stufe ist ein Bündel von Funktionen -- das, was verkauft wird.
//
// Einzelne Funktionen bleiben trotzdem der Maßstab im Code: geprüft wird immer
// gegen eine Funktion, nie gegen eine Stufe. Sonst müsste jede Abfrage wissen,
// welche Stufe was enthält, und eine Umstellung des Angebots hieße, den halben
// Code anzufassen.
type Stufe string

const (
	StufeFrei     Stufe = "free"
	StufeAdvanced Stufe = "advanced"
	StufePro      Stufe = "pro"
	StufeBusiness Stufe = "business"
)

// StufenReihe ist die Ordnung vom kleinsten zum größten Umfang.
var StufenReihe = []Stufe{StufeFrei, StufeAdvanced, StufePro, StufeBusiness}

// stufenZusatz nennt, was eine Stufe GEGENÜBER DER VORIGEN hinzufügt. Kumulativ
// aufgeschrieben wäre dieselbe Liste dreimal da, und beim nächsten Umsortieren
// stimmte eine davon nicht mehr.
var stufenZusatz = map[Stufe][]Funktion{
	StufeFrei:     {},
	StufeAdvanced: {Versionen, Anhaenge, Vorlagen, Kommentare},
	StufePro:      {Freigeben, Konflikte, Export, Anhangsuche},
	StufeBusiness: {Gruppen, Pruefspur, SSO, LDAP},
}

// FunktionenDerStufe liefert alles, was eine Stufe enthält -- einschließlich
// dessen, was die kleineren schon enthielten.
func FunktionenDerStufe(st Stufe) []Funktion {
	var raus []Funktion
	for _, s := range StufenReihe {
		raus = append(raus, stufenZusatz[s]...)
		if s == st {
			return raus
		}
	}
	// Unbekannte Stufe: nichts freischalten. Ein Tippfehler im Schlüssel darf
	// nicht zufällig zu Business führen.
	return nil
}

// StufeGueltig sagt, ob der Name eine bekannte Stufe ist.
func StufeGueltig(st Stufe) bool {
	for _, s := range StufenReihe {
		if s == st {
			return true
		}
	}
	return false
}

// Aussteller signiert neue Schlüssel. Auch das implementiert das
// premium-Paket -- der freie Kern kennt weder den privaten Schlüssel noch das
// Verfahren und kann deshalb keine Schlüssel erzeugen, egal wie er aufgerufen
// wird.
type Aussteller interface {
	// Moeglich sagt, ob überhaupt ausgestellt werden kann. Auf einer
	// gewöhnlichen Installation ist das nein: dort liegt kein privater
	// Schlüssel, und das soll auch so bleiben.
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
	Stufe      Stufe      `json:"stufe,omitempty"`     // das verkaufte Bündel, falls der Schlüssel eines nennt
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

// RegistriereAussteller installiert den Aussteller.
func RegistriereAussteller(a Aussteller) {
	mu.Lock()
	defer mu.Unlock()
	aussteller = a
}

// Ausstellbar sagt, ob diese Installation Schlüssel erzeugen kann.
func Ausstellbar() bool {
	mu.RLock()
	defer mu.RUnlock()
	return aussteller != nil && aussteller.Moeglich()
}

// Ausstellen erzeugt einen Schlüssel, sofern diese Installation das darf.
func Ausstellen(inhaber string, stufe Stufe, zusaetzlich []Funktion, ablauf time.Time) (string, error) {
	mu.RLock()
	a := aussteller
	mu.RUnlock()
	if a == nil || !a.Moeglich() {
		return "", errors.New("diese Installation kann keine Schlüssel ausstellen")
	}
	return a.Stelle(inhaber, stufe, zusaetzlich, ablauf)
}

// Pruefe liest einen Schlüssel, ohne den geltenden zu ersetzen. Für die
// Verwaltung: erst sehen, was ein Schlüssel enthält, dann entscheiden.
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
