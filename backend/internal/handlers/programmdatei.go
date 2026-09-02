// Programme kommen hier nicht herein.
//
// Ein Wiki nimmt Dateien an, damit Leute sie wiederfinden: Rechnungen, Bilder,
// Tabellen, ein Handbuch. Ein ausführbares Programm gehört nicht dazu. Es ist
// nicht bloß nutzlos an dieser Stelle -- es ist der bequemste Weg, etwas in ein
// Haus zu tragen, in dem alle einander vertrauen: einmal hochgeladen, liegt es
// unter einer Adresse, die jeder Mitlesende anklicken kann, und der Anhang
// eines Protokolls sieht harmlos aus.
//
// Erkannt wird an den ersten Bytes und nicht an der Endung. Eine Endung ist
// eine Behauptung des Hochladenden; die vier Bytes am Anfang sind das, was der
// Kern beim Starten liest. Wer ein Programm "notizen.txt" nennt, wird hier
// trotzdem abgewiesen, und wer eine Textdatei "start.sh" nennt, kommt durch.
package handlers

import (
	"bytes"
	"errors"
)

// errProgrammdatei meldet die Ablehnung dort, wo kein Antwortschreiber zur Hand
// ist -- bei der Einfuhr aus einem Archiv.
var errProgrammdatei = errors.New(programmMeldung)

// elfMagie steht am Anfang jeder ausführbaren Datei unter Linux: 0x7F, dann
// "ELF". Dieselben vier Bytes tragen auch die Bibliotheken (.so) und die
// Kernmodule, und das ist richtig so -- ausgeführt wird das eine wie das
// andere.
var elfMagie = []byte{0x7F, 'E', 'L', 'F'}

// istLinuxProgramm sagt, ob diese Bytes ein ausführbares Programm für Linux
// sind.
//
// Nur Linux, wie gewünscht. Ein Windows-Programm (MZ) und ein macOS-Programm
// (Mach-O) laufen auf dem Rechner, der diese Dateien hält, ohnehin nicht, und
// ein Wiki, das eine mitgelieferte setup.exe eines Herstellers nicht mehr
// aufbewahren darf, verliert einen Zweck, für den es gedacht ist.
func istLinuxProgramm(anfang []byte) bool {
	return bytes.HasPrefix(anfang, elfMagie)
}

// programmMeldung ist der Text, den der Hochladende zu sehen bekommt. Er sagt,
// was erkannt wurde und warum -- eine Ablehnung, die nur "nicht erlaubt" sagt,
// liest sich wie ein Fehler des Dienstes.
const programmMeldung = "ausführbare Programme werden nicht angenommen: " +
	"diese Datei ist ein Linux-Programm (ELF), egal wie sie heißt"
