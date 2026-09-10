package handlers

import "testing"

// Detection goes by the first bytes, not by the extension: an extension is a
// claim made by whoever uploads.
func TestLinuxProgrammWirdErkannt(t *testing.T) {
	faelle := []struct {
		name    string
		bytes   []byte
		program bool
	}{
		{"ELF, 64 Bit", []byte{0x7F, 'E', 'L', 'F', 2, 1, 1, 0}, true},
		{"ELF, 32 Bit", []byte{0x7F, 'E', 'L', 'F', 1, 1, 1, 0}, true},
		{"nur die vier Bytes", []byte{0x7F, 'E', 'L', 'F'}, true},
		{"Text", []byte("Guten Morgen, das ist eine Notiz."), false},
		{"Bild (PNG)", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A}, false},
		{"PDF", []byte("%PDF-1.7"), false},
		// A script is expressly NOT rejected: it is text, and a wiki that may no
		// longer keep a documented backup script loses one of its purposes.
		{"Shell-Skript", []byte("#!/bin/sh\necho hallo\n"), false},
		// Windows and macOS stay outside the rule as well: Linux was what was
		// asked about, and neither runs on the host of these files anyway.
		{"Windows-Programm", []byte{'M', 'Z', 0x90, 0x00}, false},
		{"leer", nil, false},
		{"zu kurz", []byte{0x7F, 'E'}, false},
		// The four bytes have to stand at the BEGINNING. In the middle they are
		// content, in an archive say, or in a text about file formats.
		{"ELF mittendrin", []byte("siehe \x7fELF weiter unten"), false},
	}
	for _, f := range faelle {
		if raus := istLinuxProgramm(f.bytes); raus != f.program {
			t.Errorf("%s: erwartet %v, bekam %v", f.name, f.program, raus)
		}
	}
}
