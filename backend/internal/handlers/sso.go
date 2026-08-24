// Anmeldung über einen fremden Ausweis: OIDC und LDAP.
//
// Beide beantworten dieselbe Frage -- "wer ist das" -- auf verschiedenen Wegen,
// und beide enden an derselben Stelle: einem Konto in dieser Instanz und einer
// Sitzung darauf. Was danach kommt, weiß nichts mehr von SSO.
//
// Verknüpft wird über die E-Mail-Adresse. Das ist eine Entscheidung mit einer
// Kehrseite, die man kennen muss: wer im Verzeichnis eine Adresse ändern kann,
// kann damit auf ein bestehendes Konto zeigen. Deshalb gilt die Verknüpfung nur
// für Adressen, die der Anbieter als bestätigt meldet, und nur, wenn das Konto
// nicht selbst ein Passwort trägt -- sonst wäre SSO ein Weg, die Passwortprüfung
// zu umgehen.
package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"nexora/internal/config"
	"nexora/internal/lizenz"
	"nexora/internal/models"
)

// SSOEinstellungen sind die Werte aus der Konfiguration, die hier gebraucht
// werden. Der Server bekommt sie beim Start gereicht, statt config selbst zu
// lesen -- so bleibt der Handler prüfbar.
type SSOEinstellungen struct {
	Konf config.Konfig
	// OeffentlicheURL ist die Adresse, unter der die Instanz von außen zu
	// erreichen ist. Ohne sie lässt sich keine Rücksprungadresse bilden.
	OeffentlicheURL string
}

// oidcSitzung merkt sich, was zwischen Hin- und Rückweg gebraucht wird.
type oidcSitzung struct {
	Zustand string
	Nonce   string
	Ziel    string
	Bis     time.Time
}

const oidcKeksName = "nexora_oidc"

// SSOZustand sagt der Anmeldeseite, was angeboten werden darf.
func (s *Server) SSOZustand(w http.ResponseWriter, r *http.Request) {
	k := s.SSO.Konf
	writeJSON(w, http.StatusOK, map[string]any{
		// Beides nur, wenn eingerichtet UND lizenziert. Einen Knopf zu zeigen,
		// der danach mit 402 antwortet, wäre ein Versprechen ohne Deckung.
		"oidc":     k.OIDCAktiv && k.OIDCAussteller != "" && lizenz.Frei(lizenz.SSO),
		"oidcText": k.OIDCKnopfText,
		"ldap":     k.LDAPAktiv && k.LDAPServer != "" && lizenz.Frei(lizenz.LDAP),
		"passwort": true,
		"anbieter": kurzerAussteller(k.OIDCAussteller),
	})
}

// kurzerAussteller macht aus einer Aussteller-URL etwas, das auf einen Knopf
// passt: aus "https://10.0.2.43:8180/realms/homelab" wird "homelab".
func kurzerAussteller(url string) string {
	url = strings.TrimSuffix(strings.TrimSpace(url), "/")
	if url == "" {
		return ""
	}
	teile := strings.Split(url, "/")
	return teile[len(teile)-1]
}

// oidcAnbieter baut die Verbindung zum Aussteller auf.
//
// Bei jedem Aufruf neu: die Erkennung ist ein einzelner Aufruf, sie passiert
// zweimal je Anmeldung, und dafür einen Zwischenspeicher zu pflegen, der bei
// einer Änderung am Aussteller veraltet, lohnt nicht.
func (s *Server) oidcAnbieter(ctx context.Context) (*oidc.Provider, *oauth2.Config, error) {
	k := s.SSO.Konf
	if !k.OIDCAktiv || k.OIDCAussteller == "" || k.OIDCClientID == "" {
		return nil, nil, errors.New("OIDC ist nicht eingerichtet")
	}
	anbieter, err := oidc.NewProvider(ctx, strings.TrimSuffix(k.OIDCAussteller, "/"))
	if err != nil {
		return nil, nil, fmt.Errorf("Aussteller nicht erreichbar: %w", err)
	}
	bereiche := strings.Fields(k.OIDCBereiche)
	if len(bereiche) == 0 {
		bereiche = []string{oidc.ScopeOpenID, "email", "profile"}
	}
	return anbieter, &oauth2.Config{
		ClientID:     k.OIDCClientID,
		ClientSecret: k.OIDCGeheimnis,
		Endpoint:     anbieter.Endpoint(),
		RedirectURL:  s.rueckAdresse(),
		Scopes:       bereiche,
	}, nil
}

// rueckAdresse ist die Adresse, an die der Aussteller zurückschickt. Sie muss
// beim Aussteller hinterlegt sein, deshalb wird sie aus der öffentlichen URL
// gebildet und nicht aus dem Kopf der Anfrage: ein Angreifer, der den Host-Kopf
// setzt, könnte sonst den Rücksprung umlenken.
func (s *Server) rueckAdresse() string {
	basis := strings.TrimSuffix(s.SSO.OeffentlicheURL, "/")
	return basis + "/api/auth/oidc/zurueck"
}

// OIDCStart schickt den Browser zum Aussteller.
func (s *Server) OIDCStart(w http.ResponseWriter, r *http.Request) {
	if !lizenz.Frei(lizenz.SSO) {
		writeErr(w, http.StatusPaymentRequired, "SSO gehört zum Zusatzumfang")
		return
	}
	if s.SSO.OeffentlicheURL == "" {
		writeErr(w, http.StatusPreconditionFailed,
			"oeffentliche_url ist nicht gesetzt -- ohne sie gibt es keine Rücksprungadresse")
		return
	}
	_, oauthKonf, err := s.oidcAnbieter(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	zustand, nonce := zufall(), zufall()
	// Zustand und Nonce reisen in einem eigenen Plätzchen mit, nicht im
	// Speicher des Dienstes: sonst überlebte eine begonnene Anmeldung keinen
	// Neustart, und zwei Instanzen hinter einem Verteiler könnten sich nicht
	// abwechseln.
	s.oidcKeksSetzen(w, r, oidcSitzung{
		Zustand: zustand,
		Nonce:   nonce,
		Ziel:    r.URL.Query().Get("ziel"),
		Bis:     time.Now().Add(10 * time.Minute),
	})
	http.Redirect(w, r, oauthKonf.AuthCodeURL(zustand, oidc.Nonce(nonce)), http.StatusFound)
}

// OIDCZurueck nimmt den Code entgegen und meldet an.
func (s *Server) OIDCZurueck(w http.ResponseWriter, r *http.Request) {
	if !lizenz.Frei(lizenz.SSO) {
		writeErr(w, http.StatusPaymentRequired, "SSO gehört zum Zusatzumfang")
		return
	}
	mit, err := s.oidcKeksLesen(r)
	if err != nil {
		s.ssoFehler(w, r, "Die Anmeldung ist abgelaufen. Bitte noch einmal versuchen.")
		return
	}
	s.oidcKeksLoeschen(w, r)

	// Der Zustand ist der Schutz gegen untergeschobene Anmeldungen: ohne
	// Vergleich könnte jemand einen fremden Code in den Browser des Opfers
	// schieben und es damit an sein Konto anmelden.
	if r.URL.Query().Get("state") != mit.Zustand || mit.Zustand == "" {
		s.ssoFehler(w, r, "Die Antwort gehört nicht zu dieser Anmeldung.")
		return
	}
	if fehler := r.URL.Query().Get("error"); fehler != "" {
		s.ssoFehler(w, r, "Der Anbieter hat abgelehnt: "+fehler)
		return
	}

	anbieter, oauthKonf, err := s.oidcAnbieter(r.Context())
	if err != nil {
		s.ssoFehler(w, r, err.Error())
		return
	}
	token, err := oauthKonf.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		s.ssoFehler(w, r, "Der Code wurde nicht angenommen.")
		return
	}
	rohID, ok := token.Extra("id_token").(string)
	if !ok {
		s.ssoFehler(w, r, "Der Anbieter hat kein id_token geschickt.")
		return
	}
	idToken, err := anbieter.Verifier(&oidc.Config{ClientID: s.SSO.Konf.OIDCClientID}).
		Verify(r.Context(), rohID)
	if err != nil {
		s.ssoFehler(w, r, "Das Token des Anbieters ist nicht gültig.")
		return
	}
	if idToken.Nonce != mit.Nonce {
		s.ssoFehler(w, r, "Die Kennung der Anmeldung passt nicht.")
		return
	}

	var behauptung map[string]any
	if err := idToken.Claims(&behauptung); err != nil {
		s.ssoFehler(w, r, "Die Angaben des Anbieters sind unlesbar.")
		return
	}

	k := s.SSO.Konf
	email := strings.ToLower(strings.TrimSpace(behauptungText(behauptung, k.OIDCFeldEmail, "email")))
	name := strings.TrimSpace(behauptungText(behauptung, k.OIDCFeldName, "name"))
	if email == "" {
		s.ssoFehler(w, r, "Der Anbieter hat keine E-Mail-Adresse mitgeschickt.")
		return
	}
	// Eine unbestätigte Adresse verknüpft nicht: sonst genügte es, sich beim
	// Anbieter mit fremder Adresse einzutragen, um hier fremde Seiten zu lesen.
	if bestaetigt, da := behauptung["email_verified"].(bool); da && !bestaetigt {
		s.ssoFehler(w, r, "Der Anbieter hat diese Adresse nicht bestätigt.")
		return
	}

	admin := gruppeEnthaelt(behauptung, k.OIDCGruppeAdmin)
	u, err := s.kontoAusSSO(r.Context(), email, name, admin, "oidc")
	if err != nil {
		s.ssoFehler(w, r, err.Error())
		return
	}

	s.issueSession(w, r, u.ID)
	s.spur(r.Context(), models.Spureintrag{
		AkteurID: u.ID, AkteurName: u.Name, AkteurEmail: u.Email,
		Aktion: AktAnmeldung, ObjektArt: "konto", ObjektID: u.ID,
		ObjektTitel: u.Name, IP: absenderIP(r),
	})

	ziel := mit.Ziel
	// Nur Ziele innerhalb dieser Anwendung. Alles andere wäre eine offene
	// Weiterleitung -- ein beliebter Baustein für Täuschungsseiten.
	//
	// Geprüft wird auf beide Schrägstriche: "//fremd.example" ist eine Adresse
	// mit dem Protokoll der aktuellen Seite, und "/\fremd.example" behandeln
	// manche Browser genauso.
	if !strings.HasPrefix(ziel, "/") ||
		strings.HasPrefix(ziel, "//") || strings.HasPrefix(ziel, `/\`) {
		ziel = "/"
	}
	http.Redirect(w, r, ziel, http.StatusFound)
}

// kontoAusSSO findet oder legt das Konto an.
func (s *Server) kontoAusSSO(ctx context.Context, email, name string, admin bool, herkunft string) (models.User, error) {
	var u models.User
	var hash string
	err := s.Pool.QueryRow(ctx,
		`SELECT id, email, name, password_hash, role, created_at FROM users WHERE email=$1`,
		email).Scan(&u.ID, &u.Email, &u.Name, &hash, &u.Role, &u.CreatedAt)

	if err == nil {
		// Übernommen wird nur ein Konto, das AUSDRÜCKLICH über SSO entstanden
		// ist. Jedes andere -- mit Passwort oder mit leerem Passwortfeld --
		// bleibt unangetastet.
		//
		// Die frühere Fassung ließ ein leeres Passwortfeld durchgehen. Das war
		// eine Lücke: wer im Verzeichnis eine Adresse setzen kann, hätte sich
		// damit an ein fremdes Konto gehängt, sobald dessen Feld aus
		// irgendeinem Grund leer war.
		if !strings.HasPrefix(hash, "sso:") {
			return u, errors.New("Für diese Adresse gibt es bereits ein Konto. " +
				"Ein Administrator muss es freigeben, bevor SSO darauf zugreift.")
		}
		// Höherstufen nur bei einem SSO-Konto, und mit Eintrag in der
		// Prüfspur: eine Rolle, die sich still ändert, fällt niemandem auf.
		if admin && u.Role != "admin" {
			if _, err := s.Pool.Exec(ctx, `UPDATE users SET role='admin' WHERE id=$1`, u.ID); err == nil {
				u.Role = "admin"
				s.spur(ctx, models.Spureintrag{
					AkteurName: "SSO", Aktion: AktRolleGeaendert, ObjektArt: "konto",
					ObjektID: u.ID, ObjektTitel: u.Email,
					Details: json.RawMessage(`{"rolle":"admin","durch":"` + herkunft + `"}`),
				})
			}
		}
		return u, nil
	}

	// Neues Konto. Der Passwort-Platzhalter beginnt mit "sso:" und ist kein
	// gültiger bcrypt-Wert -- eine Anmeldung mit Passwort scheitert daran
	// zuverlässig, ohne dass es dafür eine eigene Spalte braucht.
	if name == "" {
		name = strings.SplitN(email, "@", 2)[0]
	}
	rolle := "user"
	if admin {
		rolle = "admin"
	}
	// Das erste Konto einer leeren Instanz wird Administrator -- dieselbe Regel
	// wie bei der Registrierung mit Passwort.
	var vorhanden int
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&vorhanden)
	if vorhanden == 0 {
		rolle = "admin"
	}

	err = s.Pool.QueryRow(ctx,
		`INSERT INTO users (email, name, password_hash, role)
		 VALUES ($1, $2, $3, $4) RETURNING id, email, name, role, created_at`,
		email, name, "sso:"+herkunft, rolle).
		Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt)
	if err != nil {
		return u, errors.New("Konto konnte nicht angelegt werden.")
	}
	log.Printf("SSO (%s): Konto für %s angelegt, Rolle %s", herkunft, email, rolle)
	return u, nil
}

// ssoFehler schickt den Browser mit einer Meldung zurück zur Anmeldeseite.
//
// Kein JSON: an dieser Stelle steht der Browser mitten in einer Weiterleitung,
// und eine JSON-Antwort wäre eine weiße Seite mit geschweiften Klammern.
func (s *Server) ssoFehler(w http.ResponseWriter, r *http.Request, meldung string) {
	log.Printf("SSO abgebrochen: %s", meldung)
	// QueryEscape statt einer eigenen Ersetzungsliste: eine handgeschriebene
	// Liste vergisst immer ein Zeichen, und dieses eine ist dann das, mit dem
	// sich die Adresse aufbrechen lässt.
	http.Redirect(w, r, "/login?sso="+url.QueryEscape(meldung), http.StatusFound)
}

func zufall() string {
	b := make([]byte, 24)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// behauptungText liest ein Feld aus den Angaben, mit Rückfall auf den üblichen Namen.
func behauptungText(m map[string]any, feld, vorgabe string) string {
	if feld == "" {
		feld = vorgabe
	}
	if v, ok := m[feld].(string); ok {
		return v
	}
	if v, ok := m[vorgabe].(string); ok {
		return v
	}
	return ""
}

// gruppeEnthaelt sucht einen Gruppennamen in den üblichen Feldern.
func gruppeEnthaelt(m map[string]any, gesucht string) bool {
	if strings.TrimSpace(gesucht) == "" {
		return false
	}
	for _, feld := range []string{"groups", "roles", "realm_access"} {
		switch v := m[feld].(type) {
		case []any:
			for _, e := range v {
				if s, ok := e.(string); ok && s == gesucht {
					return true
				}
			}
		case map[string]any:
			// Keycloak legt die Rollen unter realm_access.roles ab.
			if rollen, ok := v["roles"].([]any); ok {
				for _, e := range rollen {
					if s, ok := e.(string); ok && s == gesucht {
						return true
					}
				}
			}
		}
	}
	return false
}
