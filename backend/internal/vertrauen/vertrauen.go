// Wem dieser Dienst beim Hinausgehen glaubt.
//
// Nexora spricht im Verbund mit Datenbank, Ablage und Zwischenspeicher, und
// diese Verbindungen sind verschlüsselt. Verschlüsselt allein ist aber wenig
// wert: wer nicht prüft, mit wem er spricht, redet unter Umständen
// verschlüsselt mit dem Falschen. Geprüft wird gegen die kleine eigene Stelle
// des Verbunds, siehe pki/erzeuge.sh.
//
// Die Stelle kommt zu den öffentlichen HINZU und ersetzt sie nicht. Sonst
// verlöre der Dienst das Vertrauen zu jedem Anmeldedienst im Netz -- ein
// Keycloak hinter einem Zertifikat von Let's Encrypt wäre plötzlich
// unerreichbar, und niemand verstünde, warum das Einrichten einer eigenen
// Zertifizierungsstelle die Anmeldung kaputtmacht.
package vertrauen

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Wurzeln liest eine zusätzliche Stelle und hängt sie an die des Systems.
//
// Ein leerer Pfad ergibt nil, und nil heißt für jeden Aufrufer: nimm, was das
// System kennt. Genau so verhält sich die Go-Bibliothek bei einem nicht
// gesetzten RootCAs, sodass der Fall ohne eigene Stelle keinen Sonderweg
// braucht.
func Wurzeln(pfad string) (*x509.CertPool, error) {
	pfad = strings.TrimSpace(pfad)
	if pfad == "" {
		return nil, nil
	}
	roh, err := os.ReadFile(pfad)
	if err != nil {
		return nil, fmt.Errorf("Zertifizierungsstelle %s: %w", pfad, err)
	}
	// Das System als Grundstock, und wenn es keinen hergibt (ein Abbild ohne
	// ca-certificates etwa), fangen wir eben leer an: die eigene Stelle ist der
	// Grund, warum diese Funktion aufgerufen wurde.
	vorrat, err := x509.SystemCertPool()
	if err != nil || vorrat == nil {
		vorrat = x509.NewCertPool()
	}
	if !vorrat.AppendCertsFromPEM(roh) {
		return nil, errors.New("in " + pfad + " steht kein lesbares Zertifikat")
	}
	return vorrat, nil
}
