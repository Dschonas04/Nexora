// Whom this service trusts on its way out.
//
// Nexora talks to database, object store and cache inside the compound, and
// those connections are encrypted. Encrypted alone is worth little, though:
// whoever does not check who they are talking to may well be talking
// encrypted to the wrong party. The check runs against the compound's own
// small authority, see pki/erzeuge.sh.
//
// That authority is ADDED to the public ones and does not replace them.
// Otherwise the service would lose its trust in every identity provider on the
// net -- a Keycloak behind a Let's Encrypt certificate would suddenly be
// unreachable, and nobody would understand why setting up a private
// certificate authority breaks signing in.
package vertrauen

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Wurzeln reads an additional authority and appends it to the system's.
//
// An empty path yields nil, and nil means to every caller: take what the
// system knows. That is exactly how the Go library behaves with RootCAs unset,
// so the case without a private authority needs no special path.
func Wurzeln(pfad string) (*x509.CertPool, error) {
	pfad = strings.TrimSpace(pfad)
	if pfad == "" {
		return nil, nil
	}
	roh, err := os.ReadFile(pfad)
	if err != nil {
		return nil, fmt.Errorf("Zertifizierungsstelle %s: %w", pfad, err)
	}
	// The system as the base, and if it yields none (an image without
	// ca-certificates, say) we simply start empty: the private authority is the
	// reason this function was called at all.
	vorrat, err := x509.SystemCertPool()
	if err != nil || vorrat == nil {
		vorrat = x509.NewCertPool()
	}
	if !vorrat.AppendCertsFromPEM(roh) {
		return nil, errors.New("in " + pfad + " steht kein lesbares Zertifikat")
	}
	return vorrat, nil
}
