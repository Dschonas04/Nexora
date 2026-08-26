package handlers

import (
	"io"
	"strings"
)

// stringLeser turns a string into a reader. strings.NewReader would do as well;
// the function of our own exists so that the call in the connection test stays
// readable.
func stringLeser(s string) io.Reader { return strings.NewReader(s) }
