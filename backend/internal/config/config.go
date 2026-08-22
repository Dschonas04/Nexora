// Package config reads Nexora's settings.
//
// Precedence, highest first:
//
//  1. environment variable
//  2. entry in config.conf
//  3. built-in default
//
// The environment wins so a container can override a single value without a
// rebuilt image, and so a secret can be injected without ever touching disk.
// The file exists because settings like LDAP need a dozen related values, and a
// dozen environment variables is a configuration nobody can read.
//
// A missing file is not an error: every setting has a working default, which is
// what lets the binary start with no configuration at all.
package config

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Konfig holds every setting. The comments in config.conf carry the same
// explanations, so the file documents itself without needing this source.
type Konfig struct {
	// Pfad ist die Datei, aus der gelesen wurde -- leer, wenn keine gefunden
	// wurde. Die Einstellungsseite zeigt und bearbeitet genau diese Datei;
	// ohne den Pfad müsste sie raten, und raten hiesse: die falsche ändern.
	Pfad string

	// --- Server ---
	Port            string
	DatenVerzeich   string
	OeffentlicheURL string

	// --- Datenbank ---
	DatenbankURL string

	// --- Sitzungen ---
	JWTGeheimnis string
	SitzungTage  int

	// --- Lizenz ---
	Lizenz string

	// --- Registrierung ---
	RegistrierungOffen bool
	ErlaubteDomaenen   []string

	// --- Suche ---
	SuchWoerterbuch string

	// --- Anhänge ---
	MaxAnhangMB int

	// --- Objektspeicher (S3) ---
	S3Aktiv     bool
	S3Endpunkt  string
	S3Bucket    string
	S3Zugriff   string
	S3Geheimnis string
	S3Region    string
	S3TLS       bool
	S3Pfadstil  bool

	// --- LDAP / Active Directory ---
	LDAPAktiv          bool
	LDAPServer         string
	LDAPStartTLS       bool
	LDAPTLSPruefen     bool
	LDAPBindDN         string
	LDAPBindPasswort   string
	LDAPBasisDN        string
	LDAPBenutzerFilter string
	LDAPFeldName       string
	LDAPFeldEmail      string
	LDAPGruppeAdmin    string

	// --- OIDC / Keycloak ---
	OIDCAktiv       bool
	OIDCAussteller  string
	OIDCClientID    string
	OIDCGeheimnis   string
	OIDCBereiche    string
	OIDCFeldName    string
	OIDCFeldEmail   string
	OIDCGruppeAdmin string
	OIDCKnopfText   string
}

// Standard returns the built-in defaults. Every one of them has to produce a
// server that starts and works, because that is what a fresh install gets.
func Standard() Konfig {
	return Konfig{
		Port:            "8080",
		DatenVerzeich:   "/data/attachments",
		OeffentlicheURL: "",
		DatenbankURL:    "postgres://nexora:nexora@localhost:5432/nexora?sslmode=disable",
		JWTGeheimnis:    "change-me-in-production",
		SitzungTage:     7,
		Lizenz:          "",

		RegistrierungOffen: true,
		ErlaubteDomaenen:   nil,

		SuchWoerterbuch: "german",
		MaxAnhangMB:     25,

		S3Aktiv:    false,
		S3Bucket:   "nexora",
		S3Region:   "us-east-1",
		S3TLS:      false,
		S3Pfadstil: true,

		LDAPAktiv:          false,
		LDAPStartTLS:       true,
		LDAPTLSPruefen:     true,
		LDAPBenutzerFilter: "(&(objectClass=person)(|(uid=%s)(sAMAccountName=%s)(mail=%s)))",
		LDAPFeldName:       "cn",
		LDAPFeldEmail:      "mail",

		OIDCAktiv:     false,
		OIDCBereiche:  "openid email profile",
		OIDCFeldName:  "name",
		OIDCFeldEmail: "email",
		OIDCKnopfText: "Mit SSO anmelden",
	}
}

// Laden reads the file, then lets the environment override. pfad may be empty,
// in which case NEXORA_CONFIG decides, falling back to ./config.conf and
// /etc/nexora/config.conf.
func Laden(pfad string) Konfig {
	k := Standard()

	if pfad == "" {
		pfad = os.Getenv("NEXORA_CONFIG")
	}
	kandidaten := []string{pfad, "config.conf", "/etc/nexora/config.conf"}

	var werte map[string]string
	var gelesen string
	for _, p := range kandidaten {
		if p == "" {
			continue
		}
		if m, err := datei(p); err == nil {
			werte, gelesen = m, p
			break
		}
	}
	k.Pfad = gelesen
	if gelesen != "" {
		log.Printf("Konfiguration gelesen aus %s (%d Einträge)", gelesen, len(werte))
	} else {
		log.Printf("Keine config.conf gefunden -- Vorgaben und Umgebungsvariablen gelten")
	}

	hol := func(schluessel, umgebung string) (string, bool) {
		merkeSchluessel(schluessel)
		// Umgebung schlägt Datei. Damit lässt sich ein einzelner Wert im
		// Container überschreiben, ohne die Datei anzufassen -- und ein
		// Geheimnis muss nie auf die Platte.
		if v := os.Getenv(umgebung); v != "" {
			return v, true
		}
		if werte != nil {
			if v, ok := werte[schluessel]; ok && v != "" {
				return v, true
			}
		}
		return "", false
	}
	text := func(ziel *string, schluessel, umgebung string) {
		if v, ok := hol(schluessel, umgebung); ok {
			*ziel = v
		}
	}
	zahl := func(ziel *int, schluessel, umgebung string) {
		if v, ok := hol(schluessel, umgebung); ok {
			if n, err := strconv.Atoi(v); err == nil {
				*ziel = n
			} else {
				log.Printf("Konfiguration: %s=%q ist keine Zahl, Vorgabe %d bleibt", schluessel, v, *ziel)
			}
		}
	}
	jaNein := func(ziel *bool, schluessel, umgebung string) {
		if v, ok := hol(schluessel, umgebung); ok {
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "1", "ja", "true", "an", "yes", "on":
				*ziel = true
			case "0", "nein", "false", "aus", "no", "off":
				*ziel = false
			default:
				log.Printf("Konfiguration: %s=%q ist kein Ja/Nein, Vorgabe bleibt", schluessel, v)
			}
		}
	}
	liste := func(ziel *[]string, schluessel, umgebung string) {
		if v, ok := hol(schluessel, umgebung); ok {
			var out []string
			for _, teil := range strings.Split(v, ",") {
				if t := strings.TrimSpace(teil); t != "" {
					out = append(out, t)
				}
			}
			*ziel = out
		}
	}

	text(&k.Port, "port", "PORT")
	text(&k.DatenVerzeich, "daten_verzeichnis", "NEXORA_DATA_DIR")
	text(&k.OeffentlicheURL, "oeffentliche_url", "NEXORA_PUBLIC_URL")
	text(&k.DatenbankURL, "datenbank_url", "DATABASE_URL")
	text(&k.JWTGeheimnis, "jwt_geheimnis", "JWT_SECRET")
	zahl(&k.SitzungTage, "sitzung_tage", "NEXORA_SESSION_DAYS")
	text(&k.Lizenz, "lizenz", "NEXORA_LIZENZ")

	jaNein(&k.RegistrierungOffen, "registrierung_offen", "NEXORA_REGISTRIERUNG_OFFEN")
	liste(&k.ErlaubteDomaenen, "erlaubte_domaenen", "NEXORA_ERLAUBTE_DOMAENEN")

	text(&k.SuchWoerterbuch, "such_woerterbuch", "NEXORA_SUCH_WOERTERBUCH")
	zahl(&k.MaxAnhangMB, "max_anhang_mb", "NEXORA_MAX_ANHANG_MB")

	jaNein(&k.S3Aktiv, "s3_aktiv", "NEXORA_S3_AKTIV")
	text(&k.S3Endpunkt, "s3_endpunkt", "NEXORA_S3_ENDPUNKT")
	text(&k.S3Bucket, "s3_bucket", "NEXORA_S3_BUCKET")
	text(&k.S3Zugriff, "s3_zugriffsschluessel", "NEXORA_S3_ZUGRIFFSSCHLUESSEL")
	text(&k.S3Geheimnis, "s3_geheimnis", "NEXORA_S3_GEHEIMNIS")
	text(&k.S3Region, "s3_region", "NEXORA_S3_REGION")
	jaNein(&k.S3TLS, "s3_tls", "NEXORA_S3_TLS")
	jaNein(&k.S3Pfadstil, "s3_pfadstil", "NEXORA_S3_PFADSTIL")

	jaNein(&k.LDAPAktiv, "ldap_aktiv", "NEXORA_LDAP_AKTIV")
	text(&k.LDAPServer, "ldap_server", "NEXORA_LDAP_SERVER")
	jaNein(&k.LDAPStartTLS, "ldap_starttls", "NEXORA_LDAP_STARTTLS")
	jaNein(&k.LDAPTLSPruefen, "ldap_tls_pruefen", "NEXORA_LDAP_TLS_PRUEFEN")
	text(&k.LDAPBindDN, "ldap_bind_dn", "NEXORA_LDAP_BIND_DN")
	text(&k.LDAPBindPasswort, "ldap_bind_passwort", "NEXORA_LDAP_BIND_PASSWORT")
	text(&k.LDAPBasisDN, "ldap_basis_dn", "NEXORA_LDAP_BASIS_DN")
	text(&k.LDAPBenutzerFilter, "ldap_benutzer_filter", "NEXORA_LDAP_BENUTZER_FILTER")
	text(&k.LDAPFeldName, "ldap_feld_name", "NEXORA_LDAP_FELD_NAME")
	text(&k.LDAPFeldEmail, "ldap_feld_email", "NEXORA_LDAP_FELD_EMAIL")
	text(&k.LDAPGruppeAdmin, "ldap_gruppe_admin", "NEXORA_LDAP_GRUPPE_ADMIN")

	jaNein(&k.OIDCAktiv, "oidc_aktiv", "NEXORA_OIDC_AKTIV")
	text(&k.OIDCAussteller, "oidc_aussteller", "NEXORA_OIDC_AUSSTELLER")
	text(&k.OIDCClientID, "oidc_client_id", "NEXORA_OIDC_CLIENT_ID")
	text(&k.OIDCGeheimnis, "oidc_geheimnis", "NEXORA_OIDC_GEHEIMNIS")
	text(&k.OIDCBereiche, "oidc_bereiche", "NEXORA_OIDC_BEREICHE")
	text(&k.OIDCFeldName, "oidc_feld_name", "NEXORA_OIDC_FELD_NAME")
	text(&k.OIDCFeldEmail, "oidc_feld_email", "NEXORA_OIDC_FELD_EMAIL")
	text(&k.OIDCGruppeAdmin, "oidc_gruppe_admin", "NEXORA_OIDC_GRUPPE_ADMIN")
	text(&k.OIDCKnopfText, "oidc_knopf_text", "NEXORA_OIDC_KNOPF_TEXT")

	return k
}

// datei parses one config file. The format is deliberately dull: key = value,
// one per line, # or ; starts a comment, blank lines ignored. Values may be
// quoted to keep leading or trailing spaces, and [sections] are read and
// discarded -- they exist to structure the file for a human, not for the parser.
func datei(pfad string) (map[string]string, error) {
	f, err := os.Open(pfad)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	werte, _, err := lesen(f, pfad)
	return werte, err
}

// lesen ist der eigentliche Parser. Getrennt von datei, weil die
// Einstellungsseite einen Entwurf prüfen können muss, BEVOR er auf der Platte
// landet -- eine Konfiguration erst zu schreiben und dann festzustellen, dass
// sie kaputt ist, hiesse: beim nächsten Start startet nichts mehr.
//
// Die zweite Rückgabe sind Beanstandungen: Zeilen ohne '=' und doppelte
// Schlüssel. Sie sind keine Fehler -- die Datei bleibt lesbar --, aber fast
// immer ein Versehen.
func lesen(r io.Reader, name string) (map[string]string, []string, error) {
	werte := map[string]string{}
	beanstandet := []string{}
	s := bufio.NewScanner(r)
	// Ein LDAP-Filter oder eine lange URL sprengt die Vorgabe von 64 KiB nicht,
	// ein versehentlich hier abgelegtes Zertifikat schon.
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	zeilennr := 0
	for s.Scan() {
		zeilennr++
		zeile := strings.TrimSpace(s.Text())
		if zeile == "" || strings.HasPrefix(zeile, "#") || strings.HasPrefix(zeile, ";") {
			continue
		}
		if strings.HasPrefix(zeile, "[") {
			continue // Abschnitt, nur zur Gliederung
		}
		i := strings.Index(zeile, "=")
		if i < 0 {
			log.Printf("Konfiguration %s Zeile %d: kein '=', übersprungen", name, zeilennr)
			beanstandet = append(beanstandet,
				fmt.Sprintf("Zeile %d: kein '=' -- wird übersprungen", zeilennr))
			continue
		}
		schluessel := strings.ToLower(strings.TrimSpace(zeile[:i]))
		wert := strings.TrimSpace(zeile[i+1:])
		// Anführungszeichen erlauben Werte mit Rand-Leerzeichen; ein Passwort
		// darf schließlich auf ein Leerzeichen enden.
		if len(wert) >= 2 && wert[0] == '"' && wert[len(wert)-1] == '"' {
			wert = wert[1 : len(wert)-1]
		}
		if schluessel != "" {
			if _, doppelt := werte[schluessel]; doppelt {
				beanstandet = append(beanstandet,
					fmt.Sprintf("Zeile %d: %s steht schon weiter oben -- der letzte Wert gilt",
						zeilennr, schluessel))
			}
			werte[schluessel] = wert
		}
	}
	if err := s.Err(); err != nil {
		return nil, beanstandet, fmt.Errorf("%s: %w", name, err)
	}
	return werte, beanstandet, nil
}

// merkeSchluessel sammelt, welche Schlüssel Laden überhaupt auswertet.
//
// Die Liste wird nicht von Hand gepflegt, sondern beim Laden nebenbei
// aufgeschrieben. Eine Liste, die man von Hand nachträgt, wäre spätestens beim
// dritten neuen Schlüssel unvollständig -- und dann meldete die
// Einstellungsseite einen richtig geschriebenen Eintrag als Tippfehler.
var schluesselWacht struct {
	sync.Mutex
	gesehen map[string]bool
	folge   []string
}

func merkeSchluessel(k string) {
	schluesselWacht.Lock()
	defer schluesselWacht.Unlock()
	if schluesselWacht.gesehen == nil {
		schluesselWacht.gesehen = map[string]bool{}
	}
	if !schluesselWacht.gesehen[k] {
		schluesselWacht.gesehen[k] = true
		schluesselWacht.folge = append(schluesselWacht.folge, k)
	}
}

// BekannteSchluessel liefert alle Schlüssel, die Laden auswertet. Gefüllt ist
// die Liste, sobald Laden einmal gelaufen ist -- und das ist beim Start immer
// der Fall.
func BekannteSchluessel() []string {
	schluesselWacht.Lock()
	defer schluesselWacht.Unlock()
	out := make([]string, len(schluesselWacht.folge))
	copy(out, schluesselWacht.folge)
	sort.Strings(out)
	return out
}

// Pruefen liest einen Entwurf und sagt, was daran auffällt: kaputte Zeilen,
// doppelte Schlüssel und Namen, die das Programm nicht kennt.
//
// Ein unbekannter Schlüssel ist kein Fehler -- die Datei darf mehr enthalten,
// als diese Fassung auswertet. Er ist aber fast immer ein Tippfehler, und ein
// Tippfehler in einer Konfiguration wirkt genau wie eine Einstellung, die man
// nie vorgenommen hat: er tut nichts und sagt nichts.
func Pruefen(inhalt string) []string {
	werte, beanstandet, err := lesen(strings.NewReader(inhalt), "Entwurf")
	if err != nil {
		return append(beanstandet, "nicht lesbar: "+err.Error())
	}
	bekannt := map[string]bool{}
	for _, k := range BekannteSchluessel() {
		bekannt[k] = true
	}
	// Nur prüfen, wenn die Liste überhaupt gefüllt ist. Sonst wäre nach einem
	// Start ohne Laden jeder Schlüssel unbekannt.
	if len(bekannt) > 0 {
		var unbekannt []string
		for k := range werte {
			if !bekannt[k] {
				unbekannt = append(unbekannt, k)
			}
		}
		sort.Strings(unbekannt)
		for _, k := range unbekannt {
			beanstandet = append(beanstandet,
				fmt.Sprintf("%s kennt diese Fassung nicht -- Tippfehler?", k))
		}
	}
	return beanstandet
}

// Warnungen reports settings that are dangerous in production. They are logged
// rather than fatal: a homelab install with the default secret should still
// start, it should just be impossible to miss that it did.
func (k Konfig) Warnungen() []string {
	// Leere Liste, nicht nil: eine nil-Liste wird zu JSON null, und ein Leser,
	// der darauf .length aufruft, stürzt ab. Genau das ist der
	// Einstellungsseite passiert, solange nichts zu bemängeln war.
	w := []string{}
	if k.JWTGeheimnis == "change-me-in-production" {
		w = append(w, "jwt_geheimnis steht auf der Vorgabe -- jede Sitzung ist fälschbar")
	}
	if strings.Contains(k.DatenbankURL, "nexora:nexora@") {
		w = append(w, "datenbank_url benutzt das Vorgabepasswort")
	}
	if k.OIDCAktiv && k.OeffentlicheURL == "" {
		w = append(w, "oidc_aktiv ohne oeffentliche_url -- die Rücksprungadresse lässt sich nicht bilden")
	}
	if k.S3Aktiv && k.S3Endpunkt == "" {
		w = append(w, "s3_aktiv ohne s3_endpunkt -- Anhänge landen weiter auf der Platte")
	}
	if k.S3Aktiv && !k.S3TLS {
		w = append(w, "S3 ohne TLS -- Zugangsschlüssel und Dateien gehen unverschlüsselt über das Netz")
	}
	if k.LDAPAktiv && k.LDAPServer == "" {
		w = append(w, "ldap_aktiv ohne ldap_server -- die Anmeldung fällt auf Passwörter zurück")
	}
	if k.LDAPAktiv && !k.LDAPStartTLS && !strings.HasPrefix(k.LDAPServer, "ldaps://") {
		w = append(w, "LDAP ohne TLS -- Zugangsdaten gehen im Klartext über das Netz")
	}
	if k.LDAPAktiv && !k.LDAPTLSPruefen {
		w = append(w, "ldap_tls_pruefen=nein -- das Serverzertifikat wird nicht geprüft")
	}
	return w
}
