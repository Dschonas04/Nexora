package handlers

import (
	"io"
	"strings"
)

// stringLeser macht aus einer Zeichenkette einen Leser. strings.NewReader
// täte es auch; die eigene Funktion existiert, damit der Aufruf im
// Verbindungstest lesbar bleibt.
func stringLeser(s string) io.Reader { return strings.NewReader(s) }
