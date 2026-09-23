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
	// RedisTLS talks to the cache encrypted. It holds session ids, and those are
	// worth as much as a password.
	RedisTLS bool

	// TLS inside the compound
	//
	// TLSZertifikat and TLSSchluessel turn the service into one that speaks
	// HTTPS. Both empty means: unencrypted as before, which is right for a
	// service that has a counterpart in front of it anyway which takes over the
	// encryption and sits on the same machine.
	TLSZertifikat string
	TLSSchluessel string
	// TLSWurzel is an ADDITIONAL certificate authority for everything this
	// service talks to in turn: database, object store, cache. The public
	// authorities stay valid alongside it, see internal/vertrauen.
	TLSWurzel string

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

	// Mail
	//
	// Optional. With smtp_server and smtp_absender set, every account can have
	// its inbox messages sent by e-mail as well. SMTPVerschluesselung is
	// starttls, tls or keine.
	SMTPServer           string
	SMTPBenutzer         string
	SMTPPasswort         string
	SMTPAbsender         string
	SMTPVerschluesselung string
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

		SMTPVerschluesselung: "starttls",
	}
}

// The keys and environment variables used to be German. They are English now,
// and the old spellings keep working: an installation that has a config.conf or
// a compose file from before must not stop booting over a rename. Each entry is
// "the English name" -> {old key, old environment variable}; a boot that hits
// one of them says so once, naming the new name.
var frueher = map[string][2]string{
	"data_directory":       {"daten_verzeichnis", "NEXORA_DATA_DIR"},
	"attachment_directory": {"anhang_verzeichnis", "NEXORA_ANHANG_PFAD"},
	"public_url":           {"oeffentliche_url", "NEXORA_PUBLIC_URL"},
	"database_url":         {"datenbank_url", "DATABASE_URL"},
	"jwt_secret":           {"jwt_geheimnis", "JWT_SECRET"},
	"session_hours":        {"sitzung_stunden", "NEXORA_SESSION_HOURS"},
	"session_days":         {"sitzung_tage", "NEXORA_SESSION_DAYS"},
	"license":              {"lizenz", "NEXORA_LIZENZ"},
	"registration_open":    {"registrierung_offen", "NEXORA_REGISTRIERUNG_OFFEN"},
	"allowed_domains":      {"erlaubte_domaenen", "NEXORA_ERLAUBTE_DOMAENEN"},
	"search_dictionary":    {"such_woerterbuch", "NEXORA_SUCH_WOERTERBUCH"},
	"max_attachment_mb":    {"max_anhang_mb", "NEXORA_MAX_ANHANG_MB"},
	"trash_days":           {"papierkorb_tage", "NEXORA_PAPIERKORB_TAGE"},
	"s3_enabled":           {"s3_aktiv", "NEXORA_S3_AKTIV"},
	"s3_endpoint":          {"s3_endpunkt", "NEXORA_S3_ENDPUNKT"},
	"s3_access_key":        {"s3_zugriffsschluessel", "NEXORA_S3_ZUGRIFFSSCHLUESSEL"},
	"s3_secret_key":        {"s3_geheimnis", "NEXORA_S3_GEHEIMNIS"},
	"s3_path_style":        {"s3_pfadstil", "NEXORA_S3_PFADSTIL"},
	"s3_fallback":          {"s3_rueckfall", "NEXORA_S3_RUECKFALL"},
	"redis_address":        {"redis_adresse", "NEXORA_REDIS_ADRESSE"},
	"redis_password":       {"redis_passwort", "NEXORA_REDIS_PASSWORT"},
	"redis_database":       {"redis_datenbank", "NEXORA_REDIS_DATENBANK"},
	"redis_prefix":         {"redis_vorsilbe", "NEXORA_REDIS_VORSILBE"},
	"tls_certificate":      {"tls_zertifikat", "NEXORA_TLS_ZERTIFIKAT"},
	"tls_key":              {"tls_schluessel", "NEXORA_TLS_SCHLUESSEL"},
	"tls_root":             {"tls_wurzel", "NEXORA_TLS_WURZEL"},
	"ldap_enabled":         {"ldap_aktiv", "NEXORA_LDAP_AKTIV"},
	"ldap_tls_verify":      {"ldap_tls_pruefen", "NEXORA_LDAP_TLS_PRUEFEN"},
	"ldap_bind_password":   {"ldap_bind_passwort", "NEXORA_LDAP_BIND_PASSWORT"},
	"ldap_base_dn":         {"ldap_basis_dn", "NEXORA_LDAP_BASIS_DN"},
	"ldap_user_filter":     {"ldap_benutzer_filter", "NEXORA_LDAP_BENUTZER_FILTER"},
	"ldap_field_name":      {"ldap_feld_name", "NEXORA_LDAP_FELD_NAME"},
	"ldap_field_email":     {"ldap_feld_email", "NEXORA_LDAP_FELD_EMAIL"},
	"ldap_admin_group":     {"ldap_gruppe_admin", "NEXORA_LDAP_GRUPPE_ADMIN"},
	"oidc_enabled":         {"oidc_aktiv", "NEXORA_OIDC_AKTIV"},
	"oidc_issuer":          {"oidc_aussteller", "NEXORA_OIDC_AUSSTELLER"},
	"oidc_secret":          {"oidc_geheimnis", "NEXORA_OIDC_GEHEIMNIS"},
	"oidc_scopes":          {"oidc_bereiche", "NEXORA_OIDC_BEREICHE"},
	"oidc_field_name":      {"oidc_feld_name", "NEXORA_OIDC_FELD_NAME"},
	"oidc_field_email":     {"oidc_feld_email", "NEXORA_OIDC_FELD_EMAIL"},
	"oidc_admin_group":     {"oidc_gruppe_admin", "NEXORA_OIDC_GRUPPE_ADMIN"},
	"oidc_button_text":     {"oidc_knopf_text", "NEXORA_OIDC_KNOPF_TEXT"},
	"smtp_user":            {"smtp_benutzer", "NEXORA_SMTP_BENUTZER"},
	"smtp_password":        {"smtp_passwort", "NEXORA_SMTP_PASSWORT"},
	"smtp_sender":          {"smtp_absender", "NEXORA_SMTP_ABSENDER"},
	"smtp_encryption":      {"smtp_verschluesselung", "NEXORA_SMTP_VERSCHLUESSELUNG"},
}

// Which old spellings this boot actually read, in the order they were asked for.
var veraltetGenutzt []string

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
		altSchluessel, altUmgebung := "", ""
		if paar, da := frueher[schluessel]; da {
			altSchluessel, altUmgebung = paar[0], paar[1]
		}
		notiere := func(was string) {
			veraltetGenutzt = append(veraltetGenutzt, was+" -> "+schluessel)
		}
		// The environment beats the file. That way a single value can be
		// overridden inside a container without touching the file, and a secret
		// never has to reach the disk. Within each of the two, the English name
		// wins over the old German one.
		if v := os.Getenv(umgebung); v != "" {
			return v, true
		}
		if altUmgebung != "" && altUmgebung != umgebung {
			if v := os.Getenv(altUmgebung); v != "" {
				notiere(altUmgebung)
				return v, true
			}
		}
		if werte != nil {
			if v, ok := werte[schluessel]; ok && v != "" {
				return v, true
			}
			if altSchluessel != "" && altSchluessel != schluessel {
				if v, ok := werte[altSchluessel]; ok && v != "" {
					notiere(altSchluessel)
					return v, true
				}
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
	text(&k.DatenVerzeich, "data_directory", "NEXORA_DATA_DIR")
	text(&k.AnhangVerzeich, "attachment_directory", "NEXORA_ATTACHMENT_PATH")
	text(&k.OeffentlicheURL, "public_url", "NEXORA_PUBLIC_URL")
	text(&k.DatenbankURL, "database_url", "DATABASE_URL")
	text(&k.JWTGeheimnis, "jwt_secret", "JWT_SECRET")
	zahl(&k.SitzungStunden, "session_hours", "NEXORA_SESSION_HOURS")
	// The old key in days is still read and converted. An existing file must not
	// silently drop from seven days to twelve hours just because the unit
	// changed.
	var alteTage int
	zahl(&alteTage, "session_days", "NEXORA_SESSION_DAYS")
	if alteTage > 0 {
		k.SitzungStunden = alteTage * 24
	}
	text(&k.Lizenz, "license", "NEXORA_LICENSE")

	jaNein(&k.RegistrierungOffen, "registration_open", "NEXORA_REGISTRATION_OPEN")
	liste(&k.ErlaubteDomaenen, "allowed_domains", "NEXORA_ALLOWED_DOMAINS")

	text(&k.SuchWoerterbuch, "search_dictionary", "NEXORA_SEARCH_DICTIONARY")
	zahl(&k.MaxAnhangMB, "max_attachment_mb", "NEXORA_MAX_ATTACHMENT_MB")
	zahl(&k.PapierkorbTage, "trash_days", "NEXORA_TRASH_DAYS")

	jaNein(&k.S3Aktiv, "s3_enabled", "NEXORA_S3_ENABLED")
	text(&k.S3Endpunkt, "s3_endpoint", "NEXORA_S3_ENDPOINT")
	text(&k.S3Bucket, "s3_bucket", "NEXORA_S3_BUCKET")
	text(&k.S3Zugriff, "s3_access_key", "NEXORA_S3_ACCESS_KEY")
	text(&k.S3Geheimnis, "s3_secret_key", "NEXORA_S3_SECRET_KEY")
	text(&k.S3Region, "s3_region", "NEXORA_S3_REGION")
	jaNein(&k.S3TLS, "s3_tls", "NEXORA_S3_TLS")
	jaNein(&k.S3Pfadstil, "s3_path_style", "NEXORA_S3_PATH_STYLE")
	jaNein(&k.S3Rueckfall, "s3_fallback", "NEXORA_S3_FALLBACK")

	text(&k.RedisAdresse, "redis_address", "NEXORA_REDIS_ADDRESS")
	text(&k.RedisPasswort, "redis_password", "NEXORA_REDIS_PASSWORD")
	zahl(&k.RedisDatenbank, "redis_database", "NEXORA_REDIS_DATABASE")
	text(&k.RedisVorsilbe, "redis_prefix", "NEXORA_REDIS_PREFIX")
	jaNein(&k.RedisTLS, "redis_tls", "NEXORA_REDIS_TLS")

	text(&k.TLSZertifikat, "tls_certificate", "NEXORA_TLS_CERTIFICATE")
	text(&k.TLSSchluessel, "tls_key", "NEXORA_TLS_KEY")
	text(&k.TLSWurzel, "tls_root", "NEXORA_TLS_ROOT")

	jaNein(&k.LDAPAktiv, "ldap_enabled", "NEXORA_LDAP_ENABLED")
	text(&k.LDAPServer, "ldap_server", "NEXORA_LDAP_SERVER")
	jaNein(&k.LDAPStartTLS, "ldap_starttls", "NEXORA_LDAP_STARTTLS")
	jaNein(&k.LDAPTLSPruefen, "ldap_tls_verify", "NEXORA_LDAP_TLS_VERIFY")
	text(&k.LDAPBindDN, "ldap_bind_dn", "NEXORA_LDAP_BIND_DN")
	text(&k.LDAPBindPasswort, "ldap_bind_password", "NEXORA_LDAP_BIND_PASSWORD")
	text(&k.LDAPBasisDN, "ldap_base_dn", "NEXORA_LDAP_BASE_DN")
	text(&k.LDAPBenutzerFilter, "ldap_user_filter", "NEXORA_LDAP_USER_FILTER")
	text(&k.LDAPFeldName, "ldap_field_name", "NEXORA_LDAP_FIELD_NAME")
	text(&k.LDAPFeldEmail, "ldap_field_email", "NEXORA_LDAP_FIELD_EMAIL")
	text(&k.LDAPGruppeAdmin, "ldap_admin_group", "NEXORA_LDAP_ADMIN_GROUP")

	jaNein(&k.OIDCAktiv, "oidc_enabled", "NEXORA_OIDC_ENABLED")
	text(&k.OIDCAussteller, "oidc_issuer", "NEXORA_OIDC_ISSUER")
	text(&k.OIDCClientID, "oidc_client_id", "NEXORA_OIDC_CLIENT_ID")
	text(&k.OIDCGeheimnis, "oidc_secret", "NEXORA_OIDC_SECRET")
	text(&k.OIDCBereiche, "oidc_scopes", "NEXORA_OIDC_SCOPES")
	text(&k.OIDCFeldName, "oidc_field_name", "NEXORA_OIDC_FIELD_NAME")
	text(&k.OIDCFeldEmail, "oidc_field_email", "NEXORA_OIDC_FIELD_EMAIL")
	text(&k.OIDCGruppeAdmin, "oidc_admin_group", "NEXORA_OIDC_ADMIN_GROUP")
	text(&k.OIDCKnopfText, "oidc_button_text", "NEXORA_OIDC_BUTTON_TEXT")

	text(&k.SMTPServer, "smtp_server", "NEXORA_SMTP_SERVER")
	text(&k.SMTPBenutzer, "smtp_user", "NEXORA_SMTP_USER")
	text(&k.SMTPPasswort, "smtp_password", "NEXORA_SMTP_PASSWORD")
	text(&k.SMTPAbsender, "smtp_sender", "NEXORA_SMTP_SENDER")
	text(&k.SMTPVerschluesselung, "smtp_encryption", "NEXORA_SMTP_ENCRYPTION")

	// Say once which old spellings were read, so a rename can be done in peace
	// instead of after a failure.
	if len(veraltetGenutzt) > 0 {
		log.Printf("Konfiguration: %d alte Schreibweise(n) gelesen, bitte umbenennen: %s",
			len(veraltetGenutzt), strings.Join(veraltetGenutzt, ", "))
	}

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
// Two keys point at it: attachment_directory and, as it always has,
// data_directory. The second one is what every installation so far has set,
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
		w = append(w, "jwt_secret steht auf der Vorgabe, jede Sitzung ist fälschbar")
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
	if k.SMTPServer != "" && k.SMTPAbsender == "" {
		w = append(w, "smtp_server ohne smtp_absender, es werden keine Mails verschickt")
	}
	if k.SMTPServer != "" && strings.EqualFold(strings.TrimSpace(k.SMTPVerschluesselung), "keine") {
		w = append(w, "SMTP ohne Verschlüsselung, Zugangsdaten und Mails gehen im Klartext über das Netz")
	}
	if k.SMTPServer != "" && k.OeffentlicheURL == "" {
		w = append(w, "smtp_server ohne oeffentliche_url, Mails enthalten keinen Link zur Seite")
	}
	return w
}
