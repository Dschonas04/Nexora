// Der Benutzername als zweiter Weg an der Anmeldung.
//
// Er ist bewusst schmal gehalten: Kleinbuchstaben, Ziffern, Punkt, Strich und
// Unterstrich. Das ist keine Bequemlichkeit, sondern der Grund, warum er
// ueberhaupt taugt -- er wird getippt, vorgelesen und verglichen, und zwei
// Namen, die sich nur in einem Zeichen unterscheiden, das man nicht sieht,
// waeren an einer Anmeldemaske eine Falle. Ein @ ist ausgeschlossen, sonst
// koennte sich jemand einen Namen nehmen, der wie die Adresse eines anderen
// aussieht.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nexora/internal/middleware"
)

const (
	benutzernameKuerzest = 3
	benutzernameLaengst  = 32
)

var benutzernameErlaubt = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// benutzernamePruefen normalisiert die Eingabe und meldet, was daran nicht
// geht. Der leere String ist kein Fehler: der Name ist freiwillig.
func benutzernamePruefen(eingabe string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(eingabe))
	if name == "" {
		return "", nil
	}
	if strings.Contains(name, "@") {
		return "", fmt.Errorf("ein Benutzername ist keine E-Mail-Adresse, das @ gehoert nicht hinein")
	}
	if len([]rune(name)) < benutzernameKuerzest || len([]rune(name)) > benutzernameLaengst {
		return "", fmt.Errorf("der Benutzername braucht %d bis %d Zeichen",
			benutzernameKuerzest, benutzernameLaengst)
	}
	if !benutzernameErlaubt.MatchString(name) {
		return "", fmt.Errorf("erlaubt sind Buchstaben, Ziffern, Punkt, Strich und Unterstrich, " +
			"beginnend mit einem Buchstaben oder einer Ziffer")
	}
	return name, nil
}

// benutzernameAusAdresse macht aus einer E-Mail einen brauchbaren Vorschlag.
// Was uebrig bleibt, kann zu kurz oder leer sein; dann gibt es eben keinen.
func benutzernameAusAdresse(email string) string {
	vorn := strings.SplitN(strings.ToLower(strings.TrimSpace(email)), "@", 2)[0]
	var b strings.Builder
	for _, r := range vorn {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		}
	}
	name := strings.TrimLeft(b.String(), "._-")
	if len([]rune(name)) < benutzernameKuerzest || len([]rune(name)) > benutzernameLaengst {
		return ""
	}
	return name
}

// freierBenutzername haengt eine Zahl an, bis der Name noch niemandem gehoert.
//
// Der Datenbankindex bleibt die eigentliche Sicherung -- zwischen dieser Frage
// und dem Einfuegen kann sich jemand denselben Namen nehmen. Diese Schleife
// erspart nur den Regelfall, dass zwei Adressen denselben vorderen Teil haben.
func (s *Server) freierBenutzername(ctx context.Context, vorschlag string) string {
	if vorschlag == "" {
		return ""
	}
	for n := 0; n < 50; n++ {
		kandidat := vorschlag
		if n > 0 {
			kandidat = fmt.Sprintf("%s%d", vorschlag, n+1)
		}
		if len([]rune(kandidat)) > benutzernameLaengst {
			return ""
		}
		var da bool
		err := s.Pool.QueryRow(ctx,
			`SELECT exists(SELECT 1 FROM users WHERE lower(benutzername) = $1)`, kandidat).Scan(&da)
		if err != nil {
			return ""
		}
		if !da {
			return kandidat
		}
	}
	return ""
}

// nameSchonVergeben sagt, ob der eindeutige Index ueber den Benutzernamen
// angeschlagen hat und nicht der ueber die Adresse.
func nameSchonVergeben(constraint string) bool {
	return strings.Contains(constraint, "benutzer_name")
}

// leerAlsNull macht aus dem leeren Namen ein NULL. Der eindeutige Index laesst
// beliebig viele NULL zu, aber nur einen leeren String -- ohne diese Umwandlung
// koennte genau ein Konto ohne Benutzernamen bestehen und jedes weitere nicht.
func leerAlsNull(name string) any {
	if name == "" {
		return nil
	}
	return name
}

type benutzernameReq struct {
	Benutzername string `json:"benutzername"`
}

// BenutzernameSetzen aendert den Anmeldenamen eines Kontos.
//
// Erlaubt ist es dem Konto selbst und einem Administrator. Der eigene Name ist
// nichts, wofuer man jemanden fragen muesste, und ein Administrator braucht den
// Griff trotzdem: an einem Konto aus dem Verzeichnis oder aus einer alten
// Fassung kann der Name fehlen, und wer ihn dann setzt, ist nicht immer der
// Besitzer.
func (s *Server) BenutzernameSetzen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	ziel := chi.URLParam(r, "id")
	if ziel != uid && !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur das Konto selbst oder ein Administrator")
		return
	}

	var req benutzernameReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	name, err := benutzernamePruefen(req.Benutzername)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := s.Pool.Exec(r.Context(),
		`UPDATE users SET benutzername = $2 WHERE id = $1`, ziel, leerAlsNull(name))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeErr(w, http.StatusConflict, "dieser Benutzername ist schon vergeben")
			return
		}
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}

	s.spurAusRequest(r, AktNameGeaendert, "konto", ziel, name, nil)
	writeJSON(w, http.StatusOK, map[string]string{"benutzername": name})
}
