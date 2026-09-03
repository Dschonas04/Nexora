// Username as a second login identifier.
//
// It is intentionally narrow: lowercase letters, digits, dot, hyphen and
// underscore. This is not convenience but the reason it is usable at all — it
// is typed, read aloud and compared, and two names that differ only in an
// invisible character would be a pitfall on a login form. An `@` is excluded
// so that a username cannot be taken that looks like another user's email.
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

// benutzernamePruefen normalizes the input and reports validation errors.
// The empty string is not an error: a username is optional.
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

// benutzernameAusAdresse derives a usable suggestion from an email address.
// What remains may be too short or empty; in that case no suggestion is
// returned.
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

// freierBenutzername appends a number until the name is not taken.
//
// The database unique index is the ultimate guarantee — between this check
// and the insert another actor may take the same name. This loop only avoids
// the common case where two addresses share the same local part.
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

// nameSchonVergeben reports whether the unique index on the username was
// violated (as opposed to the unique index on the email address).
func nameSchonVergeben(constraint string) bool {
	return strings.Contains(constraint, "benutzer_name")
}

// leerAlsNull converts an empty username to NULL. The unique index allows
// multiple NULLs but only a single empty string — without this conversion one
// account without a username could exist and any further ones would fail.
func leerAlsNull(name string) any {
	if name == "" {
		return nil
	}
	return name
}

type benutzernameReq struct {
	Benutzername string `json:"benutzername"`
}

// BenutzernameSetzen changes an account's login name.
//
// It is allowed for the account itself and for an administrator. Changing
// one's own name does not require asking someone else, and an administrator
// still needs the capability: accounts imported from a directory or an old
// export may lack a username and the person setting it is not necessarily
// the owner.
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
