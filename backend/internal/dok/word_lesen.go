// Reading a Word file and turning it into editor blocks.
//
// The counterpart to docx.go. Together they close a circle: open a .docx, edit
// it in the editor, write it back as a .docx.
//
// What survives that trip and what does not is the most important statement in
// this file. Carried over are headings, paragraphs, bullet and numbered lists
// including their nesting, tables, links, images and the bold, italic,
// underline and strikethrough marks. Text inside tracked insertions, content
// controls and smart tags comes along as well, deleted text does not.
//
// Not carried over: headers and footers, footnotes, columns, borders,
// typefaces, colours, comments. That is not negligence but the boundary of the
// undertaking: the editor knows ten kinds of block, Word knows hundreds.
// Whoever edits a Word file here and writes it back gets a clean document
// containing its content, not the same file with one line changed. Which is why
// the interface says so beforehand.
package dok

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// The parts of a .docx this reader looks at. Everything else in the archive is
// layout, and layout is what this trip does not carry.
const (
	wordHauptteil      = "word/document.xml"
	wordBeziehungen    = "word/_rels/document.xml.rels"
	wordNummerierung   = "word/numbering.xml"
	wordFormatvorlagen = "word/styles.xml"
	wordMedien         = "word/media/"
)

// maxBildBytes caps what is embedded as an image. A document full of photographs
// would otherwise turn into a JSON answer of a hundred megabytes, and the
// browser has to hold all of it at once. Beyond the cap the picture becomes a
// named line, which at least says that something stood there.
const maxBildBytes = 6 << 20

// wordPaket is what was pulled out of the archive: the text, the addresses the
// links point at, the list definitions and the pictures.
type wordPaket struct {
	Haupt        []byte
	Beziehungen  map[string]wordZiel
	Nummerierung map[string]map[int]string
	Vorlagen     map[string]wordVorlage
	Medien       map[string][]byte
	BilderBytes  int
}

// wordVorlage is one entry from styles.xml.
//
// It is needed because a paragraph often says nothing about itself beyond the
// name of its style. Word usually writes the list into the paragraph as well,
// but other writers put it only into the style, and then a numbered list
// arrives as a row of ordinary lines.
type wordVorlage struct {
	Name  string
	Basis string
	NumID string
	Ebene int
}

// wordZiel is one entry from the relationship file: where a link or a picture
// points to, and whether that is outside the document.
type wordZiel struct {
	Ziel   string
	Extern bool
}

// AusWord reads a .docx and returns it as a Dokument.
func AusWord(roh []byte) (Dokument, error) {
	leser, err := zip.NewReader(bytes.NewReader(roh), int64(len(roh)))
	if err != nil {
		return Dokument{}, errors.New("keine lesbare Word-Datei")
	}
	paket := wordPaket{
		Beziehungen:  map[string]wordZiel{},
		Nummerierung: map[string]map[int]string{},
		Vorlagen:     map[string]wordVorlage{},
		Medien:       map[string][]byte{},
	}
	for _, f := range leser.File {
		switch {
		case f.Name == wordHauptteil:
			// Bounded: a .docx with a gigantic uncompressed document.xml would
			// otherwise be a way to fill memory.
			paket.Haupt = teilLesen(f, 64<<20)
		case f.Name == wordBeziehungen:
			paket.Beziehungen = wordBeziehungenLesen(teilLesen(f, 4<<20))
		case f.Name == wordNummerierung:
			paket.Nummerierung = wordNummernLesen(teilLesen(f, 8<<20))
		case f.Name == wordFormatvorlagen:
			paket.Vorlagen = wordVorlagenLesen(teilLesen(f, 8<<20))
		case strings.HasPrefix(f.Name, wordMedien):
			// Only as much as may be embedded anyway. Whatever comes after the
			// cap is not read at all, so a file full of photographs costs
			// nothing beyond the first few.
			if paket.BilderBytes >= maxBildBytes {
				continue
			}
			daten := teilLesen(f, int64(maxBildBytes-paket.BilderBytes))
			paket.BilderBytes += len(daten)
			paket.Medien[f.Name] = daten
		}
	}
	if len(paket.Haupt) == 0 {
		return Dokument{}, errors.New("kein Hauptteil in der Datei. Ist es wirklich eine .docx?")
	}
	return wordZuDokument(paket)
}

func teilLesen(f *zip.File, grenze int64) []byte {
	auf, err := f.Open()
	if err != nil {
		return nil
	}
	defer auf.Close()
	daten, err := io.ReadAll(io.LimitReader(auf, grenze))
	if err != nil {
		return nil
	}
	return daten
}

// wordBeziehungenLesen reads the relationship file. A link in the text carries
// nothing but an id; the address behind it stands here.
func wordBeziehungenLesen(roh []byte) map[string]wordZiel {
	raus := map[string]wordZiel{}
	if len(roh) == 0 {
		return raus
	}
	var rels struct {
		Eintraege []struct {
			ID    string `xml:"Id,attr"`
			Ziel  string `xml:"Target,attr"`
			Modus string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if xml.Unmarshal(roh, &rels) != nil {
		return raus
	}
	for _, e := range rels.Eintraege {
		raus[e.ID] = wordZiel{Ziel: e.Ziel, Extern: strings.EqualFold(e.Modus, "External")}
	}
	return raus
}

// wordNummernLesen answers one question per list and level: bullet or number.
//
// Word keeps that two steps away from the paragraph. The paragraph names a
// numId, the numId points at an abstract list, and only there does each level
// say what it looks like. Without this file every numbered list would come out
// as bullets, and an instruction in five steps would lose the very thing that
// makes it an instruction.
func wordNummernLesen(roh []byte) map[string]map[int]string {
	raus := map[string]map[int]string{}
	if len(roh) == 0 {
		return raus
	}
	var n struct {
		Abstrakt []struct {
			ID     string `xml:"abstractNumId,attr"`
			Ebenen []struct {
				Ebene  string `xml:"ilvl,attr"`
				Format wWert  `xml:"numFmt"`
			} `xml:"lvl"`
		} `xml:"abstractNum"`
		Nummern []struct {
			ID       string `xml:"numId,attr"`
			Abstrakt wWert  `xml:"abstractNumId"`
		} `xml:"num"`
	}
	if xml.Unmarshal(roh, &n) != nil {
		return raus
	}
	formate := map[string]map[int]string{}
	for _, a := range n.Abstrakt {
		ebenen := map[int]string{}
		for _, e := range a.Ebenen {
			stufe, _ := strconv.Atoi(e.Ebene)
			ebenen[stufe] = e.Format.Wert
		}
		formate[a.ID] = ebenen
	}
	for _, num := range n.Nummern {
		if e, ok := formate[num.Abstrakt.Wert]; ok {
			raus[num.ID] = e
		}
	}
	return raus
}

// wordVorlagenLesen reads styles.xml.
func wordVorlagenLesen(roh []byte) map[string]wordVorlage {
	raus := map[string]wordVorlage{}
	if len(roh) == 0 {
		return raus
	}
	var v struct {
		Vorlagen []struct {
			ID    string `xml:"styleId,attr"`
			Name  wWert  `xml:"name"`
			Basis wWert  `xml:"basedOn"`
			PPr   struct {
				Nummern wNumPr `xml:"numPr"`
			} `xml:"pPr"`
		} `xml:"style"`
	}
	if xml.Unmarshal(roh, &v) != nil {
		return raus
	}
	for _, e := range v.Vorlagen {
		ebene, _ := strconv.Atoi(e.PPr.Nummern.Ebene.Wert)
		raus[e.ID] = wordVorlage{
			Name:  e.Name.Wert,
			Basis: e.Basis.Wert,
			NumID: e.PPr.Nummern.ID.Wert,
			Ebene: ebene,
		}
	}
	return raus
}

// vorlagenListe follows a style up its chain of ancestors until one of them
// names a list. Styles inherit, and "List Number 2" mostly says nothing itself
// beyond which style it is based on.
func (p wordPaket) vorlagenListe(id string) (string, int, bool) {
	for i := 0; i < 10 && id != ""; i++ {
		v, ok := p.Vorlagen[id]
		if !ok {
			return "", 0, false
		}
		if v.NumID != "" {
			return v.NumID, v.Ebene, true
		}
		id = v.Basis
	}
	return "", 0, false
}

// vorlagenName returns the readable name of a style, "List Number" for the id
// "ListNumber". Whoever wrote the file decides which of the two says something,
// so both are looked at.
func (p wordPaket) vorlagenName(id string) string {
	if v, ok := p.Vorlagen[id]; ok && v.Name != "" {
		return v.Name
	}
	return id
}

// The shape of document.xml, as far as it matters here.
type wDokument struct {
	Body wBody `xml:"body"`
}

type wBody struct {
	// Order matters: paragraphs and tables alternate. Hence one shared field
	// instead of two lists, which would put every table at the end.
	Inhalt []wInhalt `xml:",any"`
}

type wInhalt struct {
	XMLName xml.Name
	// Absatz
	Eigenschaften wAbsatzEigenschaften `xml:"pPr"`
	// Everything the paragraph carries, in order. Not just <w:r>: a link, a
	// tracked insertion, a content control and a smart tag all wrap their runs
	// in another element, and reading only the direct runs made exactly that
	// text disappear. A link in the middle of a sentence left a hole.
	Kinder []wKind `xml:",any"`
	// Tabelle
	Zeilen []wZeile `xml:"tr"`
}

type wAbsatzEigenschaften struct {
	Stil    wWert  `xml:"pStyle"`
	Nummern wNumPr `xml:"numPr"`
}

type wNumPr struct {
	Ebene wWert `xml:"ilvl"`
	ID    wWert `xml:"numId"`
}

type wWert struct {
	Wert string `xml:"val,attr"`
}

// wKind is one element inside a paragraph. It is deliberately one type for all
// of them: a run carries text, everything else carries runs, and both cases are
// answered by the same walk.
type wKind struct {
	XMLName       xml.Name
	RelID         string             `xml:"id,attr"`
	Anker         string             `xml:"anchor,attr"`
	Eigenschaften wLaufEigenschaften `xml:"rPr"`
	Zeichnungen   []wZeichnung       `xml:"drawing"`
	Kinder        []wKind            `xml:",any"`
	// The run as it stands, because inside it the order matters: a manual break
	// between two pieces of text belongs between them. Read through the fields
	// above, every break ended up behind the whole text, and an address block
	// became one long line.
	Inneres []byte `xml:",innerxml"`
}

// laufStuecke walks one run in document order.
func laufStuecke(k wKind, verweis string) []Stueck {
	stil := k.Eigenschaften
	mit := func(text string) Stueck {
		return Stueck{
			Text:   text,
			Fett:   stil.Fett != nil,
			Kursiv: stil.Kursiv != nil,
			Durch:  stil.Durch != nil,
			// <w:u w:val="none"/> means explicitly NOT underlined.
			Unter:   stil.Unter != nil && stil.Unter.Wert != "none",
			Verweis: verweis,
		}
	}
	var raus []Stueck
	d := xml.NewDecoder(bytes.NewReader(k.Inneres))
	for {
		t, err := d.Token()
		if err != nil {
			break
		}
		start, ok := t.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "t":
			var text string
			if d.DecodeElement(&text, &start) == nil && text != "" {
				raus = append(raus, mit(text))
			}
		case "tab":
			raus = append(raus, mit(" "))
		case "br":
			// A marker, not text: the caller breaks the paragraph here.
			raus = append(raus, Stueck{Text: "\n"})
		}
	}
	return raus
}

// wZeichnung is a picture in the running text.
//
// Kept as raw XML rather than as a struct: between <w:drawing> and the id of
// the picture lie half a dozen elements from three namespaces, and their order
// differs between an inline picture and a floating one. What is needed is one
// attribute and two labels, so the piece is walked through once instead of
// being modelled.
type wZeichnung struct {
	Inneres []byte `xml:",innerxml"`
}

// bildAngaben pulls the id of the picture and its description out of a drawing.
func (z wZeichnung) bildAngaben() (embed, name, beschreibung string) {
	d := xml.NewDecoder(bytes.NewReader(z.Inneres))
	for {
		t, err := d.Token()
		if err != nil {
			break
		}
		start, ok := t.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "blip":
			for _, a := range start.Attr {
				if a.Name.Local == "embed" && embed == "" {
					embed = a.Value
				}
			}
		case "docPr":
			for _, a := range start.Attr {
				switch a.Name.Local {
				case "name":
					if name == "" {
						name = a.Value
					}
				case "descr":
					if beschreibung == "" {
						beschreibung = a.Value
					}
				}
			}
		}
	}
	return embed, name, beschreibung
}

type wLaufEigenschaften struct {
	Fett   *struct{} `xml:"b"`
	Kursiv *struct{} `xml:"i"`
	Unter  *wWert    `xml:"u"`
	Durch  *struct{} `xml:"strike"`
}

type wZeile struct {
	Zellen []wZelle `xml:"tc"`
}

type wZelle struct {
	Absaetze []wInhalt `xml:"p"`
}

func wordZuDokument(paket wordPaket) (Dokument, error) {
	var d wDokument
	if err := xml.Unmarshal(paket.Haupt, &d); err != nil {
		return Dokument{}, errors.New("Word-Datei ist beschädigt")
	}

	var raus Dokument
	// One counter per nesting level. A sub list starts at one again, and when
	// the text returns to the level above, that one carries on where it left
	// off, which is what a reader expects from a numbered list.
	zaehler := map[int]int{}
	for _, teil := range d.Body.Inhalt {
		switch teil.XMLName.Local {
		case "p":
			for _, a := range wordAbsaetze(teil, paket) {
				if a.Art == ArtNummer {
					zaehler[a.Stufe]++
					for tiefer := range zaehler {
						if tiefer > a.Stufe {
							delete(zaehler, tiefer)
						}
					}
					a.Nummer = zaehler[a.Stufe]
				} else if a.Art != ArtAufzaehlung {
					zaehler = map[int]int{}
				}
				// Collapse consecutive empty paragraphs: Word likes to use them
				// as spacers, and turning each into a blank line in the editor
				// bloats the document.
				if a.Art == ArtAbsatz && len(a.Text) == 0 {
					if len(raus.Absatz) > 0 &&
						raus.Absatz[len(raus.Absatz)-1].Art == ArtAbsatz &&
						len(raus.Absatz[len(raus.Absatz)-1].Text) == 0 {
						continue
					}
				}
				raus.Absatz = append(raus.Absatz, a)
			}
		case "tbl":
			t := wordTabelle(teil, paket)
			if len(t) > 0 {
				raus.Absatz = append(raus.Absatz, Absatz{Art: ArtTabelle, Tabelle: t})
			}
			zaehler = map[int]int{}
		}
	}

	// The title is the first level 1 heading; otherwise it stays empty and the
	// caller falls back to the file name.
	for i, a := range raus.Absatz {
		if a.Art == ArtUeberschrift && a.Stufe == 1 {
			raus.Titel = nurText(a.Text)
			raus.Absatz = append(raus.Absatz[:i], raus.Absatz[i+1:]...)
			break
		}
	}
	return raus, nil
}

// wordAbsaetze turns one <w:p> into one or more paragraphs.
//
// More than one for two reasons: a manual line break inside the paragraph
// becomes a paragraph of its own, because the editor has no line break within a
// block and a swallowed one runs two sentences together. And a picture becomes
// its own block, because that is the only shape in which the editor can show
// one.
func wordAbsaetze(p wInhalt, paket wordPaket) []Absatz {
	grund := Absatz{Art: ArtAbsatz}
	stilID := p.Eigenschaften.Stil.Wert
	stil := strings.ToLower(strings.ReplaceAll(paket.vorlagenName(stilID)+" "+stilID, " ", ""))

	switch {
	case strings.HasPrefix(stil, "heading"), strings.HasPrefix(stil, "berschrift"),
		strings.HasPrefix(stil, "überschrift"):
		grund.Art = ArtUeberschrift
		grund.Stufe = 1
		// The digit at the end of the style name is the level: "heading2".
		for _, z := range stil {
			if z >= '1' && z <= '6' {
				grund.Stufe = int(z - '0')
			}
		}
	case strings.HasPrefix(stil, "title"), strings.HasPrefix(stil, "titel"):
		// The title of the document is a heading of the first level. Word has a
		// style of its own for it, and without this line the actual title of a
		// document arrived as an ordinary paragraph.
		grund.Art = ArtUeberschrift
		grund.Stufe = 1
	case strings.HasPrefix(stil, "listparagraph"), strings.HasPrefix(stil, "listenabsatz"):
		grund.Art = ArtAufzaehlung
	}

	// Which list the paragraph belongs to. It says so itself, or its style says
	// it: writers other than Word put the list into the style only, and reading
	// the paragraph alone turned every numbered list into plain lines.
	numID := p.Eigenschaften.Nummern.ID.Wert
	ebene, _ := strconv.Atoi(p.Eigenschaften.Nummern.Ebene.Wert)
	if numID == "" && grund.Art != ArtUeberschrift {
		if id, e, ok := paket.vorlagenListe(stilID); ok {
			numID, ebene = id, e
		}
	}
	if numID != "" && grund.Art != ArtUeberschrift {
		if ebene < 0 {
			ebene = 0
		}
		// A style like "List Number 2" carries its depth in the name; without it
		// every level would come out flat.
		if p.Eigenschaften.Nummern.ID.Wert == "" {
			for _, z := range stil {
				if z >= '2' && z <= '9' {
					ebene = int(z-'0') - 1
				}
			}
		}
		grund.Stufe = ebene
		grund.Art = ArtAufzaehlung
		// numbering.xml says what this level looks like. Anything that is not
		// explicitly a bullet counts as numbered: decimal, letters, roman
		// numerals, they all read as an order.
		format := paket.Nummerierung[numID][ebene]
		if format != "" && format != "bullet" && format != "none" {
			grund.Art = ArtNummer
		}
		if strings.Contains(stil, "number") || strings.Contains(stil, "nummer") {
			grund.Art = ArtNummer
		}
		if strings.Contains(stil, "bullet") || strings.Contains(stil, "aufzhlung") ||
			strings.Contains(stil, "aufzählung") {
			grund.Art = ArtAufzaehlung
		}
	}

	stuecke, bilder := wordStuecke(p.Kinder, paket, "")

	var raus []Absatz
	teil := grund
	teil.Text = nil
	schliessen := func() {
		if teil.Art == ArtUeberschrift && len(teil.Text) == 0 {
			teil.Art = ArtAbsatz
		}
		raus = append(raus, teil)
		naechster := grund
		naechster.Text = nil
		// Only the first piece of a broken up paragraph keeps the heading or the
		// list marker; the rest belongs to it and would otherwise become a
		// second item.
		if grund.Art != ArtAbsatz {
			naechster.Art = ArtAbsatz
			naechster.Stufe = 0
		}
		teil = naechster
	}
	for _, st := range stuecke {
		if st.Text == "\n" {
			schliessen()
			continue
		}
		teil.Text = append(teil.Text, st)
	}
	schliessen()

	// Pictures go below the text of their paragraph. In Word they sit in the
	// line; the editor knows an image only as a block of its own.
	raus = append(raus, bilder...)
	if len(raus) == 0 {
		raus = append(raus, grund)
	}
	return raus
}

// wordStuecke walks a paragraph and collects its text.
//
// verweis is the address inherited from an enclosing link; it is passed down
// rather than looked up again, because a link may well wrap several runs, one
// per change of formatting inside the linked words.
func wordStuecke(kinder []wKind, paket wordPaket, verweis string) ([]Stueck, []Absatz) {
	var raus []Stueck
	var bilder []Absatz
	for _, k := range kinder {
		switch k.XMLName.Local {
		case "del":
			// Text somebody deleted with tracked changes on. It is still in the
			// file, but it is not part of the document any more.
			continue

		case "hyperlink":
			ziel := verweis
			if z, ok := paket.Beziehungen[k.RelID]; ok && z.Ziel != "" {
				ziel = z.Ziel
			} else if k.Anker != "" {
				// A jump inside the document. It leads nowhere outside of Word,
				// so the text stays a link but points at the anchor.
				ziel = "#" + k.Anker
			}
			st, bi := wordStuecke(k.Kinder, paket, ziel)
			raus = append(raus, st...)
			bilder = append(bilder, bi...)

		case "r":
			for _, z := range k.Zeichnungen {
				if b, ok := wordBild(z, paket); ok {
					bilder = append(bilder, b)
				}
			}
			raus = append(raus, laufStuecke(k, verweis)...)

		default:
			// Tracked insertions, content controls, smart tags, bookmarks: all of
			// them are wrappers around runs. Whatever is not known is walked
			// through, because dropping it would drop text with it.
			st, bi := wordStuecke(k.Kinder, paket, verweis)
			raus = append(raus, st...)
			bilder = append(bilder, bi...)
		}
	}
	return raus, bilder
}

// wordBild turns a drawing into a block. The picture travels along inside the
// answer as a data address: it lies in the .docx and nowhere else, so there is
// no address that could point at it.
func wordBild(z wZeichnung, paket wordPaket) (Absatz, bool) {
	embed, bildName, beschreibung := z.bildAngaben()
	if embed == "" {
		return Absatz{}, false
	}
	ziel, ok := paket.Beziehungen[embed]
	if !ok || ziel.Extern {
		return Absatz{}, false
	}
	pfad := "word/" + strings.TrimPrefix(path.Clean(ziel.Ziel), "/")
	daten := paket.Medien[pfad]

	// The description first: it says what the picture shows, while the name is
	// mostly "Picture 3". The file name is the last resort.
	name := path.Base(pfad)
	if bildName != "" {
		name = bildName
	}
	if beschreibung != "" {
		name = beschreibung
	}

	a := Absatz{Art: ArtDatei, Text: []Stueck{{Text: name}}}
	if len(daten) == 0 {
		// Too large, or not in the archive. The line says that something stood
		// here, which is more honest than a gap in the text.
		return a, true
	}
	typ := http.DetectContentType(daten)
	if !strings.HasPrefix(typ, "image/") {
		return a, true
	}
	a.Bild = "data:" + typ + ";base64," + base64.StdEncoding.EncodeToString(daten)
	return a, true
}

func wordTabelle(t wInhalt, paket wordPaket) [][]string {
	var raus [][]string
	for _, z := range t.Zeilen {
		var zeile []string
		for _, zelle := range z.Zellen {
			var text []string
			for _, p := range zelle.Absaetze {
				st, _ := wordStuecke(p.Kinder, paket, "")
				if s := nurText(st); s != "" {
					text = append(text, s)
				}
			}
			zeile = append(zeile, strings.Join(text, " "))
		}
		if len(zeile) > 0 {
			raus = append(raus, zeile)
		}
	}
	return raus
}

func nurText(st []Stueck) string {
	var b strings.Builder
	for _, s := range st {
		if s.Text == "\n" {
			b.WriteString(" ")
			continue
		}
		b.WriteString(s.Text)
	}
	return strings.TrimSpace(b.String())
}

// NachBloecken translates a document into the blocks the editor understands.
//
// Only the types BlockNote knows: paragraph, heading (levels 1 to 3), bullet
// list, numbered list, table, image. An unknown one becomes a paragraph, because
// a block the editor does not know would keep the page from opening at all, and
// that would be the worse outcome.
func NachBloecken(d Dokument) []map[string]any {
	bloecke := []map[string]any{}
	// The stack holds the list item currently open at each nesting level. A
	// deeper item is hung into the one above it rather than being appended flat:
	// a sub list that lands beside its parent turns an ordered instruction into
	// a heap of equal lines.
	var stapel []map[string]any

	anhaengen := func(b map[string]any, stufe int, liste bool) {
		if !liste {
			stapel = nil
			bloecke = append(bloecke, b)
			return
		}
		if stufe > len(stapel) {
			stufe = len(stapel)
		}
		stapel = stapel[:stufe]
		if stufe == 0 {
			bloecke = append(bloecke, b)
		} else {
			eltern := stapel[stufe-1]
			kinder, _ := eltern["children"].([]map[string]any)
			eltern["children"] = append(kinder, b)
		}
		stapel = append(stapel, b)
	}

	for _, a := range d.Absatz {
		switch a.Art {
		case ArtUeberschrift:
			stufe := a.Stufe
			// BlockNote knows three levels. Deeper ones become the third rather
			// than being discarded: the structure suffers, the text remains.
			if stufe < 1 {
				stufe = 1
			}
			if stufe > 3 {
				stufe = 3
			}
			anhaengen(map[string]any{
				"type":    "heading",
				"props":   map[string]any{"level": stufe},
				"content": stueckeNachInhalt(a.Text),
			}, 0, false)
		case ArtAufzaehlung:
			anhaengen(map[string]any{
				"type":    "bulletListItem",
				"content": stueckeNachInhalt(a.Text),
			}, a.Stufe, true)
		case ArtNummer:
			anhaengen(map[string]any{
				"type":    "numberedListItem",
				"content": stueckeNachInhalt(a.Text),
			}, a.Stufe, true)
		case ArtTabelle:
			anhaengen(tabelleNachBlock(a.Tabelle), 0, false)
		case ArtDatei:
			if a.Bild != "" {
				anhaengen(map[string]any{
					"type": "image",
					"props": map[string]any{
						"url":     a.Bild,
						"name":    nurText(a.Text),
						"caption": "",
					},
				}, 0, false)
				continue
			}
			// No picture behind it: a line saying what stood there. An empty gap
			// would read as if the document simply went on.
			anhaengen(map[string]any{
				"type":    "paragraph",
				"content": stueckeNachInhalt([]Stueck{{Text: "Bild: " + nurText(a.Text), Kursiv: true}}),
			}, 0, false)
		default:
			anhaengen(map[string]any{
				"type":    "paragraph",
				"content": stueckeNachInhalt(a.Text),
			}, 0, false)
		}
	}
	if len(bloecke) == 0 {
		// An empty document: the editor insists on at least one block.
		bloecke = append(bloecke, map[string]any{"type": "paragraph", "content": []any{}})
	}
	return bloecke
}

func stueckeNachInhalt(st []Stueck) []any {
	inhalt := []any{}
	for _, s := range st {
		if s.Text == "" || s.Text == "\n" {
			continue
		}
		stile := map[string]any{}
		if s.Fett {
			stile["bold"] = true
		}
		if s.Kursiv {
			stile["italic"] = true
		}
		if s.Unter {
			stile["underline"] = true
		}
		if s.Durch {
			stile["strike"] = true
		}
		if s.Fest {
			stile["code"] = true
		}
		text := map[string]any{"type": "text", "text": s.Text, "styles": stile}
		if s.Verweis == "" {
			inhalt = append(inhalt, text)
			continue
		}
		// A link is its own kind of piece in the editor, with the text inside
		// it. Written as plain text it would still be readable but no longer
		// lead anywhere, and a link that leads nowhere is only half of it.
		inhalt = append(inhalt, map[string]any{
			"type":    "link",
			"href":    s.Verweis,
			"content": []any{text},
		})
	}
	return inhalt
}

func tabelleNachBlock(zeilen [][]string) map[string]any {
	// Make it rectangular: BlockNote wants the same number of cells in every
	// row, while Word allows merged and therefore missing ones.
	breite := 0
	for _, z := range zeilen {
		if len(z) > breite {
			breite = len(z)
		}
	}
	rows := []any{}
	for _, z := range zeilen {
		cells := []any{}
		for i := 0; i < breite; i++ {
			var text string
			if i < len(z) {
				text = z[i]
			}
			inhalt := []any{}
			if text != "" {
				inhalt = append(inhalt, map[string]any{
					"type": "text", "text": text, "styles": map[string]any{},
				})
			}
			cells = append(cells, inhalt)
		}
		rows = append(rows, map[string]any{"cells": cells})
	}
	return map[string]any{
		"type": "table",
		"content": map[string]any{
			"type": "tableContent",
			"rows": rows,
		},
	}
}
