// Was zum Verbund gehört: die Dienste, mit denen dieser hier redet.
//
// Eine Bemerkung vorweg, weil sie die Form dieser Datei erklärt: Nexora sieht
// den Docker-Verbund NICHT. Der Dienst läuft in einem Container ohne Zugang zum
// Docker-Steuerkanal, und das ist Absicht -- wer diesen Kanal hat, ist auf dem
// Wirt allmächtig. Ihn hereinzureichen, damit eine Seite eine Liste von
// Containern zeigen kann, wäre ein schlechtes Geschäft.
//
// Also steht hier, was sich ohne ihn feststellen lässt, und das ist mehr, als
// es klingt: mit welchen Diensten dieser hier spricht, ob sie antworten, wie
// schnell, welche Fassung sie haben und wie viel sie halten. Das beantwortet
// die Fragen, die man tatsächlich stellt, wenn etwas klemmt.
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

// startZeit merkt sich, wann dieser Prozess hochgekommen ist.
var startZeit = time.Now()

// Dienst ist ein Teil des Verbunds, wie die Oberfläche ihn zeigt.
type Dienst struct {
	Name      string `json:"name"`
	Rolle     string `json:"rolle"`
	Adresse   string `json:"adresse"`
	Zustand   string `json:"zustand"` // "läuft", "fehlt", "nicht eingerichtet"
	Fassung   string `json:"fassung,omitempty"`
	Antwort   string `json:"antwort,omitempty"` // gemessene Zeit
	Hinweis   string `json:"hinweis,omitempty"`
	Notwendig bool   `json:"notwendig"`
}

// verbund stellt die Dienste zusammen. Jede Prüfung hat eine kurze Frist: die
// Übersicht soll auch dann erscheinen, wenn einer der Dienste hängt -- gerade
// dann.
func (s *Server) verbund(ctx context.Context) []Dienst {
	ctx, abbruch := context.WithTimeout(ctx, 5*time.Second)
	defer abbruch()

	speicher.RLock()
	k := speicher.basis
	speicher.RUnlock()

	dienste := []Dienst{s.dienstSelbst()}

	// Datenbank
	d := Dienst{Name: "PostgreSQL", Rolle: "Datenbank -- hier steht alles, was zählt",
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
			d.Hinweis = fmt.Sprintf("%d offene Verbindungen", verbindungen)
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
		Name:      "Nexora",
		Rolle:     "dieser Dienst",
		Adresse:   rechner,
		Zustand:   "läuft",
		Fassung:   runtime.Version(),
		Hinweis:   fmt.Sprintf("seit %s, %d Fäden", dauer(time.Since(startZeit)), runtime.NumGoroutine()),
		Notwendig: true,
	}
}

func (s *Server) dienstRedis(ctx context.Context, adresse string) Dienst {
	d := Dienst{Name: "Redis", Rolle: "Zwischenspeicher -- entbehrlich", Adresse: adresse}
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
		d.Hinweis += fmt.Sprintf("%d Einträge", n)
	}
	return d
}

func (s *Server) dienstAblage(ctx context.Context, k config.Konfig) Dienst {
	d := Dienst{Name: "Objektspeicher", Rolle: "Anhänge"}
	if !k.S3Aktiv || k.S3Endpunkt == "" {
		d.Name = "Platte"
		d.Rolle = "Anhänge -- im Datenverzeichnis"
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
		d.Hinweis = "Der Endpunkt antwortet nicht -- neue Anhänge scheitern."
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

// erreichbar öffnet kurz eine Verbindung. Bewusst kein HTTP-Aufruf: eine
// Antwort mit 401 oder 404 ist für diese Frage genauso gut wie eine mit 200 --
// gefragt ist, ob überhaupt jemand da ist.
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

// ohneGeheimnis nimmt das Passwort aus einer Verbindungsadresse. Die Adresse
// gehört in die Übersicht, das Passwort nicht.
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

func dauer(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000)
	case d < time.Minute:
		return fmt.Sprintf("%d ms", d.Milliseconds())
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
