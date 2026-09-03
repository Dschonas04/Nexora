// Executable programs are not accepted here.
//
// A wiki accepts files so people can find them later: invoices, images,
// spreadsheets, a manual. An executable program does not belong. It is not
// merely useless here — it is the easiest way to bring something into a
// trusted environment: once uploaded it sits at a URL anyone can click, and
// an attachment to a log looks harmless.
//
// Detection is based on the initial bytes, not on the filename extension.
// An extension is the uploader's claim; the four bytes at the start are what
// the kernel reads when executing. A file named "notizen.txt" that is an
// executable will still be rejected, and a text file called "start.sh" will
// pass.
package handlers

import (
	"bytes"
	"errors"
)

// errProgrammdatei reports a rejection where no response writer is available
// — for example when importing from an archive.
var errProgrammdatei = errors.New(programmMeldung)

// elfMagic appears at the start of every executable on Linux: 0x7F followed by
// "ELF". The same four bytes are present in shared libraries (.so) and
// kernel modules, and that is intentional — both are executed in the same
// manner.
var elfMagie = []byte{0x7F, 'E', 'L', 'F'}

// istLinuxProgramm reports whether the given bytes look like an executable
// for Linux.
//
// Only Linux, by design. A Windows program (MZ) or macOS binary (Mach-O)
// would not run on the host, and a wiki that refuses to hold a bundled
// setup.exe is losing a use case it was not intended to support.
func istLinuxProgramm(anfang []byte) bool {
	return bytes.HasPrefix(anfang, elfMagie)
}

// programmMeldung is the text shown to the uploader. It explains what was
// detected and why — a rejection that merely says "not allowed" reads like a
// service error.
const programmMeldung = "ausführbare Programme werden nicht angenommen: " +
	"diese Datei ist ein Linux-Programm (ELF), egal wie sie heißt"
