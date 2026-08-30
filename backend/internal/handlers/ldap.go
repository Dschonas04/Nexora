// Signing in against a directory (LDAP or Active Directory).
//
// The sequence is the same as with any directory service: bind with a service
// account, search for the user, then try to bind as that user with the password
// they typed. If that succeeds the password is correct, and it was the
// directory that checked it, not Nexora.
//
// The password is never stored here, not even as a hash. Someone locked out in
// the directory cannot get back in on their next attempt; a session already
// running ends with its deadline or when somebody revokes it.
package handlers

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"nexora/internal/lizenz"
)

type ldapAnmeldeReq struct {
	Benutzer string `json:"benutzer"`
	Passwort string `json:"passwort"`
}

// LDAPAnmeldung signs in through the directory.
func (s *Server) LDAPAnmeldung(w http.ResponseWriter, r *http.Request) {
	if !lizenz.Frei(lizenz.LDAP) {
		writeErr(w, http.StatusPaymentRequired, "LDAP gehört zum Zusatzumfang")
		return
	}
	k := s.SSO.Konf
	if !k.LDAPAktiv || k.LDAPServer == "" {
		writeErr(w, http.StatusPreconditionFailed, "LDAP ist nicht eingerichtet")
		return
	}

	var req ldapAnmeldeReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Benutzer = strings.TrimSpace(req.Benutzer)
	if req.Benutzer == "" || req.Passwort == "" {
		writeErr(w, http.StatusBadRequest, "Benutzer und Passwort werden gebraucht")
		return
	}

	name, email, admin, err := s.ldapPruefen(req.Benutzer, req.Passwort)
	if err != nil {
		// The same message for "does not exist" and "wrong password": otherwise
		// the directory could be enumerated through this endpoint.
		log.Printf("LDAP-Anmeldung für %q gescheitert: %v", req.Benutzer, err)
		s.anmeldeSpur(r, WegLDAP, req.Benutzer, GrundVerzeichn, nil)
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	u, err := s.kontoAusSSO(r.Context(), email, name, admin, "ldap")
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	s.issueSession(w, r, u.ID)
	s.anmeldeSpur(r, WegLDAP, req.Benutzer, "", &u)
	writeJSON(w, http.StatusOK, u)
}

// ldapPruefen asks the directory. Returns name, email and whether admin.
func (s *Server) ldapPruefen(benutzer, passwort string) (string, string, bool, error) {
	k := s.SSO.Konf

	verbindung, err := ldap.DialURL(k.LDAPServer, ldap.DialWithTLSConfig(&tls.Config{
		// Only switchable because an in-house directory often carries a
		// self-issued certificate. The default verifies.
		InsecureSkipVerify: !k.LDAPTLSPruefen,
	}))
	if err != nil {
		return "", "", false, fmt.Errorf("Verbindung: %w", err)
	}
	defer verbindung.Close()
	verbindung.SetTimeout(10 * time.Second)

	// StartTLS on an already encrypted connection (ldaps://) would be an error,
	// hence only for ldap://.
	if k.LDAPStartTLS && strings.HasPrefix(strings.ToLower(k.LDAPServer), "ldap://") {
		if err := verbindung.StartTLS(&tls.Config{InsecureSkipVerify: !k.LDAPTLSPruefen}); err != nil {
			return "", "", false, fmt.Errorf("StartTLS: %w", err)
		}
	}

	// The service account first: many directories do not allow searching without
	// one. If none is configured the search is anonymous, which some allow.
	if k.LDAPBindDN != "" {
		if err := verbindung.Bind(k.LDAPBindDN, k.LDAPBindPasswort); err != nil {
			return "", "", false, fmt.Errorf("Dienstkonto: %w", err)
		}
	}

	filter := k.LDAPBenutzerFilter
	if filter == "" {
		filter = "(&(objectClass=person)(|(uid=%s)(sAMAccountName=%s)(mail=%s)))"
	}
	// The typed name goes into the filter escaped. Without that, ")(|(uid=*"
	// would rewrite the filter and match every account.
	sicher := ldap.EscapeFilter(benutzer)
	filter = strings.ReplaceAll(filter, "%s", sicher)

	felder := []string{"dn", k.LDAPFeldName, k.LDAPFeldEmail, "memberOf"}
	suche := ldap.NewSearchRequest(k.LDAPBasisDN, ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases, 2, 10, false, filter, felder, nil)

	antwort, err := verbindung.Search(suche)
	if err != nil {
		return "", "", false, fmt.Errorf("Suche: %w", err)
	}
	if len(antwort.Entries) != 1 {
		// Abort on more than one hit as well: which one was meant cannot be
		// decided, and guessing would mean signing somebody in as somebody
		// else.
		return "", "", false, errors.New("kein eindeutiger Eintrag")
	}
	eintrag := antwort.Entries[0]

	// The actual check: bind as the user that was found.
	if err := verbindung.Bind(eintrag.DN, passwort); err != nil {
		return "", "", false, errors.New("Passwort abgelehnt")
	}

	name := eintrag.GetAttributeValue(k.LDAPFeldName)
	email := strings.ToLower(strings.TrimSpace(eintrag.GetAttributeValue(k.LDAPFeldEmail)))
	if email == "" {
		return "", "", false, errors.New("kein E-Mail-Feld im Eintrag. Ohne Adresse lässt sich kein Konto anlegen.")
	}

	admin := false
	if k.LDAPGruppeAdmin != "" {
		for _, g := range eintrag.GetAttributeValues("memberOf") {
			if strings.EqualFold(g, k.LDAPGruppeAdmin) {
				admin = true
			}
		}
	}
	return name, email, admin, nil
}
