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
//	schluessel -inhaber "Firma X" -stufe pro
//	    signs a key for one year; the private key comes from
//	    NEXORA_SIGNIERSCHLUESSEL
//
//	schluessel -inhaber "Firma X" -stufe advanced -funktionen gruppen
//	    a tier plus one extra bought on top
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
	stufe := flag.String("stufe", "", "free, advanced, pro oder business")
	funktionen := flag.String("funktionen", "", "einzelne Zusätze über die Stufe hinaus, Komma-Liste")
	ablauf := flag.String("ablauf", "", "Ablaufdatum JJJJ-MM-TT, leer heißt ein Jahr")
	zeige := flag.String("zeige", "", "Inhalt eines Schlüssels anzeigen")
	flag.Parse()

	switch {
	case *neu:
		schluesselpaar()
	case *zeige != "":
		anzeigen(*zeige)
	case *inhaber != "":
		ausstellen(*inhaber, *stufe, *funktionen, *ablauf)
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
	fmt.Println("Öffentlicher Schlüssel, gehört als Konstante nach premium/lizenz/pruefer.go:")
	fmt.Println()
	fmt.Println("  " + base64.RawURLEncoding.EncodeToString(oeff))
	fmt.Println()
	fmt.Println("Privater Schlüssel. NIEMALS ins Repository, sicher verwahren:")
	fmt.Println()
	fmt.Println("  " + base64.RawURLEncoding.EncodeToString(priv))
	fmt.Println()
	fmt.Println("Zum Ausstellen als NEXORA_SIGNIERSCHLUESSEL setzen.")
}

func ausstellen(inhaber, stufe, funktionen, ablauf string) {
	rohPriv := os.Getenv("NEXORA_SIGNIERSCHLUESSEL")
	if rohPriv == "" {
		fatal("NEXORA_SIGNIERSCHLUESSEL ist nicht gesetzt")
	}
	priv, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(rohPriv))
	if err != nil || len(priv) != ed25519.PrivateKeySize {
		fatal("NEXORA_SIGNIERSCHLUESSEL ist kein gültiger privater Ed25519-Schlüssel")
	}

	var zusatz []kern.Funktion
	for _, teil := range strings.Split(funktionen, ",") {
		teil = strings.TrimSpace(teil)
		if teil == "" {
			continue
		}
		if !bekannt(teil) {
			fatal("unbekannte Funktion %q. Bekannt sind: %s", teil, namen(kern.Alle))
		}
		zusatz = append(zusatz, kern.Funktion(teil))
	}

	var bis time.Time
	if ablauf != "" {
		bis, err = time.Parse("2006-01-02", ablauf)
		if err != nil {
			fatal("Ablaufdatum %q ist nicht JJJJ-MM-TT", ablauf)
		}
	}

	schluessel, err := plizenz.Ausstellen(ed25519.PrivateKey(priv), inhaber,
		kern.Stufe(stufe), zusatz, bis)
	if err != nil {
		fatal("%v", err)
	}

	fmt.Printf("Lizenz für %s", inhaber)
	if stufe != "" {
		fmt.Printf(", Stufe %s", stufe)
	}
	if len(zusatz) > 0 {
		fmt.Printf(" plus %s", namen(zusatz))
	}
	fmt.Println()
	fmt.Printf("Enthalten: %s\n\n", namen(append(kern.FunktionenDerStufe(kern.Stufe(stufe)), zusatz...)))
	fmt.Println(schluessel)
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
