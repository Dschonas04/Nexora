package middleware

import (
	"net/http"
	"strings"
)

// Sicherheitskopfzeilen sets the headers a browser needs in order to treat a
// response defensively.
//
// The filter sits in front of every route, including the ones that serve
// uploaded files. It only ever sets, never overwrites what a handler decides
// later: a handler calling Header().Set replaces this value, which is exactly
// what dateiausgabe.go needs when it hands out an attachment under a stricter
// policy than the API needs.
//
// The policy here is written for the API. What comes out of these routes is
// JSON, and JSON needs nothing at all: no script, no style, no image. Should a
// browser be persuaded to interpret an answer as a document anyway, that
// document may then load nothing.
//
// The policy for the application itself does not live here, it lives in the
// nginx that serves index.html. That is where a policy can name the bundles it
// permits; this service never sees them.
func Sicherheitskopfzeilen(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k := w.Header()

		// Do not guess the type. Without this a text file that a browser
		// considers to look like HTML would be treated as HTML.
		k.Set("X-Content-Type-Options", "nosniff")

		// No frame, ever. frame-ancestors is the current form and
		// X-Frame-Options the older one; both stand here because it costs one
		// line and there are still browsers that only know the second.
		k.Set("X-Frame-Options", "DENY")

		// The API knows no cross-origin caller: the SPA arrives through the
		// same nginx and is therefore the same origin. Whatever wants to
		// embed an answer from elsewhere has no business doing so.
		k.Set("Cross-Origin-Resource-Policy", "same-origin")

		// Nothing may be loaded from an API answer, and nobody may frame it.
		k.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")

		// Page ids sit in the paths of this service. A referrer would carry
		// them to whatever a user clicks on next.
		k.Set("Referrer-Policy", "no-referrer")

		// HSTS only where it is true. Sent over plain HTTP the header is
		// ignored by browsers, and promising encryption an installation does
		// not offer would lock out an instance that runs open on purpose.
		// The proxy header is what counts: nginx terminates TLS and this
		// service usually sees the request unencrypted afterwards.
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			k.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}
