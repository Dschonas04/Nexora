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

// anhangAdresse matches addresses under which attachments appear in a
// document. Two identifiers are present because permissions depend on the
// page, not the attachment.
var anhangAdresse = regexp.MustCompile(`^/api/pages/([0-9a-fA-F-]{36})/attachments/([0-9a-fA-F-]{36})$`)

// maxBildBytes is the per-image limit. An image larger than this is not
// embedded and remains a reference line: a document that takes a quarter of
// an hour to assemble is of no practical use.
const maxBildBytes = 25 << 20

// bildquelle returns the callback that `dok` uses to obtain image bytes.
//
// Read images are cached: the same file may occur multiple times in a
// storage (for example a logo) and fetching it separately for every page in
// an export of a hundred pages would be noticeable.
func (s *Server) bildquelle(ctx context.Context, uid string) dok.Bildquelle {
	gelesen := map[string][]byte{}
	darf := map[string]bool{}

	return func(adresse string) ([]byte, bool) {
		// A data URL already contains the image; `dok` will decode it. We just
		// pass it through here.
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

		// Only things that can be an image. A PDF or table inside the text
		// would not be embedded anyway, and reading it here would consume
		// memory twice.
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
		// One byte more than the limit is read so we can distinguish whether the
		// file exactly reaches the limit or is over it.
		daten, err := io.ReadAll(io.LimitReader(f, maxBildBytes+1))
		if err != nil || len(daten) > maxBildBytes {
			gelesen[anhang] = nil
			return nil, false
		}
		gelesen[anhang] = daten
		return daten, true
	}
}
