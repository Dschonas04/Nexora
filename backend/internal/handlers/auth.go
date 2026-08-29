// Registration, login, logout and the /auth/me lookup.
package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"nexora/internal/auth"
	"nexora/internal/middleware"
	"nexora/internal/models"
)

type registerReq struct {
	Email        string `json:"email"`
	Name         string `json:"name"`
	Benutzername string `json:"benutzername"`
	Password     string `json:"password"`
}

// Register creates an account and logs it straight in. Registration is open to
// anyone who can reach the API, so an instance exposed to the internet should
// sit behind a reverse proxy that restricts this route.
func (s *Server) Register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	// Lowercase the address so Alice@example.com and alice@example.com cannot
	// become two accounts; the UNIQUE index works on the stored value.
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Name = strings.TrimSpace(req.Name)
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		writeErr(w, http.StatusBadRequest, "valid email required")
		return
	}

	// Self registration can be switched off. The very first account gets through
	// nonetheless: it becomes the administrator, and without that exception a
	// fresh instance with closed registration would be unusable.
	if !RegistrierungOffen() {
		var vorhanden int
		_ = s.Pool.QueryRow(r.Context(), `SELECT count(*) FROM users`).Scan(&vorhanden)
		if vorhanden > 0 {
			writeErr(w, http.StatusForbidden, "Selbstregistrierung ist abgeschaltet")
			return
		}
	}

	// Domain filter. Applies only here, not to accounts an administrator creates,
	// who knows what they are doing.
	if erlaubt := ErlaubteDomaenen(); len(erlaubt) > 0 {
		domaene := req.Email[strings.LastIndex(req.Email, "@")+1:]
		passt := false
		for _, d := range erlaubt {
			if domaene == d {
				passt = true
				break
			}
		}
		if !passt {
			writeErr(w, http.StatusForbidden, "diese E-Mail-Domäne ist nicht zugelassen")
			return
		}
	}
	// Fall back to the local part of the address so no account is nameless.
	if req.Name == "" {
		req.Name = strings.Split(req.Email, "@")[0]
	}
	if len(req.Password) < 6 {
		writeErr(w, http.StatusBadRequest, "password must be at least 6 characters")
		return
	}

	// Der Benutzername ist freiwillig. Wer keinen angibt, bekommt einen aus dem
	// vorderen Teil der Adresse: sonst haetten die meisten Konten keinen, und
	// die Anmeldung mit Namen waere eine Einstellung, die niemand findet.
	benutzername, err := benutzernamePruefen(req.Benutzername)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if benutzername == "" {
		benutzername = s.freierBenutzername(r.Context(), benutzernameAusAdresse(req.Email))
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash failed")
		return
	}

	var u models.User
	err = s.Pool.QueryRow(r.Context(),
		`INSERT INTO users (email, name, benutzername, password_hash) VALUES ($1, $2, $3, $4)
		 RETURNING id, email, name, coalesce(benutzername, ''), role, created_at`,
		req.Email, req.Name, leerAlsNull(benutzername), hash,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Benutzername, &u.Role, &u.CreatedAt)
	if err != nil {
		// 23505 is unique_violation: either the email or the username is taken.
		// Catching it avoids a select-then-insert race between two signups.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if nameSchonVergeben(pgErr.ConstraintName) {
				writeErr(w, http.StatusConflict, "dieser Benutzername ist schon vergeben")
				return
			}
			writeErr(w, http.StatusConflict, "email already registered")
			return
		}
		writeErr(w, http.StatusInternalServerError, "could not create user")
		return
	}

	// The very first account becomes the workspace admin. Counting after the
	// insert means the very first registration wins even if two arrive at once.
	var count int
	if s.Pool.QueryRow(r.Context(), `SELECT count(*) FROM users`).Scan(&count) == nil && count == 1 {
		if _, err := s.Pool.Exec(r.Context(), `UPDATE users SET role='admin' WHERE id=$1`, u.ID); err == nil {
			u.Role = "admin"
		}
	}

	s.issueSession(w, r, u.ID)
	s.spur(r.Context(), models.Spureintrag{
		AkteurID: u.ID, AkteurName: u.Name, AkteurEmail: u.Email,
		Aktion: AktKontoAngelegt, ObjektArt: "konto", ObjektID: u.ID,
		ObjektTitel: u.Name, IP: absenderIP(r),
	})
	writeJSON(w, http.StatusCreated, u)
}

type loginReq struct {
	// Kennung ist Adresse oder Benutzername. Das Feld email bleibt daneben
	// stehen, weil aeltere Fassungen der Oberflaeche und jedes Skript, das
	// gegen diese Schnittstelle geschrieben wurde, es so schicken.
	Kennung  string `json:"kennung"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login checks the credentials and issues a session cookie.
//
// Angemeldet wird mit der Adresse ODER dem Benutzernamen. Beides in einem Feld
// und nicht in zweien: an einer Anmeldemaske erst zu waehlen, womit man sich
// gleich anmeldet, ist eine Frage, die niemand stellen muss -- ein @ im Text
// beantwortet sie.
func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	kennung := strings.ToLower(strings.TrimSpace(req.Kennung))
	if kennung == "" {
		kennung = strings.ToLower(strings.TrimSpace(req.Email))
	}

	var u models.User
	var hash string
	err := s.Pool.QueryRow(r.Context(),
		`SELECT id, email, name, coalesce(benutzername, ''), password_hash, role, created_at
		   FROM users
		  WHERE email = $1 OR lower(benutzername) = $1`,
		kennung,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Benutzername, &hash, &u.Role, &u.CreatedAt)
	// One message for an unknown address and for a wrong password, so the
	// response cannot be used to find out which addresses are registered.
	if err != nil || !auth.CheckPassword(hash, req.Password) {
		// The failed attempt is recorded; without it the audit trail would lack
		// exactly those events one opens it for in the first place. The address
		// that was entered stands there in the clear, the password never.
		s.spur(r.Context(), models.Spureintrag{
			Aktion:      AktAnmeldungFehl,
			AkteurEmail: kennung,
			ObjektArt:   "konto",
			IP:          absenderIP(r),
		})
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	s.issueSession(w, r, u.ID)
	s.spur(r.Context(), models.Spureintrag{
		AkteurID: u.ID, AkteurName: u.Name, AkteurEmail: u.Email,
		Aktion: AktAnmeldung, ObjektArt: "konto", ObjektID: u.ID,
		ObjektTitel: u.Name, IP: absenderIP(r),
	})
	writeJSON(w, http.StatusOK, u)
}

// Logout ends the session and clears the cookie.
//
// Revoking it is the actual part: previously the token stayed valid because it
// was signed, and only the browser that threw it away was logged out.
func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	// The id comes from the cookie, not from the context: logging out sits
	// deliberately before the authentication check so that it still works with an
	// expired token. Without this line the session would remain and the token stay
	// usable, precisely what logging out is meant to prevent.
	sid := middleware.SitzungID(r)
	if sid == "" {
		if c, err := r.Cookie("nexora_token"); err == nil {
			_, ausKeks, err := auth.ParseToken(s.Secret, c.Value)
			if err == nil {
				sid = ausKeks
			}
		}
	}
	if sid != "" {
		s.Pool.Exec(r.Context(),
			`UPDATE sitzungen SET widerrufen_am=now() WHERE id=$1 AND widerrufen_am IS NULL`, sid)
		s.sitzungMerken(sid, false)
	}
	s.spurAusRequest(r, AktAbmeldung, "konto", middleware.UserID(r), "", nil)
	s.clearAuthCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Me returns the signed-in account. The frontend calls it on load to decide
// between the app and the login screen, and it re-reads the row so a role
// changed by an admin shows up without a new login.
func (s *Server) Me(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var u models.User
	err := s.Pool.QueryRow(r.Context(),
		`SELECT id, email, name, coalesce(benutzername, ''), role, created_at FROM users WHERE id = $1`, uid,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Benutzername, &u.Role, &u.CreatedAt)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// issueSession creates the session, signs a token on it and sets the cookie. If
// creating it fails a token is issued anyway, only without a session id: better
// logged in without a list than not logged in at all because a side matter is
// stuck.
func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, userID string) {
	sid, err := s.sitzungAnlegen(r.Context(), r, userID)
	if err != nil {
		log.Printf("Sitzung anlegen: %v", err)
		sid = ""
	}
	token, err := auth.GenerateToken(s.Secret, userID, sid, SitzungDauer())
	if err == nil {
		s.setAuthCookieFuer(w, r, token)
	}
}
