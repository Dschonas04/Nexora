//go:build !nur_kern

// This file is what connects the paid extras to the core. Importing the premium
// package runs its init(), which registers the license verifier; without that
// import the gate in internal/lizenz never gets a verifier and every extra
// stays locked.
//
// Build the free core alone with:
//
//	rm -rf premium && go build -tags nur_kern ./...
//
// premium_aus.go takes over under that tag, so the tree still compiles.
package main

import _ "nexora/premium/lizenz"
