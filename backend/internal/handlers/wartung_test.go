package handlers

import (
	"strings"
	"testing"
)

// The credentials shall not travel into the browser, but saving must not
// replace them with asterisks either. Both together make a round trip that is
// correct exactly when the original stands there again afterwards.
func TestGeheimnisseUeberlebenDenRundlauf(t *testing.T) {
	alt := `# Kopf
[Datenbank]
datenbank_url = postgres://nexora:s3hr%20geheim@db:5432/nexora
port = 8080

[Sitzungen]
jwt_geheimnis = abcdef123456
; ein Kommentar
oidc_geheimnis =
`
	gezeigt := verstecken(alt)
	if strings.Contains(gezeigt, "s3hr%20geheim") || strings.Contains(gezeigt, "abcdef123456") {
		t.Fatalf("Geheimnis steht noch drin:\n%s", gezeigt)
	}
	if !strings.Contains(gezeigt, "port = 8080") {
		t.Error("harmlose Zeile wurde mit versteckt")
	}
	// An empty value stays empty, otherwise asterisks would stand there that one
	// would take for a real value while saving.
	if !strings.Contains(gezeigt, "oidc_geheimnis =\n") {
		t.Error("leerer Wert wurde zu Sternen")
	}

	zurueck := zurueckSetzen(gezeigt, alt)
	if zurueck != alt {
		t.Errorf("Rundlauf verändert die Datei:\n--- vorher ---\n%s\n--- nachher ---\n%s", alt, zurueck)
	}
}

// Whoever really changes a value must have their input arrive; the restoring
// step may only replace the asterisks.
func TestGeaendertesGeheimnisWirdUebernommen(t *testing.T) {
	alt := "jwt_geheimnis = altwert\nport = 8080\n"
	entwurf := "jwt_geheimnis = neuwert\nport = 9090\n"
	got := zurueckSetzen(entwurf, alt)
	if !strings.Contains(got, "jwt_geheimnis = neuwert") {
		t.Errorf("neuer Wert ging verloren: %q", got)
	}
	if !strings.Contains(got, "port = 9090") {
		t.Errorf("geänderter Port ging verloren: %q", got)
	}
}
