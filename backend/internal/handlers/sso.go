// SPDX-License-Identifier: LicenseRef-Nexora-Enterprise
// Copyright (c) 2026 Jonas Groll. See LICENSE-ENTERPRISE.

// Signing in with an outside identity: OIDC and LDAP.
//
// Both answer the same question, "who is this", by different routes, and both
// end in the same place: an account in this instance and a session on it.
// Everything downstream knows nothing about SSO.
//
// Accounts are matched by email address. That decision has a downside worth
// knowing: whoever can change an address in the directory can point it at an
// existing account. So the match only counts for addresses the provider reports
// as verified, and only when the account carries no password of its own, or SSO
// would be a way around the password check.
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
	"nexora/internal/auth"

	"nexora/internal/config"
	"nexora/internal/lizenz"
	"nexora/internal/middleware"
	"nexora/internal/models"
)

// SSOEinstellungen are the configuration values needed here. The server is
// handed them at startup instead of reading config itself, which keeps the
// handler testable.
type SSOEinstellungen struct {
	Konf config.Konfig
	// OeffentlicheURL is the address the instance can be reached at from
	// outside. Without it no callback address can be formed.
	OeffentlicheURL string
}

// oidcWerte are the OIDC settings in effect right now: a value set in the
// administration wins over config.conf and the environment, and applies
// without a restart. Scopes and claim names stay in the file; they are only
// changed together with the provider anyway.
type oidcWerte struct {
	Aktiv, Aussteller, ClientID, Geheimnis, KnopfText, GruppeAdmin string
	Bereiche, FeldName, FeldEmail                                  string
}

func (s *Server) oidcJetzt() oidcWerte {
	k := s.SSO.Konf
	return oidcWerte{
		Aktiv:       wert("oidc_enabled"),
		Aussteller:  strings.TrimRight(strings.TrimSpace(wert("oidc_issuer")), "/"),
		ClientID:    strings.TrimSpace(wert("oidc_client_id")),
		Geheimnis:   wert("oidc_secret"),
		KnopfText:   wert("oidc_button_text"),
		GruppeAdmin: wert("oidc_admin_group"),
		Bereiche:    k.OIDCBereiche,
		FeldName:    k.OIDCFeldName,
		FeldEmail:   k.OIDCFeldEmail,
	}
}

// oeffentlicheURL is the address the instance is reached at, as set in the
// administration or the file.
func (s *Server) oeffentlicheURL() string { return speicherOeffentlicheURL() }

// oidcSitzung remembers what is needed between the outbound and return leg.
type oidcSitzung struct {
	Zustand string
	Nonce   string
	Ziel    string
	Bis     time.Time
}

const oidcKeksName = "nexora_oidc"

// SSOZustand tells the sign-in page what may be offered.
func (s *Server) SSOZustand(w http.ResponseWriter, r *http.Request) {
	k := s.SSO.Konf
	o := s.oidcJetzt()
	knopf := o.KnopfText
	if strings.TrimSpace(knopf) == "" {
		knopf = "Mit SSO anmelden"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		// Both only when configured AND licensed. Showing a button that then
		// answers with 402 would be a promise without cover.
		"oidc": o.Aktiv == "ja" && o.Aussteller != "" && o.ClientID != "" &&
			s.oeffentlicheURL() != "" && lizenz.Frei(lizenz.SSO),
		"oidcButtonText": knopf,
		"ldap":           k.LDAPAktiv && k.LDAPServer != "" && lizenz.Frei(lizenz.LDAP),
		"password":       true,
		"provider":       kurzerAussteller(o.Aussteller),
	})
}

// kurzerAussteller turns an issuer URL into something that fits on a button:
// "https://10.0.2.43:8180/realms/homelab" becomes "homelab".
func kurzerAussteller(url string) string {
	url = strings.TrimSuffix(strings.TrimSpace(url), "/")
	if url == "" {
		return ""
	}
	teile := strings.Split(url, "/")
	return teile[len(teile)-1]
}

// oidcAnbieter builds the connection to the issuer.
//
// Rebuilt on every call: discovery is a single request, it happens twice per
// sign-in, and keeping a cache that goes stale whenever the issuer changes is
// not worth it.
func (s *Server) oidcAnbieter(ctx context.Context) (*oidc.Provider, *oauth2.Config, error) {
	o := s.oidcJetzt()
	if o.Aktiv != "ja" || o.Aussteller == "" || o.ClientID == "" {
		return nil, nil, errors.New("OIDC ist nicht eingerichtet")
	}
	return s.oidcVerbinden(ctx, o)
}

// oidcVerbinden fetches the discovery document; split off so the test in the
// administration can check values before SSO is switched on.
func (s *Server) oidcVerbinden(ctx context.Context, o oidcWerte) (*oidc.Provider, *oauth2.Config, error) {
	anbieter, err := oidc.NewProvider(ctx, o.Aussteller)
	if err != nil {
		return nil, nil, fmt.Errorf("Aussteller nicht erreichbar: %w", err)
	}
	bereiche := strings.Fields(o.Bereiche)
	if len(bereiche) == 0 {
		bereiche = []string{oidc.ScopeOpenID, "email", "profile"}
	}
	return anbieter, &oauth2.Config{
		ClientID:     o.ClientID,
		ClientSecret: o.Geheimnis,
		Endpoint:     anbieter.Endpoint(),
		RedirectURL:  s.rueckAdresse(),
		Scopes:       bereiche,
	}, nil
}

// rueckAdresse is the address the issuer sends the browser back to. It has to
// be registered with the issuer, which is why it is built from the configured
// public URL and not from the request header: an attacker who sets the Host
// header could otherwise redirect the callback.
func (s *Server) rueckAdresse() string {
	basis := strings.TrimSuffix(s.oeffentlicheURL(), "/")
	return basis + "/api/auth/oidc/callback"
}

// OIDCStart sends the browser to the provider.
func (s *Server) OIDCStart(w http.ResponseWriter, r *http.Request) {
	if !lizenz.Frei(lizenz.SSO) {
		writeErr(w, http.StatusPaymentRequired, "SSO gehört zum Zusatzumfang")
		return
	}
	if s.oeffentlicheURL() == "" {
		writeErr(w, http.StatusPreconditionFailed,
			"oeffentliche_url ist nicht gesetzt. Ohne sie gibt es keine Rücksprungadresse.")
		return
	}
	_, oauthKonf, err := s.oidcAnbieter(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	zustand, nonce := zufall(), zufall()
	// State and nonce travel in a cookie of their own rather than in the
	// service's memory: otherwise a sign-in under way would not survive a
	// restart, and two instances behind a load balancer could not take turns.
	s.oidcKeksSetzen(w, r, oidcSitzung{
		Zustand: zustand,
		Nonce:   nonce,
		Ziel:    r.URL.Query().Get("target"),
		Bis:     time.Now().Add(10 * time.Minute),
	})
	http.Redirect(w, r, oauthKonf.AuthCodeURL(zustand, oidc.Nonce(nonce)), http.StatusFound)
}

// OIDCZurueck takes the code and signs the person in.
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

	// The state parameter is the protection against smuggled-in sign-ins:
	// without comparing it, somebody could push a foreign code into a victim's
	// browser and sign them into the attacker's account.
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
	idToken, err := anbieter.Verifier(&oidc.Config{ClientID: s.oidcJetzt().ClientID}).
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

	k := s.oidcJetzt()
	email := strings.ToLower(strings.TrimSpace(behauptungText(behauptung, k.FeldEmail, "email")))
	name := strings.TrimSpace(behauptungText(behauptung, k.FeldName, "name"))
	if email == "" {
		s.ssoFehler(w, r, "Der Anbieter hat keine E-Mail-Adresse mitgeschickt.")
		return
	}
	// An unverified address does not match an account: otherwise registering at
	// the provider with somebody else's address would be enough to read their
	// pages here.
	if bestaetigt, da := behauptung["email_verified"].(bool); da && !bestaetigt {
		s.ssoFehler(w, r, "Der Anbieter hat diese Adresse nicht bestätigt.")
		return
	}

	admin := gruppeEnthaelt(behauptung, k.GruppeAdmin)
	// Only an explicit "verified" counts for an account with a password: a
	// provider that does not say is not taken at its word there.
	bestaetigt, _ := behauptung["email_verified"].(bool)
	u, err := s.kontoAusSSO(r.Context(), email, name, admin, "oidc", bestaetigt)
	if err != nil {
		s.ssoFehler(w, r, err.Error())
		return
	}

	// The second factor hangs on the account, not on the way in, as with
	// LDAP. A released password account that has one set up is asked for the
	// code. The ticket goes in the fragment: it never reaches a server log or
	// a Referer, and the sign-in page picks it up from there.
	if s.zweitfaktorAktiv(r.Context(), u.ID) {
		ticket, err := auth.ZweitTicket(s.Secret, u.ID)
		if err != nil {
			s.ssoFehler(w, r, "Die Anmeldung ließ sich nicht fortsetzen.")
			return
		}
		http.Redirect(w, r, "/login#zweit="+url.QueryEscape(ticket), http.StatusFound)
		return
	}

	s.issueSession(w, r, u.ID)
	s.anmeldeSpur(r, WegSSO, email, "", &u)

	ziel := mit.Ziel
	// Only targets inside this application. Anything else would be an open
	// redirect, a favourite building block for phishing pages.
	//
	// Both slashes are checked: "//other.example" is an address using the
	// current page's protocol, and some browsers treat "/\other.example" the
	// same way.
	if !strings.HasPrefix(ziel, "/") ||
		strings.HasPrefix(ziel, "//") || strings.HasPrefix(ziel, `/\`) {
		ziel = "/"
	}
	http.Redirect(w, r, ziel, http.StatusFound)
}

// kontoAusSSO finds or creates the account.
//
// adresseBestaetigt says whether the provider vouched for the address. A
// released account with a password is taken over only then.
func (s *Server) kontoAusSSO(ctx context.Context, email, name string, admin bool, herkunft string, adresseBestaetigt bool) (models.User, error) {
	var u models.User
	var hash string
	var freigegeben bool
	err := s.Pool.QueryRow(ctx,
		`SELECT id, email, name, coalesce(benutzername, ''), password_hash, role, created_at, sso_freigegeben
		 FROM users WHERE email=$1`,
		email).Scan(&u.ID, &u.Email, &u.Name, &u.Benutzername, &hash, &u.Role, &u.CreatedAt, &freigegeben)

	if err == nil {
		// Only an account that was EXPLICITLY created through SSO is taken
		// over. Every other one, with a password or with an empty password
		// field, is left alone.
		//
		// The earlier version let an empty password field pass. That was a
		// hole: whoever can set an address in the directory would have attached
		// themselves to a stranger's account as soon as its field was empty for
		// whatever reason.
		//
		// An account with a password counts only once an administrator has
		// released it in the account dialog (sso_freigegeben). Its role is
		// then left as it is: the password account was given its role by a
		// person, not by a group in the directory.
		if !strings.HasPrefix(hash, "sso:") {
			if freigegeben && adresseBestaetigt {
				return u, nil
			}
			return u, errors.New("Für diese Adresse gibt es bereits ein Konto. " +
				"Ein Administrator muss es freigeben, bevor SSO darauf zugreift.")
		}
		// Promote only an SSO account, and write it to the audit trail: a role
		// that changes quietly is a role nobody notices changing.
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

	// A new account. The password placeholder starts with "sso:" and is not a
	// valid bcrypt hash, so a password sign-in fails on it reliably without
	// needing a column of its own.
	if name == "" {
		name = strings.SplitN(email, "@", 2)[0]
	}
	rolle := "user"
	if admin {
		rolle = "admin"
	}
	// The first account on an empty instance becomes an administrator, the same
	// rule as for registering with a password.
	var vorhanden int
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&vorhanden)
	if vorhanden == 0 {
		rolle = "admin"
	}

	// An account from the directory gets a user name too, so that it is not
	// later the only one addressable by address alone. If nothing usable
	// remains, the field stays empty.
	benutzername := s.freierBenutzername(ctx, benutzernameAusAdresse(email))

	err = s.Pool.QueryRow(ctx,
		`INSERT INTO users (email, name, benutzername, password_hash, role)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id, email, name, coalesce(benutzername, ''), role, created_at`,
		email, name, leerAlsNull(benutzername), "sso:"+herkunft, rolle).
		Scan(&u.ID, &u.Email, &u.Name, &u.Benutzername, &u.Role, &u.CreatedAt)
	if err != nil {
		return u, errors.New("Konto konnte nicht angelegt werden.")
	}
	log.Printf("SSO (%s): Konto für %s angelegt, Rolle %s", herkunft, email, rolle)
	return u, nil
}

// ssoFehler sends the browser back to the sign-in page with a message.
//
// Not JSON: at this point the browser is in the middle of a redirect, and a
// JSON answer would be a white page full of curly braces.
func (s *Server) ssoFehler(w http.ResponseWriter, r *http.Request, meldung string) {
	log.Printf("SSO abgebrochen: %s", meldung)
	// QueryEscape rather than a hand-written replacement list: such a list
	// always forgets one character, and that one turns out to be the one that
	// breaks the address open.
	http.Redirect(w, r, "/login?sso="+url.QueryEscape(meldung), http.StatusFound)
}

func zufall() string {
	b := make([]byte, 24)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// behauptungText reads a claim, falling back to the usual field name.
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

// gruppeEnthaelt looks for a group name in the usual claims.
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
			// Keycloak keeps the roles under realm_access.roles.
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

// SSOPruefen checks the OIDC values as they stand: is the issuer reachable,
// does it publish a discovery document, is a public address there for the
// callback. Admins only. It cannot check the client secret, that only shows
// at the first real sign-in.
func (s *Server) SSOPruefen(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	o := s.oidcJetzt()
	antwort := map[string]any{
		"callback": "",
		"licensed": lizenz.Frei(lizenz.SSO),
		"enabled":  o.Aktiv == "ja",
	}
	if u := s.oeffentlicheURL(); u != "" {
		antwort["callback"] = s.rueckAdresse()
	}
	switch {
	case o.Aussteller == "":
		antwort["ok"], antwort["message"] = false, "Kein Aussteller eingetragen."
	case o.ClientID == "":
		antwort["ok"], antwort["message"] = false, "Keine Client-ID eingetragen."
	case s.oeffentlicheURL() == "":
		antwort["ok"], antwort["message"] = false, "Keine öffentliche Adresse eingetragen, ohne sie gibt es keine Rücksprungadresse."
	default:
		ctx, abbruch := context.WithTimeout(r.Context(), 10*time.Second)
		defer abbruch()
		if _, _, err := s.oidcVerbinden(ctx, o); err != nil {
			antwort["ok"], antwort["message"] = false, err.Error()
		} else {
			antwort["ok"], antwort["message"] = true, "Aussteller erreichbar, Discovery-Dokument gelesen."
		}
	}
	writeJSON(w, http.StatusOK, antwort)
}
