//go:build nur_kern

// Counterpart to premium_an.go for a build without the paid extras. It imports
// nothing on purpose: no verifier is registered, so internal/lizenz keeps every
// extra locked and the server runs on the free feature set alone.
package main
