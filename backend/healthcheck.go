package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	"nexora/internal/config"
)

// healthcheck asks the running server in the same container for /healthz and
// returns the exit code for the container runtime: 0 when the server answers
// and reaches its database, 1 otherwise. It is part of the binary so the image
// needs neither curl nor wget.
func healthcheck() int {
	k := config.Laden("")
	schema := "http"
	transport := &http.Transport{}
	if k.TLSZertifikat != "" && k.TLSSchluessel != "" {
		schema = "https"
		// The certificate names the service, not 127.0.0.1. The request never
		// leaves the container, so there is nothing to verify it against.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	client := &http.Client{Timeout: 4 * time.Second, Transport: transport}
	resp, err := client.Get(schema + "://127.0.0.1:" + k.Port + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}
