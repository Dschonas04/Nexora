// LDAP administration: inspect how the directory is configured and test whether
// it responds.
//
// The raw values live in config.conf and cannot be changed here. That is not
// a bug but the same rule as everywhere: values needed before the database
// is open belong in the file. A service account password in a database row
// would also be captured by any dump.
//
// What this endpoint adds is the information the config file cannot answer:
// whether the server is reachable, whether the service account is accepted,
// whether the filter matches exactly one entry, and whether the configured
// fields are present in that entry. LDAP setup fails at any of these four
// points, and none of them is visible in a static configuration file.
package handlers

import (
	"net/http"
	"strings"

	"nexora/internal/lizenz"
	"nexora/internal/middleware"
)

// LDAPEinrichtung returns how the directory is configured.
//
// The service account password is NOT included, only whether one is present.
// That value would give someone access to the directory itself, and a simple
// yes/no is sufficient for the question "is it set?".
func (s *Server) LDAPEinrichtung(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	k := s.SSO.Konf

	// Womit ein Eintrag gesucht wird. Steht nichts in der Datei, greift dieselbe
	// Vorgabe wie beim Anmelden; sie hier zu verschweigen hieße, dem Leser einen
	// leeren Filter zu zeigen, der in Wahrheit nicht leer ist.
	filter := k.LDAPBenutzerFilter
	if filter == "" {
		filter = "(&(objectClass=person)(|(uid=%s)(sAMAccountName=%s)(mail=%s)))"
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"aktiv":          k.LDAPAktiv,
		"lizenziert":     lizenz.Frei(lizenz.LDAP),
		"server":         k.LDAPServer,
		"startTLS":       k.LDAPStartTLS,
		"tlsPruefen":     k.LDAPTLSPruefen,
		"bindDN":         k.LDAPBindDN,
		"bindPasswortDa": k.LDAPBindPasswort != "",
		"basisDN":        k.LDAPBasisDN,
		"benutzerFilter": filter,
		"feldName":       k.LDAPFeldName,
		"feldEmail":      k.LDAPFeldEmail,
		"gruppeAdmin":    k.LDAPGruppeAdmin,
		// Verschlüsselt die Verbindung wirklich? ldaps:// bringt es mit,
		// ldap:// nur mit StartTLS. Beides zusammengerechnet, weil die Frage
		// eine ist und nicht zwei.
		"verschluesselt": k.LDAPStartTLS || strings.HasPrefix(strings.ToLower(k.LDAPServer), "ldaps://"),
	})
}

type ldapTestReq struct {
	Benutzer string `json:"benutzer"`
	Passwort string `json:"passwort"`
}

// LDAPTesten fragt das Verzeichnis nach einem Konto.
//
// Without a password only a lookup is performed. This is the typical case:
// an administrator tests the setup against an external account and does not
// have (and should not have) its password. Providing a password additionally
// attempts a bind, thus verifying the whole chain.
//
// Nothing is created in Nexora as part of the test. An account is only
// created on an actual login; a test that created accounts as a side effect
// would not be a harmless test.
func (s *Server) LDAPTesten(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	if !lizenz.Frei(lizenz.LDAP) {
		writeErr(w, http.StatusPaymentRequired, "LDAP gehört zum Zusatzumfang")
		return
	}
	k := s.SSO.Konf
	if !k.LDAPAktiv || k.LDAPServer == "" {
		writeErr(w, http.StatusPreconditionFailed, "LDAP ist nicht eingerichtet")
		return
	}

	var req ldapTestReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Benutzer = strings.TrimSpace(req.Benutzer)
	if req.Benutzer == "" {
		writeErr(w, http.StatusBadRequest, "Es wird ein Benutzername gebraucht")
		return
	}

	befund, err := s.ldapAbfragen(req.Benutzer, req.Passwort, req.Passwort != "")

	// Der Test wird verzeichnet, nicht aber als Anmeldung: es ist keine. Er
	// steht in der Prüfspur, weil er das Dienstkonto benutzt und weil damit
	// jemand nachsehen kann, ob ein Konto im Verzeichnis existiert.
	s.spurAusRequest(r, AktLDAPTest, "verzeichnis", "", req.Benutzer, map[string]interface{}{
		"geglueckt":   err == nil,
		"mitPasswort": req.Passwort != "",
	})

	if err != nil {
		// 200 und nicht 4xx: die Anfrage war in Ordnung, das Ergebnis ist nur
		// negativ. Ein Fehlerstatus würde die Oberfläche zwingen, zwischen
		// "Test gescheitert" und "Test nicht durchführbar" zu raten.
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":     false,
			"fehler": err.Error(),
		})
		return
	}

	// Without an email address a login later would not succeed. The test
	// reports that here rather than leaving it to the first user to discover.
	hinweis := ""
	if befund.Email == "" {
		hinweis = "Der Eintrag wurde gefunden, trägt aber nichts im Feld " + k.LDAPFeldEmail +
			". Ohne Adresse kann daraus kein Konto entstehen."
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      befund.Email != "",
		"hinweis": hinweis,
		"befund":  befund,
	})
}
