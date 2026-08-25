// Anmeldung gegen ein Verzeichnis (LDAP oder Active Directory).
//
// Der Ablauf ist derselbe wie bei jedem Verzeichnisdienst: mit einem
// Dienstkonto anmelden, den Benutzer suchen, dann versuchen, sich als dieser
// Benutzer mit dem eingegebenen Passwort anzumelden. Gelingt das, stimmt das
// Passwort, geprüft hat es das Verzeichnis, nicht Nexora.
//
// Das Passwort wird hier nie gespeichert, auch nicht als Prüfsumme. Wer im
// Verzeichnis gesperrt wird, kommt beim nächsten Versuch nicht mehr herein; die
// laufende Sitzung endet mit ihrer Frist oder wenn jemand sie beendet.
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
	"nexora/internal/models"
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
		// Dieselbe Meldung für "gibt es nicht" und "falsches Passwort": sonst
		// ließe sich das Verzeichnis über diese Schnittstelle durchprobieren.
		log.Printf("LDAP-Anmeldung für %q gescheitert: %v", req.Benutzer, err)
		s.spur(r.Context(), models.Spureintrag{
			Aktion: AktAnmeldungFehl, AkteurEmail: req.Benutzer,
			ObjektArt: "konto", IP: absenderIP(r),
		})
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	u, err := s.kontoAusSSO(r.Context(), email, name, admin, "ldap")
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
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

// ldapPruefen asks the directory. Returns name, email and whether admin.
func (s *Server) ldapPruefen(benutzer, passwort string) (string, string, bool, error) {
	k := s.SSO.Konf

	verbindung, err := ldap.DialURL(k.LDAPServer, ldap.DialWithTLSConfig(&tls.Config{
		// Nur abschaltbar, weil ein Verzeichnis im eigenen Haus oft ein selbst
		// ausgestelltes Zertifikat trägt. Die Vorgabe prüft.
		InsecureSkipVerify: !k.LDAPTLSPruefen,
	}))
	if err != nil {
		return "", "", false, fmt.Errorf("Verbindung: %w", err)
	}
	defer verbindung.Close()
	verbindung.SetTimeout(10 * time.Second)

	// StartTLS auf einer bereits verschlüsselten Verbindung (ldaps://) wäre ein
	// Fehler, deshalb nur bei ldap://.
	if k.LDAPStartTLS && strings.HasPrefix(strings.ToLower(k.LDAPServer), "ldap://") {
		if err := verbindung.StartTLS(&tls.Config{InsecureSkipVerify: !k.LDAPTLSPruefen}); err != nil {
			return "", "", false, fmt.Errorf("StartTLS: %w", err)
		}
	}

	// Erst das Dienstkonto: ohne es darf man in vielen Verzeichnissen gar nicht
	// suchen. Fehlt es, wird anonym gesucht, manche erlauben das.
	if k.LDAPBindDN != "" {
		if err := verbindung.Bind(k.LDAPBindDN, k.LDAPBindPasswort); err != nil {
			return "", "", false, fmt.Errorf("Dienstkonto: %w", err)
		}
	}

	filter := k.LDAPBenutzerFilter
	if filter == "" {
		filter = "(&(objectClass=person)(|(uid=%s)(sAMAccountName=%s)(mail=%s)))"
	}
	// Der eingegebene Name geht maskiert in den Filter. Ohne das ließe sich mit
	// ")(|(uid=*" der Filter umschreiben und jedes Konto treffen.
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
		// Auch bei mehr als einem Treffer abbrechen: welcher gemeint war, ist
		// dann nicht entscheidbar, und zu raten hieße, jemanden als jemand
		// anderen anzumelden.
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
