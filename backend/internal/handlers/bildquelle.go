// Where a rendered document obtains its images from.
//
// In the stored document an image is represented only by its address:
// /api/pages/<page>/attachments/<attachment>, or a data URL as found in an
// imported Word document. PDF and Word rendering need the raw bytes.
//
// The path to them goes through the same permission check as opening the
// page. Otherwise export would become the easiest route to foreign images:
// one could reference someone else's attachment address and export it.
package handlers

import (
	"context"
	"io"
	"regexp"
	"strings"

	"nexora/internal/dok"
)

// anhangAdresse trifft die Adressen, unter denen Anhaenge in einem Dokument
// stehen. Zwei Kennungen, weil die Rechte an der Seite haengen und nicht am
// Anhang.
var anhangAdresse = regexp.MustCompile(`^/api/pages/([0-9a-fA-F-]{36})/attachments/([0-9a-fA-F-]{36})$`)

// maxBildBytes ist die Grenze je Bild. Ein Bild, das groesser ist, wird nicht
// eingebettet, sondern bleibt die Verweiszeile: ein Dokument, das eine Viertel
// Stunde braucht, ist niemandem geholfen.
const maxBildBytes = 25 << 20

// bildquelle liefert den Rueckruf, mit dem dok an die Bilder kommt.
//
// Die gelesenen Bilder werden gemerkt: dieselbe Datei kommt in einer Ablage
// gerne mehrfach vor, etwa ein Logo, und sie fuer jede Seite neu aus der Ablage
// zu holen waere bei einem Export ueber hundert Seiten spuerbar.
func (s *Server) bildquelle(ctx context.Context, uid string) dok.Bildquelle {
	gelesen := map[string][]byte{}
	darf := map[string]bool{}

	return func(adresse string) ([]byte, bool) {
		// Eine Datenadresse traegt das Bild schon bei sich; dok packt sie selbst
		// aus. Hier wird sie nur durchgereicht.
		if strings.HasPrefix(adresse, "data:") {
			return []byte(adresse), true
		}
		treffer := anhangAdresse.FindStringSubmatch(adresse)
		if treffer == nil {
			return nil, false
		}
		seite, anhang := treffer[1], treffer[2]

		if daten, ok := gelesen[anhang]; ok {
			return daten, len(daten) > 0
		}

		erlaubt, gemerkt := darf[seite]
		if !gemerkt {
			canRead, _, _, ok := s.pagePerm(ctx, uid, seite)
			erlaubt = ok && canRead
			darf[seite] = erlaubt
		}
		if !erlaubt {
			gelesen[anhang] = nil
			return nil, false
		}

		// Nur, was auch ein Bild sein kann. Ein PDF oder eine Tabelle im Text
		// waere ohnehin nicht zu setzen, und es zu lesen kostete den Speicher
		// zweimal.
		var mime string
		if err := s.Pool.QueryRow(ctx,
			`SELECT mime FROM attachments WHERE id=$1 AND page_id=$2`, anhang, seite).Scan(&mime); err != nil ||
			!strings.HasPrefix(mime, "image/") {
			gelesen[anhang] = nil
			return nil, false
		}

		f, err := s.Ablage.Lesen(ctx, anhang)
		if err != nil {
			gelesen[anhang] = nil
			return nil, false
		}
		defer f.Close()
		// Ein Byte mehr als erlaubt wird mitgelesen: nur so laesst sich
		// unterscheiden, ob die Datei genau an die Grenze reicht oder darueber.
		daten, err := io.ReadAll(io.LimitReader(f, maxBildBytes+1))
		if err != nil || len(daten) > maxBildBytes {
			gelesen[anhang] = nil
			return nil, false
		}
		gelesen[anhang] = daten
		return daten, true
	}
}
