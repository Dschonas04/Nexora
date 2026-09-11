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

// Without a path the system's authorities stand, and those are what nil means.
func TestOhnePfadKeinVorrat(t *testing.T) {
	vorrat, err := Wurzeln("")
	if err != nil || vorrat != nil {
		t.Fatalf("erwartet nil ohne Fehler, bekam %v / %v", vorrat, err)
	}
	if vorrat, err := Wurzeln("   "); err != nil || vorrat != nil {
		t.Fatalf("Leerraum ist auch kein Pfad, bekam %v / %v", vorrat, err)
	}
}

// The private authority is ADDED: were it a replacement, the service would
// lose its trust in every public identity provider.
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
	// One authority more than the system's: exactly the one from the file.
	if len(vorrat.Subjects()) <= len(system.Subjects()) { //nolint:staticcheck // Subjects reicht hier zum Zaehlen
		t.Fatalf("der Vorrat wuchs nicht: %d gegen %d",
			len(vorrat.Subjects()), len(system.Subjects())) //nolint:staticcheck
	}
}

// A missing or unreadable file is an error and not a silent nil: otherwise the
// service would run on without the authority somebody expressly entered, and
// would fail later on a connection.
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
