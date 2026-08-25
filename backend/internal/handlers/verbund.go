// The stack around this service: the other services it talks to.
//
// One remark up front, because it explains the shape of this file: Nexora does
// NOT see the Docker stack. The service runs in a container with no access to
// the Docker control socket, and that is on purpose, because whoever holds that
// socket is all-powerful on the host. Handing it in so that a page can list
// containers would be a bad bargain.
//
// So what stands here is what can be established without it, and that is more
// than it sounds: which services this one speaks to, whether they answer, how
// fast, which version they run and how much they hold. Those are the questions
// people actually ask when something is stuck.
package handlers

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"nexora/internal/config"
)

// startZeit remembers when this process came up.
var startZeit = time.Now()

// Dienst is one part of the stack as the interface shows it.
type Dienst struct {
	Name    string `json:"name"`
	Rolle   string `json:"rolle"`
	Adresse string `json:"adresse"`
	// One of "läuft", "fehlt", "nicht eingerichtet". The values stay German
	// because they are shown as they are.
	Zustand   string `json:"zustand"`
	Fassung   string `json:"fassung,omitempty"`
	Antwort   string `json:"antwort,omitempty"` // gemessene Zeit
	Hinweis   string `json:"hinweis,omitempty"`
	Notwendig bool   `json:"notwendig"`
}

// verbund assembles the list of services. Every check has a short deadline: the
// overview has to appear even when one of the services is hanging, and
// especially then.
func (s *Server) verbund(ctx context.Context) []Dienst {
	ctx, abbruch := context.WithTimeout(ctx, 5*time.Second)
	defer abbruch()

	speicher.RLock()
	k := speicher.basis
	speicher.RUnlock()

	dienste := []Dienst{s.dienstSelbst()}

	// The database.
	d := Dienst{Name: "PostgreSQL", Rolle: "Datenbank",
		Adresse: ohneGeheimnis(k.DatenbankURL), Notwendig: true}
	beginn := time.Now()
	var version string
	if err := s.Pool.QueryRow(ctx, `SHOW server_version`).Scan(&version); err != nil {
		d.Zustand = "fehlt"
		d.Hinweis = kurz(err.Error())
	} else {
		d.Zustand = "läuft"
		d.Fassung = version
		d.Antwort = dauer(time.Since(beginn))
		var verbindungen int
		if err := s.Pool.QueryRow(ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()`).
			Scan(&verbindungen); err == nil {
			d.Hinweis = fmt.Sprintf("%d %s", verbindungen,
				mehrzahl(verbindungen, "offene Verbindung", "offene Verbindungen"))
		}
	}
	dienste = append(dienste, d)

	dienste = append(dienste, s.dienstRedis(ctx, k.RedisAdresse))
	dienste = append(dienste, s.dienstAblage(ctx, k))
	if oidc := dienstOIDC(ctx, k); oidc != nil {
		dienste = append(dienste, *oidc)
	}
	if ldap := dienstLDAP(ctx, k); ldap != nil {
		dienste = append(dienste, *ldap)
	}
	return dienste
}

func (s *Server) dienstSelbst() Dienst {
	rechner, _ := os.Hostname()
	return Dienst{
		Name:    "Nexora",
		Rolle:   "dieser Dienst",
		Adresse: rechner,
		Zustand: "läuft",
		Fassung: runtime.Version(),
		Hinweis: fmt.Sprintf("seit %s, %d %s", dauer(time.Since(startZeit)),
			runtime.NumGoroutine(), mehrzahl(runtime.NumGoroutine(), "Faden", "Fäden")),
		Notwendig: true,
	}
}

func (s *Server) dienstRedis(ctx context.Context, adresse string) Dienst {
	d := Dienst{Name: "Redis", Rolle: "Zwischenspeicher", Adresse: adresse}
	if strings.TrimSpace(adresse) == "" {
		d.Zustand = "nicht eingerichtet"
		d.Hinweis = "Ohne ihn wird jede Sitzung in der Datenbank nachgeschlagen."
		return d
	}
	if s.Redis == nil {
		d.Zustand = "fehlt"
		d.Hinweis = "Beim Start nicht erreichbar. Die Anwendung läuft ohne ihn weiter."
		return d
	}
	beginn := time.Now()
	if err := s.Redis.client.Ping(ctx).Err(); err != nil {
		d.Zustand = "fehlt"
		d.Hinweis = kurz(err.Error())
		return d
	}
	d.Zustand = "läuft"
	d.Antwort = dauer(time.Since(beginn))
	if info, err := s.Redis.client.Info(ctx, "server", "memory").Result(); err == nil {
		d.Fassung = ausInfo(info, "redis_version")
		if m := ausInfo(info, "used_memory_human"); m != "" {
			d.Hinweis = m + " belegt"
		}
	}
	if n, err := s.Redis.client.DBSize(ctx).Result(); err == nil {
		if d.Hinweis != "" {
			d.Hinweis += ", "
		}
		d.Hinweis += fmt.Sprintf("%d %s", n, mehrzahl(int(n), "Eintrag", "Einträge"))
	}
	return d
}

func (s *Server) dienstAblage(ctx context.Context, k config.Konfig) Dienst {
	d := Dienst{Name: "Objektspeicher", Rolle: "Anhänge"}
	if !k.S3Aktiv || k.S3Endpunkt == "" {
		d.Name = "Platte"
		d.Rolle = "Anhänge auf der Platte"
		d.Adresse = k.DatenVerzeich
		d.Zustand = "läuft"
		d.Hinweis = "Kein Objektspeicher eingerichtet."
		d.Notwendig = true
		return d
	}
	d.Adresse = k.S3Endpunkt + "/" + k.S3Bucket
	d.Notwendig = true
	beginn := time.Now()
	if erreichbar(ctx, k.S3Endpunkt) {
		d.Zustand = "läuft"
		d.Antwort = dauer(time.Since(beginn))
	} else {
		d.Zustand = "fehlt"
		d.Hinweis = "Der Endpunkt antwortet nicht. Neue Anhänge scheitern."
	}
	if !k.S3TLS {
		if d.Hinweis != "" {
			d.Hinweis += " "
		}
		d.Hinweis += "Ohne TLS: Zugangsdaten gehen im Klartext über das Netz."
	}
	return d
}

func dienstOIDC(ctx context.Context, k config.Konfig) *Dienst {
	if !k.OIDCAktiv || k.OIDCAussteller == "" {
		return nil
	}
	d := Dienst{Name: "OIDC-Anbieter", Rolle: "Anmeldung", Adresse: k.OIDCAussteller}
	beginn := time.Now()
	if erreichbar(ctx, k.OIDCAussteller) {
		d.Zustand = "läuft"
		d.Antwort = dauer(time.Since(beginn))
	} else {
		d.Zustand = "fehlt"
		d.Hinweis = "Anmeldung über SSO schlägt fehl, das Passwort geht weiter."
	}
	return &d
}

func dienstLDAP(ctx context.Context, k config.Konfig) *Dienst {
	if !k.LDAPAktiv || k.LDAPServer == "" {
		return nil
	}
	d := Dienst{Name: "Verzeichnis", Rolle: "Anmeldung", Adresse: k.LDAPServer}
	beginn := time.Now()
	if erreichbar(ctx, k.LDAPServer) {
		d.Zustand = "läuft"
		d.Antwort = dauer(time.Since(beginn))
	} else {
		d.Zustand = "fehlt"
		d.Hinweis = "Anmeldung über das Verzeichnis schlägt fehl."
	}
	return &d
}

// erreichbar opens a connection briefly. Deliberately not an HTTP request: for
// this question a 401 or a 404 answers just as well as a 200, because all that
// is being asked is whether anybody is there at all.
func erreichbar(ctx context.Context, adresse string) bool {
	ziel := adresse
	if u, err := url.Parse(adresse); err == nil && u.Host != "" {
		ziel = u.Host
		if u.Port() == "" {
			switch u.Scheme {
			case "https", "ldaps":
				ziel = u.Hostname() + ":443"
			case "ldap":
				ziel = u.Hostname() + ":389"
			default:
				ziel = u.Hostname() + ":80"
			}
		}
	}
	waehler := net.Dialer{Timeout: 2 * time.Second}
	verbindung, err := waehler.DialContext(ctx, "tcp", ziel)
	if err != nil {
		return false
	}
	verbindung.Close()
	return true
}

// ohneGeheimnis strips the password from a connection string. The address
// belongs in the overview, the password does not.
func ohneGeheimnis(adresse string) string {
	u, err := url.Parse(adresse)
	if err != nil || u.User == nil {
		return adresse
	}
	u.User = url.User(u.User.Username())
	return u.String()
}

func ausInfo(info, feld string) string {
	for _, zeile := range strings.Split(info, "\n") {
		if strings.HasPrefix(zeile, feld+":") {
			return strings.TrimSpace(strings.TrimPrefix(zeile, feld+":"))
		}
	}
	return ""
}

// dauer writes a duration the way a person would read it aloud. The step from
// milliseconds to seconds was missing at first, so an uptime showed up as
// "48023 ms", and nobody reads that.
func dauer(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000)
	case d < 2*time.Second:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	case d < 90*time.Second:
		return fmt.Sprintf("%d s", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	default:
		return fmt.Sprintf("%d Tagen", int(d.Hours()/24))
	}
}

func kurz(s string) string {
	s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if len(s) > 160 {
		return s[:157] + "..."
	}
	return s
}

// mehrzahl picks singular or plural. A "1 Einträge" in an overview looks like a
// bug in the program even when it is not one.
func mehrzahl(n int, eins, viele string) string {
	if n == 1 {
		return eins
	}
	return viele
}
