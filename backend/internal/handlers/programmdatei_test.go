package handlers

import "testing"

// Erkannt wird an den ersten Bytes, nicht an der Endung: eine Endung ist eine
// Behauptung des Hochladenden.
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
		// Ein Skript wird ausdruecklich NICHT abgewiesen: es ist Text, und ein
		// Wiki, das ein dokumentiertes Sicherungsskript nicht mehr aufbewahren
		// darf, verliert einen seiner Zwecke.
		{"Shell-Skript", []byte("#!/bin/sh\necho hallo\n"), false},
		// Windows und macOS bleiben ebenfalls draussen vor der Regel: gefragt
		// war Linux, und beides laeuft auf dem Wirt dieser Dateien ohnehin nicht.
		{"Windows-Programm", []byte{'M', 'Z', 0x90, 0x00}, false},
		{"leer", nil, false},
		{"zu kurz", []byte{0x7F, 'E'}, false},
		// Die vier Bytes muessen am ANFANG stehen. Mittendrin sind sie Inhalt,
		// etwa in einem Archiv oder in einem Text ueber Dateiformate.
		{"ELF mittendrin", []byte("siehe \x7fELF weiter unten"), false},
	}
	for _, f := range faelle {
		if raus := istLinuxProgramm(f.bytes); raus != f.program {
			t.Errorf("%s: erwartet %v, bekam %v", f.name, f.program, raus)
		}
	}
}
