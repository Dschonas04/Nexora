// Command schluessel issues and inspects Nexora license keys.
//
// It is the only place the private signing key is used. That key must stay with
// the licensor and must never end up in the repository: whoever holds it can
// mint keys for every extra, and since verification is offline, such a key
// cannot be revoked afterwards.
//
// Usage:
//
//	schluessel -neu
//	    generates a fresh key pair and prints both halves
//
//	schluessel -inhaber "Firma X" -funktionen versionen,anhaenge -ablauf 2027-12-31
//	    signs a key; the private key comes from NEXORA_SIGNIERSCHLUESSEL
//
//	schluessel -zeige <schlüssel>
//	    prints what a key contains, without checking the signature
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	kern "nexora/internal/lizenz"
	plizenz "nexora/premium/lizenz"
)

func main() {
	neu := flag.Bool("neu", false, "neues Schlüsselpaar erzeugen")
	inhaber := flag.String("inhaber", "", "auf wen die Lizenz ausgestellt wird")
	funktionen := flag.String("funktionen", "", "Komma-Liste, oder 'alle'")
	ablauf := flag.String("ablauf", "", "Ablaufdatum JJJJ-MM-TT, leer heißt unbefristet")
	zeige := flag.String("zeige", "", "Inhalt eines Schlüssels anzeigen")
	flag.Parse()

	switch {
	case *neu:
		schluesselpaar()
	case *zeige != "":
		anzeigen(*zeige)
	case *inhaber != "":
		ausstellen(*inhaber, *funktionen, *ablauf)
	default:
		flag.Usage()
		fmt.Fprintf(os.Stderr, "\nBekannte Funktionen: %s\n", namen(kern.Alle))
		os.Exit(2)
	}
}

func schluesselpaar() {
	oeff, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fatal("Schlüsselpaar konnte nicht erzeugt werden: %v", err)
	}
	fmt.Println("Öffentlicher Schlüssel -- gehört als Konstante nach premium/lizenz/pruefer.go:")
	fmt.Println()
	fmt.Println("  " + base64.RawURLEncoding.EncodeToString(oeff))
	fmt.Println()
	fmt.Println("Privater Schlüssel -- NIEMALS ins Repository, sicher verwahren:")
	fmt.Println()
	fmt.Println("  " + base64.RawURLEncoding.EncodeToString(priv))
	fmt.Println()
	fmt.Println("Zum Ausstellen als NEXORA_SIGNIERSCHLUESSEL setzen.")
}

func ausstellen(inhaber, funktionen, ablauf string) {
	rohPriv := os.Getenv("NEXORA_SIGNIERSCHLUESSEL")
	if rohPriv == "" {
		fatal("NEXORA_SIGNIERSCHLUESSEL ist nicht gesetzt")
	}
	priv, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(rohPriv))
	if err != nil || len(priv) != ed25519.PrivateKeySize {
		fatal("NEXORA_SIGNIERSCHLUESSEL ist kein gültiger privater Ed25519-Schlüssel")
	}

	var liste []string
	if funktionen == "alle" || funktionen == "" {
		for _, f := range kern.Alle {
			liste = append(liste, string(f))
		}
	} else {
		for _, teil := range strings.Split(funktionen, ",") {
			teil = strings.TrimSpace(teil)
			if !bekannt(teil) {
				fatal("unbekannte Funktion %q -- bekannt sind: %s", teil, namen(kern.Alle))
			}
			liste = append(liste, teil)
		}
	}

	if ablauf != "" {
		if _, err := time.Parse("2006-01-02", ablauf); err != nil {
			fatal("Ablaufdatum %q ist nicht JJJJ-MM-TT", ablauf)
		}
	}

	n := plizenz.Nutzlast{
		Inhaber:     inhaber,
		Funktionen:  liste,
		Ablauf:      ablauf,
		Ausgestellt: time.Now().Format("2006-01-02"),
	}
	daten, err := json.Marshal(n)
	if err != nil {
		fatal("Daten konnten nicht verpackt werden: %v", err)
	}
	sig := ed25519.Sign(ed25519.PrivateKey(priv), daten)

	fmt.Printf("Lizenz für %s", inhaber)
	if ablauf != "" {
		fmt.Printf(", gültig bis %s", ablauf)
	} else {
		fmt.Printf(", unbefristet")
	}
	fmt.Printf("\nFunktionen: %s\n\n", strings.Join(liste, ", "))
	fmt.Println(base64.RawURLEncoding.EncodeToString(daten) + "." +
		base64.RawURLEncoding.EncodeToString(sig))
}

func anzeigen(schluessel string) {
	teile := strings.Split(strings.TrimSpace(schluessel), ".")
	if len(teile) != 2 {
		fatal("Schlüssel hat nicht die Form <Daten>.<Signatur>")
	}
	daten, err := base64.RawURLEncoding.DecodeString(teile[0])
	if err != nil {
		fatal("Datenteil ist unlesbar")
	}
	var n plizenz.Nutzlast
	if err := json.Unmarshal(daten, &n); err != nil {
		fatal("Daten sind kein gültiges JSON")
	}
	fmt.Printf("Inhaber:      %s\n", n.Inhaber)
	fmt.Printf("Funktionen:   %s\n", strings.Join(n.Funktionen, ", "))
	fmt.Printf("Ausgestellt:  %s\n", n.Ausgestellt)
	if n.Ablauf == "" {
		fmt.Println("Ablauf:       unbefristet")
	} else {
		fmt.Printf("Ablauf:       %s\n", n.Ablauf)
	}
	fmt.Println()
	fmt.Println("Hinweis: die Signatur wird hier NICHT geprüft, nur der Inhalt gezeigt.")
}

func bekannt(s string) bool {
	for _, f := range kern.Alle {
		if string(f) == s {
			return true
		}
	}
	return false
}

func namen(fs []kern.Funktion) string {
	var s []string
	for _, f := range fs {
		s = append(s, string(f))
	}
	return strings.Join(s, ", ")
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
