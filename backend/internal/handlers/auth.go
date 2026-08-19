// Registration, login, logout and the /auth/me lookup.
package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"nexora/internal/auth"
	"nexora/internal/middleware"
	"nexora/internal/models"
)

type registerReq struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
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

	// Selbstregistrierung kann abgeschaltet sein. Das allererste Konto kommt
	// trotzdem durch: es wird zum Administrator, und ohne diese Ausnahme wäre
	// eine frische Instanz mit geschlossener Registrierung unbenutzbar.
	if !RegistrierungOffen() {
		var vorhanden int
		_ = s.Pool.QueryRow(r.Context(), `SELECT count(*) FROM users`).Scan(&vorhanden)
		if vorhanden > 0 {
			writeErr(w, http.StatusForbidden, "Selbstregistrierung ist abgeschaltet")
			return
		}
	}

	// Domänenfilter. Greift nur hier, nicht auf Konten, die ein Administrator
	// anlegt -- der weiß, was er tut.
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
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash failed")
		return
	}

	var u models.User
	err = s.Pool.QueryRow(r.Context(),
		`INSERT INTO users (email, name, password_hash) VALUES ($1, $2, $3)
		 RETURNING id, email, name, role, created_at`,
		req.Email, req.Name, hash,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt)
	if err != nil {
		// 23505 is unique_violation, which here can only be the email index.
		// Catching it avoids a select-then-insert race between two signups.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
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

	s.issueSession(w, u.ID)
	s.spur(r.Context(), models.Spureintrag{
		AkteurID: u.ID, AkteurName: u.Name, AkteurEmail: u.Email,
		Aktion: AktKontoAngelegt, ObjektArt: "konto", ObjektID: u.ID,
		ObjektTitel: u.Name, IP: absenderIP(r),
	})
	writeJSON(w, http.StatusCreated, u)
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login checks the credentials and issues a session cookie.
func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	var u models.User
	var hash string
	err := s.Pool.QueryRow(r.Context(),
		`SELECT id, email, name, password_hash, role, created_at FROM users WHERE email = $1`,
		req.Email,
	).Scan(&u.ID, &u.Email, &u.Name, &hash, &u.Role, &u.CreatedAt)
	// One message for an unknown address and for a wrong password, so the
	// response cannot be used to find out which addresses are registered.
	if err != nil || !auth.CheckPassword(hash, req.Password) {
		// Der Fehlversuch wird festgehalten -- ohne ihn hätte die Prüfspur
		// genau die Vorgänge nicht, für die man sie am ehesten aufschlägt.
		// Die eingegebene Adresse steht dabei im Klartext, das Passwort nie.
		s.spur(r.Context(), models.Spureintrag{
			Aktion:      AktAnmeldungFehl,
			AkteurEmail: req.Email,
			ObjektArt:   "konto",
			IP:          absenderIP(r),
		})
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	s.issueSession(w, u.ID)
	s.spur(r.Context(), models.Spureintrag{
		AkteurID: u.ID, AkteurName: u.Name, AkteurEmail: u.Email,
		Aktion: AktAnmeldung, ObjektArt: "konto", ObjektID: u.ID,
		ObjektTitel: u.Name, IP: absenderIP(r),
	})
	writeJSON(w, http.StatusOK, u)
}

// Logout clears the cookie. See clearAuthCookie: the token stays valid.
func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
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
		`SELECT id, email, name, role, created_at FROM users WHERE id = $1`, uid,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// issueSession signs a token and sets the cookie. A signing failure leaves the
// caller without a session; the surrounding handler still reports success, and
// the frontend then lands back on the login screen.
func (s *Server) issueSession(w http.ResponseWriter, userID string) {
	token, err := auth.GenerateToken(s.Secret, userID, SitzungDauer())
	if err == nil {
		s.setAuthCookie(w, token)
	}
}
