package vertrauen

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func stelleSchreiben(t *testing.T, pfad string) {
	t.Helper()
	schluessel, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	vorlage := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Probe-Stelle"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	roh, err := x509.CreateCertificate(rand.Reader, &vorlage, &vorlage, &schluessel.PublicKey, schluessel)
	if err != nil {
		t.Fatal(err)
	}
	datei, err := os.Create(pfad)
	if err != nil {
		t.Fatal(err)
	}
	defer datei.Close()
	if err := pem.Encode(datei, &pem.Block{Type: "CERTIFICATE", Bytes: roh}); err != nil {
		t.Fatal(err)
	}
}

// Ohne Pfad bleibt es bei den Stellen des Systems, und die stehen fuer nil.
func TestOhnePfadKeinVorrat(t *testing.T) {
	vorrat, err := Wurzeln("")
	if err != nil || vorrat != nil {
		t.Fatalf("erwartet nil ohne Fehler, bekam %v / %v", vorrat, err)
	}
	if vorrat, err := Wurzeln("   "); err != nil || vorrat != nil {
		t.Fatalf("Leerraum ist auch kein Pfad, bekam %v / %v", vorrat, err)
	}
}

// Die eigene Stelle kommt HINZU: waere sie ein Ersatz, verloere der Dienst das
// Vertrauen zu jedem oeffentlichen Anmeldedienst.
func TestEigeneStelleKommtHinzu(t *testing.T) {
	pfad := filepath.Join(t.TempDir(), "ca.crt")
	stelleSchreiben(t, pfad)

	vorrat, err := Wurzeln(pfad)
	if err != nil {
		t.Fatal(err)
	}
	if vorrat == nil {
		t.Fatal("kein Vorrat trotz Datei")
	}
	system, err := x509.SystemCertPool()
	if err != nil || system == nil {
		t.Skip("dieses System hat keinen eigenen Vorrat, der Vergleich entfaellt")
	}
	// Eine Stelle mehr als das System: genau die eine aus der Datei.
	if len(vorrat.Subjects()) <= len(system.Subjects()) { //nolint:staticcheck // Subjects reicht hier zum Zaehlen
		t.Fatalf("der Vorrat wuchs nicht: %d gegen %d",
			len(vorrat.Subjects()), len(system.Subjects())) //nolint:staticcheck
	}
}

// Eine fehlende oder unlesbare Datei ist ein Fehler und keine stille Null:
// sonst liefe der Dienst ohne die Stelle weiter, die jemand ausdruecklich
// eingetragen hat, und scheiterte spaeter an einer Verbindung.
func TestFehlendeDateiIstEinFehler(t *testing.T) {
	if _, err := Wurzeln(filepath.Join(t.TempDir(), "gibtsnicht.crt")); err == nil {
		t.Fatal("fehlende Datei ohne Fehler")
	}
	murks := filepath.Join(t.TempDir(), "murks.crt")
	if err := os.WriteFile(murks, []byte("kein Zertifikat"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Wurzeln(murks); err == nil {
		t.Fatal("Unsinn in der Datei ohne Fehler")
	}
}
