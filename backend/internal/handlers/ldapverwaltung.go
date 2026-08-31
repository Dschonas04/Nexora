// Die LDAP-Verwaltung: nachsehen, wie das Verzeichnis eingerichtet ist, und
// ausprobieren, ob es antwortet.
//
// Die Werte selbst stehen in config.conf und lassen sich hier nicht ändern. Das
// ist keine Lücke, sondern dieselbe Regel wie überall: was gebraucht wird, bevor
// die Datenbank offen ist, gehört in die Datei. Ein Dienstkonto-Passwort in einer
// Datenbankzeile würde außerdem jeder Dump mitnehmen.
//
// Was hier dazukommt, ist das, was die Datei nicht beantworten kann: ob der
// Server erreichbar ist, ob das Dienstkonto angenommen wird, ob der Filter
// genau einen Eintrag trifft und ob in diesem Eintrag die Felder stehen, die
// konfiguriert sind. An diesen vier Stellen scheitert eine LDAP-Einrichtung,
// und keine davon sieht man einer Konfigurationsdatei an.
package handlers

import (
	"net/http"
	"strings"

	"nexora/internal/lizenz"
	"nexora/internal/middleware"
)

// LDAPEinrichtung gibt zurück, wie das Verzeichnis eingerichtet ist.
//
// Das Passwort des Dienstkontos steht NICHT dabei, nur ob eines hinterlegt ist.
// Es wäre der eine Wert hier, mit dem sich jemand am Verzeichnis selbst
// bedienen könnte, und für die Frage "ist es gesetzt" reicht ein Ja.
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
// Ohne Passwort wird nur gesucht. Das ist der übliche Fall: ein Administrator
// prüft die Einrichtung an einem fremden Konto und hat dessen Passwort nicht,
// soll es auch nicht haben. Mit Passwort wird zusätzlich gebunden, dann ist die
// ganze Kette geprüft.
//
// Angelegt wird dabei nichts. Ein Konto entsteht in Nexora erst bei einer
// wirklichen Anmeldung; ein Test, der nebenbei Konten anlegt, wäre keiner.
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

	// Ohne Adresse käme die Anmeldung später nicht weiter. Der Test sagt das
	// hier, statt es dem ersten Benutzer zu überlassen, es herauszufinden.
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
