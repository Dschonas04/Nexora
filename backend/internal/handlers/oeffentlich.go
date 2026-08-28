// Was ein Besucher ohne Konto zu sehen bekommt.
//
// Eine oeffentlich geteilte Seite bestand bisher nur aus ihrem Text. Bilder
// darin zeigten auf /api/pages/<id>/attachments/<id>, und dieser Weg verlangt
// eine Sitzung: der Besucher bekam 401 und damit ein zerbrochenes Bild. Wer
// eine bebilderte Seite teilte, teilte ihre Loecher.
//
// Darum ein eigener Weg fuer die Dateien einer geteilten Seite, und im Text
// zeigen die Adressen darauf. Das nimmt zugleich die Kennungen aus der Antwort:
// bisher stand die Kennung der Seite in jeder Bildadresse.
package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// oeffentlicheDateien baut die Adresse, unter der die Dateien einer geteilten
// Seite liegen. Eine Funktion und keine Zeichenkette an zwei Stellen: der Weg
// steht im Router und hier, und beide muessen dasselbe meinen.
func oeffentlicheDateien(token string) string { return "/api/public/" + token + "/dateien/" }

// adressenOeffnen schreibt die Bild- und Dateiadressen einer Seite auf den
// oeffentlichen Weg um.
//
// Bewusst eine Ersetzung auf dem rohen JSON und kein Gang durch den Baum: die
// Adresse steht in props.url, aber ebenso in einer Bildunterschrift, in einem
// Verweis und in dem, was ein spaeterer Blocktyp mitbringt. Der gesuchte Text
// enthaelt die Kennung der Seite und ist damit eindeutig genug, dass er nichts
// anderes trifft.
//
// Nur die Dateien DIESER Seite: ein Block, der auf den Anhang einer anderen
// zeigt, bleibt stehen und bleibt verschlossen. Ihn mit freizugeben hiesse, mit
// einer Seite eine zweite zu teilen, von der niemand spricht.
func adressenOeffnen(inhalt json.RawMessage, seitenID, token string) json.RawMessage {
	alt := "/api/pages/" + seitenID + "/attachments/"
	if !strings.Contains(string(inhalt), alt) {
		return inhalt
	}
	return json.RawMessage(strings.ReplaceAll(string(inhalt), alt, oeffentlicheDateien(token)))
}

// OeffentlicheDatei liefert einen Anhang der geteilten Seite an jeden, der den
// Verweis hat.
//
// Die Prüfung ist dieselbe wie bei der Seite selbst: Zeichen und is_public, und
// der Anhang muss zu genau dieser Seite gehoeren. Wird der Verweis
// zurueckgezogen, sind die Bilder im selben Augenblick wieder zu.
func (s *Server) OeffentlicheDatei(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	attID := chi.URLParam(r, "attId")

	var filename, mime string
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT a.filename, a.mime
		   FROM attachments a
		   JOIN pages p ON p.id = a.page_id
		  WHERE a.id = $1 AND p.public_token = $2 AND p.is_public = true AND p.deleted_at IS NULL`,
		attID, token).Scan(&filename, &mime); err != nil {
		writeErr(w, http.StatusNotFound, "nicht gefunden")
		return
	}

	f, err := s.Ablage.Lesen(r.Context(), attID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Datei fehlt")
		return
	}
	defer f.Close()

	// Ueber anhangKopf und nicht von Hand: der Typ ist die Behauptung dessen,
	// der die Datei hochgeladen hat. Unveraendert und "inline" ausgeliefert
	// waere eine HTML- oder SVG-Datei ein Dokument auf dem Ursprung dieser
	// Instanz -- und hier reichte dafuer ein Fremder mit dem Verweis.
	anhangKopf(w, mime, filename)
	// Kein Suchmaschinenfutter: eine geteilte Seite ist nicht veroeffentlicht,
	// sie ist weitergegeben.
	w.Header().Set("X-Robots-Tag", "noindex")
	io.Copy(w, f)
}
