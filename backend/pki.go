package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// "nexora pki" sets up the certificates for the internal traffic of the
// compound and ends. docker/docker-compose.yml runs it once as the service
// "pki" (same image as the backend, as root so it can hand each key to its
// owner); db, redis, minio and the backend wait until it has finished.
//
// It replaces the former pki/create.sh with openssl. Repeatable: whatever
// already lies in the directory stays there, so restarting the compound never
// changes a key. The layout is the same as before:
//
//	/pki/ca.crt, /pki/ca.key                  the authority of our own
//	/pki/<service>/<service>.crt and .key     db, redis, backend, ...
//	/pki/minio/public.crt, private.key        the names MinIO expects
//	/pki/minio/CAs/nexora.crt                 the authority for MinIO
//
// Settings: PKI_DIR (default /pki), PKI_DAYS (3650), PKI_SERVICES
// ("backend db minio redis"); the old German names PKI_VERZ, PKI_TAGE and
// PKI_DIENSTE are still read.
func pki() int {
	dir := envOr("/pki", "PKI_DIR", "PKI_VERZ")
	days, err := strconv.Atoi(envOr("3650", "PKI_DAYS", "PKI_TAGE"))
	if err != nil || days < 1 {
		fmt.Fprintln(os.Stderr, "PKI: PKI_DAYS must be a positive number of days")
		return 1
	}
	services := strings.Fields(envOr("backend db minio redis", "PKI_SERVICES", "PKI_DIENSTE"))
	if err := runPKI(dir, days, services); err != nil {
		fmt.Fprintln(os.Stderr, "PKI:", err)
		return 1
	}
	fmt.Printf("PKI: done, %d certificates and one authority are in %s.\n", len(services), dir)
	return 0
}

func envOr(fallback string, names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return fallback
}

func runPKI(dir string, days int, services []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	validity := time.Duration(days) * 24 * time.Hour

	caCert, caKey, err := loadOrCreateCA(filepath.Join(dir, "ca.crt"), filepath.Join(dir, "ca.key"), validity)
	if err != nil {
		return fmt.Errorf("authority: %w", err)
	}

	for _, s := range services {
		sdir := filepath.Join(dir, s)
		if err := os.MkdirAll(sdir, 0o700); err != nil {
			return err
		}
		certPath := filepath.Join(sdir, certName(s))
		keyPath := filepath.Join(sdir, keyName(s))
		if exists(certPath) && exists(keyPath) {
			fmt.Printf("PKI: %s already has a certificate\n", s)
		} else {
			fmt.Printf("PKI: issuing a certificate for %s\n", s)
			if err := issue(s, certPath, keyPath, caCert, caKey, validity); err != nil {
				return fmt.Errorf("%s: %w", s, err)
			}
		}
		if s == "minio" {
			if err := os.MkdirAll(filepath.Join(sdir, "CAs"), 0o755); err != nil {
				return err
			}
			if err := copyFile(filepath.Join(dir, "ca.crt"), filepath.Join(sdir, "CAs", "nexora.crt"), 0o644); err != nil {
				return err
			}
		}
		uid, gid := ownerFor(s)
		if err := chownAll(sdir, uid, gid); err != nil {
			fmt.Printf("PKI: cannot set the owner of %s, it may not be able to read its key\n", s)
		}
		_ = os.Chmod(sdir, 0o700)
		_ = os.Chmod(keyPath, 0o600)
		_ = os.Chmod(certPath, 0o644)
	}
	return nil
}

func certName(service string) string {
	if service == "minio" {
		return "public.crt"
	}
	return service + ".crt"
}

func keyName(service string) string {
	if service == "minio" {
		return "private.key"
	}
	return service + ".key"
}

// ownerFor names the user each service runs as in its image, so it can read
// its own key and nobody else's.
func ownerFor(service string) (int, int) {
	switch service {
	case "db":
		return 70, 70 // postgres in the alpine image
	case "redis":
		return 999, 1000 // redis in the alpine image
	case "backend":
		return 10001, 10001 // nexora, see backend/Dockerfile
	default:
		return 0, 0 // minio and everything else runs as root
	}
}

func loadOrCreateCA(certPath, keyPath string, validity time.Duration) (*x509.Certificate, *rsa.PrivateKey, error) {
	if exists(certPath) && exists(keyPath) {
		fmt.Println("PKI: using the authority that is already there")
		cert, err := readCert(certPath)
		if err != nil {
			return nil, nil, err
		}
		key, err := readKey(keyPath)
		if err != nil {
			return nil, nil, err
		}
		return cert, key, nil
	}
	fmt.Println("PKI: setting up a certificate authority of our own")
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "Nexora internal authority"},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(validity),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		SubjectKeyId:          keyID(&key.PublicKey),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	if err := writePEM(keyPath, "PRIVATE KEY", mustPKCS8(key), 0o600); err != nil {
		return nil, nil, err
	}
	if err := writePEM(certPath, "CERTIFICATE", der, 0o644); err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	return cert, key, err
}

func issue(service, certPath, keyPath string, ca *x509.Certificate, caKey *rsa.PrivateKey, validity time.Duration) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: service},
		DNSNames:              []string{service, "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(validity),
		BasicConstraintsValid: true,
		IsCA:                  false,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		SubjectKeyId:          keyID(&key.PublicKey),
		AuthorityKeyId:        ca.SubjectKeyId,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return err
	}
	if err := writePEM(keyPath, "PRIVATE KEY", mustPKCS8(key), 0o600); err != nil {
		return err
	}
	return writePEM(certPath, "CERTIFICATE", der, 0o644)
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		panic(err)
	}
	return n
}

func keyID(pub *rsa.PublicKey) []byte {
	sum := sha256.Sum256(x509.MarshalPKCS1PublicKey(pub))
	return sum[:20]
}

func mustPKCS8(key *rsa.PrivateKey) []byte {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		panic(err)
	}
	return der
}

// writePEM writes atomically: a half-written key after a crash would be worse
// than none, because the next run would keep it.
func writePEM(path, typ string, der []byte, mode fs.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if err := pem.Encode(f, &pem.Block{Type: typ, Bytes: der}); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readCert(path string) (*x509.Certificate, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return nil, errors.New(path + " is not PEM")
	}
	return x509.ParseCertificate(blk.Bytes)
}

// readKey accepts PKCS#8 (what openssl and this file write) and PKCS#1.
func readKey(path string) (*rsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return nil, errors.New(path + " is not PEM")
	}
	if k, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, errors.New(path + " is not an RSA key")
	}
	return x509.ParsePKCS1PrivateKey(blk.Bytes)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copyFile(src, dst string, mode fs.FileMode) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, mode)
}

func chownAll(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(p, uid, gid)
	})
}
