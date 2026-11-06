package main

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPKIIssuesAndKeeps(t *testing.T) {
	dir := t.TempDir()
	services := []string{"backend", "db", "minio", "redis"}
	if err := runPKI(dir, 30, services); err != nil {
		t.Fatal(err)
	}

	ca, err := readCert(filepath.Join(dir, "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	if !ca.IsCA {
		t.Fatal("ca.crt is not a CA")
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)

	for _, s := range services {
		c, err := readCert(filepath.Join(dir, s, certName(s)))
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		if _, err := c.Verify(x509.VerifyOptions{DNSName: s, Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			t.Fatalf("%s does not verify against the CA: %v", s, err)
		}
		if _, err := c.Verify(x509.VerifyOptions{DNSName: "localhost", Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			t.Fatalf("%s lacks localhost: %v", s, err)
		}
		if c.NotAfter.Sub(time.Now()) < 29*24*time.Hour {
			t.Fatalf("%s validity too short", s)
		}
		if _, err := readKey(filepath.Join(dir, s, keyName(s))); err != nil {
			t.Fatalf("%s key: %v", s, err)
		}
		st, _ := os.Stat(filepath.Join(dir, s, keyName(s)))
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("%s key mode %v", s, st.Mode().Perm())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "minio", "CAs", "nexora.crt")); err != nil {
		t.Fatal("minio/CAs/nexora.crt missing")
	}

	before, _ := os.ReadFile(filepath.Join(dir, "db", "db.key"))
	caBefore, _ := os.ReadFile(filepath.Join(dir, "ca.key"))
	if err := runPKI(dir, 30, services); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "db", "db.key"))
	caAfter, _ := os.ReadFile(filepath.Join(dir, "ca.key"))
	if string(before) != string(after) || string(caBefore) != string(caAfter) {
		t.Fatal("second run changed existing keys")
	}
}
