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
	// Pfad is the file that was read, empty when none was found. The settings
	// page shows and edits exactly this file; without the path it would have to
	// guess, and guessing means editing the wrong one.
	Pfad string

	// Server
	Port          string
	DatenVerzeich string
	// AnhangVerzeich is where the uploaded files go, separately from the rest
	// of the data directory. Attachments are the only part that grows without
	// bound, so they often belong somewhere else than the handful of
	// configuration backups: on a disk of their own, a share, a mounted volume.
	// Empty means the same directory as before, so an existing installation
	// still finds its files after an upgrade.
	AnhangVerzeich  string
	OeffentlicheURL string

	// Datenbank
	DatenbankURL string

	// Sitzungen
	JWTGeheimnis   string
	SitzungStunden int

	// Lizenz
	Lizenz string

	// Registrierung
	RegistrierungOffen bool
	ErlaubteDomaenen   []string

	// Search
	SuchWoerterbuch string

	// Attachments
	MaxAnhangMB int

	// Trash
	// After how many days in the trash a page disappears for good.
	// 0 means never by itself, and then it stays until somebody deletes it.
	PapierkorbTage int

	// Objektspeicher (S3)
	S3Aktiv     bool
	S3Endpunkt  string
	S3Bucket    string
	S3Zugriff   string
	S3Geheimnis string
	S3Region    string
	S3TLS       bool
	S3Pfadstil  bool
	// S3Rueckfall allows attachments to land on the disk after all when the
	// object store does not answer at startup. The default is no: whoever set
	// the store up wants the files there and nowhere else, and an instance
	// quietly writing locally again spreads the attachments over two places
	// without anybody noticing.
	S3Rueckfall bool

	// Redis
	//
	// Optional. Redis is a cache here, not a store: everything in it also stands
	// in the database. Without Redis the application carries on in full.
	RedisAdresse   string
	RedisPasswort  string
	RedisDatenbank int
	RedisVorsilbe  string

	// Kennzahlen für Prometheus. Leer heißt: den Weg gibt es nicht. Ein
	// Losungswort und kein Ja/Nein, weil die Zahlen verraten, wie viele Leute
	// hier arbeiten und wann; wer sie abholt, soll sich ausweisen.
	MetrikenToken string

	// LDAP / Active Directory
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

	// OIDC / Keycloak
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
		AnhangVerzeich:  "",
		OeffentlicheURL: "",
		DatenbankURL:    "postgres://nexora:nexora@localhost:5432/nexora?sslmode=disable",
		JWTGeheimnis:    "change-me-in-production",
		SitzungStunden:  12,
		Lizenz:          "",

		RegistrierungOffen: true,
		ErlaubteDomaenen:   nil,

		SuchWoerterbuch: "german",
		MaxAnhangMB:     25,
		PapierkorbTage:  30,

		S3Aktiv:     false,
		S3Bucket:    "nexora",
		S3Region:    "us-east-1",
		S3TLS:       false,
		S3Pfadstil:  true,
		S3Rueckfall: false,

		RedisVorsilbe: "nexora",

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
		log.Printf("Keine config.conf gefunden. Vorgaben und Umgebungsvariablen gelten.")
	}

	hol := func(schluessel, umgebung string) (string, bool) {
		merkeSchluessel(schluessel)
		// The environment beats the file. That way a single value can be
		// overridden inside a container without touching the file, and a secret
		// never has to reach the disk.
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
	text(&k.AnhangVerzeich, "anhang_verzeichnis", "NEXORA_ANHANG_PFAD")
	text(&k.OeffentlicheURL, "oeffentliche_url", "NEXORA_PUBLIC_URL")
	text(&k.DatenbankURL, "datenbank_url", "DATABASE_URL")
	text(&k.JWTGeheimnis, "jwt_geheimnis", "JWT_SECRET")
	zahl(&k.SitzungStunden, "sitzung_stunden", "NEXORA_SESSION_HOURS")
	// The old key in days is still read and converted. An existing file must not
	// silently drop from seven days to twelve hours just because the unit
	// changed.
	var alteTage int
	zahl(&alteTage, "sitzung_tage", "NEXORA_SESSION_DAYS")
	if alteTage > 0 {
		k.SitzungStunden = alteTage * 24
	}
	text(&k.Lizenz, "lizenz", "NEXORA_LIZENZ")

	jaNein(&k.RegistrierungOffen, "registrierung_offen", "NEXORA_REGISTRIERUNG_OFFEN")
	liste(&k.ErlaubteDomaenen, "erlaubte_domaenen", "NEXORA_ERLAUBTE_DOMAENEN")

	text(&k.SuchWoerterbuch, "such_woerterbuch", "NEXORA_SUCH_WOERTERBUCH")
	zahl(&k.MaxAnhangMB, "max_anhang_mb", "NEXORA_MAX_ANHANG_MB")
	zahl(&k.PapierkorbTage, "papierkorb_tage", "NEXORA_PAPIERKORB_TAGE")

	jaNein(&k.S3Aktiv, "s3_aktiv", "NEXORA_S3_AKTIV")
	text(&k.S3Endpunkt, "s3_endpunkt", "NEXORA_S3_ENDPUNKT")
	text(&k.S3Bucket, "s3_bucket", "NEXORA_S3_BUCKET")
	text(&k.S3Zugriff, "s3_zugriffsschluessel", "NEXORA_S3_ZUGRIFFSSCHLUESSEL")
	text(&k.S3Geheimnis, "s3_geheimnis", "NEXORA_S3_GEHEIMNIS")
	text(&k.S3Region, "s3_region", "NEXORA_S3_REGION")
	jaNein(&k.S3TLS, "s3_tls", "NEXORA_S3_TLS")
	jaNein(&k.S3Pfadstil, "s3_pfadstil", "NEXORA_S3_PFADSTIL")
	jaNein(&k.S3Rueckfall, "s3_rueckfall", "NEXORA_S3_RUECKFALL")

	text(&k.RedisAdresse, "redis_adresse", "NEXORA_REDIS_ADRESSE")
	text(&k.RedisPasswort, "redis_passwort", "NEXORA_REDIS_PASSWORT")
	zahl(&k.RedisDatenbank, "redis_datenbank", "NEXORA_REDIS_DATENBANK")
	text(&k.RedisVorsilbe, "redis_vorsilbe", "NEXORA_REDIS_VORSILBE")

	text(&k.MetrikenToken, "metriken_token", "NEXORA_METRIKEN_TOKEN")

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
// discarded, they exist to structure the file for a human, not for the parser.
func datei(pfad string) (map[string]string, error) {
	f, err := os.Open(pfad)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	werte, _, err := lesen(f, pfad)
	return werte, err
}

// lesen is the actual parser. Kept apart from datei because the settings page
// has to be able to check a draft BEFORE it reaches the disk: writing a
// configuration first and finding out afterwards that it is broken would mean
// nothing starts next time.
//
// The second return value holds the complaints: lines without '=' and duplicate
// keys. They are not errors, the file stays readable, but they are almost always
// a slip.
func lesen(r io.Reader, name string) (map[string]string, []string, error) {
	werte := map[string]string{}
	beanstandet := []string{}
	s := bufio.NewScanner(r)
	// An LDAP filter or a long URL does not exceed the 64 KiB default; a
	// certificate accidentally dropped in here does.
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	zeilennr := 0
	for s.Scan() {
		zeilennr++
		zeile := strings.TrimSpace(s.Text())
		if zeile == "" || strings.HasPrefix(zeile, "#") || strings.HasPrefix(zeile, ";") {
			continue
		}
		if strings.HasPrefix(zeile, "[") {
			continue // a section header, purely for grouping
		}
		i := strings.Index(zeile, "=")
		if i < 0 {
			log.Printf("Konfiguration %s Zeile %d: kein '=', übersprungen", name, zeilennr)
			beanstandet = append(beanstandet,
				fmt.Sprintf("Zeile %d: kein '=', wird übersprungen", zeilennr))
			continue
		}
		schluessel := strings.ToLower(strings.TrimSpace(zeile[:i]))
		wert := strings.TrimSpace(zeile[i+1:])
		// Quotes allow values with leading or trailing spaces; a password may
		// after all end in one.
		if len(wert) >= 2 && wert[0] == '"' && wert[len(wert)-1] == '"' {
			wert = wert[1 : len(wert)-1]
		}
		if schluessel != "" {
			if _, doppelt := werte[schluessel]; doppelt {
				beanstandet = append(beanstandet,
					fmt.Sprintf("Zeile %d: %s steht schon weiter oben, der letzte Wert gilt",
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

// merkeSchluessel collects which keys Laden actually evaluates.
//
// The list is not maintained by hand but written down in passing while loading.
// A hand-maintained list would be incomplete by the third new key at the latest,
// and the settings page would then report a correctly spelled entry as a typo.
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

// BekannteSchluessel returns every key Laden evaluates. The list is filled once
// Laden has run, which at startup is always the case.
func BekannteSchluessel() []string {
	schluesselWacht.Lock()
	defer schluesselWacht.Unlock()
	out := make([]string, len(schluesselWacht.folge))
	copy(out, schluesselWacht.folge)
	sort.Strings(out)
	return out
}

// Pruefen reads a draft and reports what stands out about it: broken lines,
// duplicate keys and names the program does not know.
//
// An unknown key is not an error, since the file may contain more than this
// version evaluates. It is almost always a typo, though, and a typo in a
// configuration behaves exactly like a setting nobody ever made: it does
// nothing and says nothing.
func Pruefen(inhalt string) []string {
	werte, beanstandet, err := lesen(strings.NewReader(inhalt), "Entwurf")
	if err != nil {
		return append(beanstandet, "nicht lesbar: "+err.Error())
	}
	bekannt := map[string]bool{}
	for _, k := range BekannteSchluessel() {
		bekannt[k] = true
	}
	// Only check when the list is filled at all. Otherwise every key would count
	// as unknown after a start without Laden.
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
				fmt.Sprintf("%s kennt diese Fassung nicht. Tippfehler?", k))
		}
	}
	return beanstandet
}

// AnhangOrt is the directory the attachments lie in.
//
// Two keys point at it: the new anhang_verzeichnis and, as it always has,
// daten_verzeichnis. The second one is what every installation so far has set,
// so it stays the fallback -- an upgrade must not move the files out from under
// a running instance.
func (k Konfig) AnhangOrt() string {
	if strings.TrimSpace(k.AnhangVerzeich) != "" {
		return strings.TrimSpace(k.AnhangVerzeich)
	}
	return k.DatenVerzeich
}

// Warnungen reports settings that are dangerous in production. They are logged
// rather than fatal: a homelab install with the default secret should still
// start, it should just be impossible to miss that it did.
func (k Konfig) Warnungen() []string {
	// An empty list, not nil: a nil slice becomes JSON null, and a reader calling
	// .length on it crashes. That is exactly what happened to the settings page
	// while there was nothing to complain about.
	w := []string{}
	if k.JWTGeheimnis == "change-me-in-production" {
		w = append(w, "jwt_geheimnis steht auf der Vorgabe, jede Sitzung ist fälschbar")
	}
	if strings.Contains(k.DatenbankURL, "nexora:nexora@") {
		w = append(w, "datenbank_url benutzt das Vorgabepasswort")
	}
	if k.OIDCAktiv && k.OeffentlicheURL == "" {
		w = append(w, "oidc_aktiv ohne oeffentliche_url, die Rücksprungadresse lässt sich nicht bilden")
	}
	if k.S3Aktiv && k.S3Endpunkt == "" {
		w = append(w, "s3_aktiv ohne s3_endpunkt, Anhänge landen weiter auf der Platte")
	}
	if k.S3Aktiv && k.S3Rueckfall {
		w = append(w, "s3_rueckfall=ja, bei einer Störung des Objektspeichers landen neue Anhänge doch auf der Platte")
	}
	if k.S3Aktiv && !k.S3TLS {
		w = append(w, "S3 ohne TLS, Zugangsschlüssel und Dateien gehen unverschlüsselt über das Netz")
	}
	if k.LDAPAktiv && k.LDAPServer == "" {
		w = append(w, "ldap_aktiv ohne ldap_server, die Anmeldung fällt auf Passwörter zurück")
	}
	if k.LDAPAktiv && !k.LDAPStartTLS && !strings.HasPrefix(k.LDAPServer, "ldaps://") {
		w = append(w, "LDAP ohne TLS, Zugangsdaten gehen im Klartext über das Netz")
	}
	if k.LDAPAktiv && !k.LDAPTLSPruefen {
		w = append(w, "ldap_tls_pruefen=nein, das Serverzertifikat wird nicht geprüft")
	}
	return w
}
