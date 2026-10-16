package middleware

import (
	"net/http"
	"strings"
)

// AltePfade translates the German path names and query parameters the API
// used up to 2.3 into the English ones it uses now.
//
// It runs before routing, so a script that still calls /api/pruefspur?tage=7
// lands exactly where /api/audit-log?days=7 lands. Only fixed path segments are
// translated; ids and tokens never collide with these words. Bodies are not
// touched: their field names changed in 3.0 without a fallback.
func AltePfade(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			teile := strings.Split(r.URL.Path, "/")
			geaendert := false
			for i, t := range teile {
				if neu, ok := altePfadteile[t]; ok {
					teile[i] = neu
					geaendert = true
				}
			}
			if geaendert {
				r.URL.Path = strings.Join(teile, "/")
				r.URL.RawPath = ""
			}
			if r.URL.RawQuery != "" {
				q := r.URL.Query()
				umbenannt := false
				for alt, neu := range alteParameter {
					if v, ok := q[alt]; ok {
						if _, schon := q[neu]; !schon {
							q[neu] = v
						}
						delete(q, alt)
						umbenannt = true
					}
				}
				if umbenannt {
					r.URL.RawQuery = q.Encode()
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

var altePfadteile = map[string]string{
	"ablage":            "storage",
	"aktionen":          "actions",
	"an":                "enable",
	"anhangindex":       "attachment-index",
	"anmeldungen":       "sign-ins",
	"anzahl":            "count",
	"aus":               "disable",
	"ausstellen":        "issue",
	"benachrichtigung":  "notifications",
	"benutzername":      "username",
	"bild":              "image",
	"breite":            "width",
	"dateien":           "files",
	"echtzeit":          "realtime",
	"einstellungen":     "settings",
	"erledigt":          "resolved",
	"erwaehnbare":       "mentionable",
	"farbe":             "color",
	"gelesen":           "read",
	"grenzprobe":        "size-probe",
	"gruppen":           "groups",
	"kommentare":        "comments",
	"konfig":            "config",
	"konto":             "account",
	"lizenz":            "license",
	"mitglieder":        "members",
	"mitschreibende":    "collaborators",
	"mitschrift":        "collab",
	"neustart":          "restart",
	"oeffentlich":       "public-access",
	"papierkorb":        "trash",
	"passwort":          "password",
	"postfach":          "inbox",
	"profil":            "profile",
	"pruefen":           "verify",
	"pruefspur":         "audit-log",
	"puls":              "pulse",
	"rechner":           "hosts",
	"rechte":            "permissions",
	"reihenfolge":       "order",
	"sicherung":         "backup",
	"sitzungen":         "sessions",
	"sprache":           "language",
	"suchindex":         "search-index",
	"umfang":            "scope",
	"wiederherstellung": "restore",
	"zurueck":           "callback",
	"zweitfaktor":       "two-factor",
}

var alteParameter = map[string]string{
	"akteur":     "actor",
	"aktion":     "action",
	"nur":        "only",
	"objekt":     "object",
	"schluessel": "key",
	"tage":       "days",
	"ungelesen":  "unread",
	"wer":        "who",
	"ziel":       "target",
}
