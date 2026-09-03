// How large a transfer can actually be.
//
// The `max_anhang_mb` setting is only the last of several limits. In front of
// Nexora there is typically at least one nginx and often a second reverse
// proxy. If their `client_max_body_size` is smaller the transfer fails before
// Nexora ever sees it; the configured value would then be inaccurate and the
// user-facing error originates from a service the user does not know about.
//
// This cannot be calculated: Nexora does not know the configuration of the
// services in front of it and should not try to. But it can be measured. This
// endpoint accepts a payload and discards it; how far it makes it is reported
// by the browser that sent it. It measures exactly the path where a later
// failure would occur: from the browser through all intermediaries to here.
package handlers

import (
	"io"
	"net/http"

	"nexora/internal/middleware"
)

// Maximum amount the path will accept. Not as a protection against a large
// file (those are counted and discarded) but to prevent someone from
// occupying the connection indefinitely.
const grenzprobeMax = 512 << 20

func (s *Server) Grenzprobe(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	// Counted and discarded. The content is irrelevant; the goal is merely to
	// see whether it arrives. Persisting it would invite abusing this path as
	// a storage endpoint.
	n, err := io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, grenzprobeMax))
	if err != nil {
		// MaxBytesReader truncated the payload or the connection failed in
		// transit. Either way the result for the probe is the same: that many
		// bytes do not get through.
		writeErr(w, http.StatusRequestEntityTooLarge, "zu groß")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"bytes": n})
}
