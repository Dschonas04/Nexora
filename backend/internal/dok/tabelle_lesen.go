// Reading spreadsheets (.xlsx, .ods) for the viewer.
//
// Only the values are read, as text the way a reader expects to see them:
// numbers as numbers, dates as dates, formulas as their last computed result.
// Formatting, charts and merged cells are not carried over; the viewer shows
// a plain table per sheet, which is what one wants for a quick look.
//
// Both formats are ZIP packages with XML inside, so no extra library is
// needed. Everything is bounded: a sheet with a million rows or a cell
// reference in column XFD must not fill memory.
package dok

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"path"
	"strconv"
	"strings"
	"time"
)

// Blatt is one sheet of a workbook.
type Blatt struct {
	Name   string     `json:"name"`
	Zeilen [][]string `json:"rows"`
	// Gekuerzt is set when rows or columns beyond the limits were left out.
	Gekuerzt bool `json:"truncated"`
}

const (
	maxBlaetter     = 30
	maxTabZeilen    = 5000
	maxTabSpalten   = 200
	maxTabTeilBytes = 64 << 20
)

// AusTabelle reads an .xlsx or .ods file. The format is recognised by the
// content of the package, not by the name.
func AusTabelle(roh []byte) ([]Blatt, error) {
	leser, err := zip.NewReader(bytes.NewReader(roh), int64(len(roh)))
	if err != nil {
		return nil, errors.New("keine lesbare Tabellendatei")
	}
	dateien := map[string]*zip.File{}
	for _, f := range leser.File {
		dateien[f.Name] = f
	}
	if f, ok := dateien["xl/workbook.xml"]; ok {
		return ausXlsx(dateien, f)
	}
	if f, ok := dateien["content.xml"]; ok {
		return ausOds(f)
	}
	return nil, errors.New("weder .xlsx noch .ods")
}

// ---- xlsx ----

type xlsxMappe struct {
	Blaetter []struct {
		Name string `xml:"name,attr"`
		RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
	} `xml:"sheets>sheet"`
	Datum1904 struct {
		Wert string `xml:"date1904,attr"`
	} `xml:"workbookPr"`
}

type xlsxBeziehungen struct {
	Liste []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

func ausXlsx(dateien map[string]*zip.File, mappeDatei *zip.File) ([]Blatt, error) {
	var mappe xlsxMappe
	if err := xml.Unmarshal(teilLesen(mappeDatei, 8<<20), &mappe); err != nil {
		return nil, errors.New("Arbeitsmappe nicht lesbar")
	}
	ziele := map[string]string{}
	if f, ok := dateien["xl/_rels/workbook.xml.rels"]; ok {
		var bz xlsxBeziehungen
		_ = xml.Unmarshal(teilLesen(f, 4<<20), &bz)
		for _, b := range bz.Liste {
			t := b.Target
			if strings.HasPrefix(t, "/") {
				t = strings.TrimPrefix(t, "/")
			} else {
				t = path.Join("xl", t)
			}
			ziele[b.ID] = t
		}
	}
	var texte []string
	if f, ok := dateien["xl/sharedStrings.xml"]; ok {
		texte = xlsxTexte(teilLesen(f, maxTabTeilBytes))
	}
	var datumsStile map[int]bool
	if f, ok := dateien["xl/styles.xml"]; ok {
		datumsStile = xlsxDatumsStile(teilLesen(f, 8<<20))
	}
	basis1904 := mappe.Datum1904.Wert == "1" || mappe.Datum1904.Wert == "true"

	var aus []Blatt
	for i, b := range mappe.Blaetter {
		if i >= maxBlaetter {
			break
		}
		ziel := ziele[b.RID]
		if ziel == "" {
			ziel = "xl/worksheets/sheet" + strconv.Itoa(i+1) + ".xml"
		}
		f, ok := dateien[ziel]
		if !ok {
			continue
		}
		blatt := xlsxBlatt(teilLesen(f, maxTabTeilBytes), texte, datumsStile, basis1904)
		blatt.Name = b.Name
		aus = append(aus, blatt)
	}
	if len(aus) == 0 {
		return nil, errors.New("keine Tabellenblätter gefunden")
	}
	return aus, nil
}

// xlsxTexte reads the shared string table. Rich text (several <r> runs) is
// joined; phonetic hints (<rPh>) are skipped.
func xlsxTexte(roh []byte) []string {
	var aus []string
	d := xml.NewDecoder(bytes.NewReader(roh))
	var akt strings.Builder
	inSi, inT, inPh := false, false, false
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				inSi = true
				akt.Reset()
			case "t":
				inT = inSi
			case "rPh":
				inPh = true
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "si":
				aus = append(aus, akt.String())
				inSi = false
			case "t":
				inT = false
			case "rPh":
				inPh = false
			}
		case xml.CharData:
			if inT && !inPh {
				akt.Write(t)
			}
		}
	}
	return aus
}

// Built-in number formats that display a date or time.
var xlsxDatumsFormate = map[int]bool{14: true, 15: true, 16: true, 17: true, 18: true, 19: true,
	20: true, 21: true, 22: true, 45: true, 46: true, 47: true}

// xlsxDatumsStile returns the cell style indices whose number format is a
// date. Custom formats count as dates when they contain a date or time letter
// outside quotes and brackets.
func xlsxDatumsStile(roh []byte) map[int]bool {
	var stile struct {
		Formate []struct {
			ID   int    `xml:"numFmtId,attr"`
			Code string `xml:"formatCode,attr"`
		} `xml:"numFmts>numFmt"`
		Xfs []struct {
			ID int `xml:"numFmtId,attr"`
		} `xml:"cellXfs>xf"`
	}
	if xml.Unmarshal(roh, &stile) != nil {
		return nil
	}
	eigene := map[int]bool{}
	for _, f := range stile.Formate {
		eigene[f.ID] = istDatumsCode(f.Code)
	}
	aus := map[int]bool{}
	for i, x := range stile.Xfs {
		if datum, ok := eigene[x.ID]; ok {
			aus[i] = datum
		} else {
			aus[i] = xlsxDatumsFormate[x.ID]
		}
	}
	return aus
}

func istDatumsCode(code string) bool {
	inZitat, inKlammer := false, false
	for _, r := range strings.ToLower(code) {
		switch {
		case r == '"':
			inZitat = !inZitat
		case inZitat:
		case r == '[':
			inKlammer = true
		case r == ']':
			inKlammer = false
		case inKlammer:
		case r == 'd' || r == 'm' || r == 'y' || r == 'h' || r == 's':
			return true
		}
	}
	return false
}

type xlsxZelle struct {
	Ref   string `xml:"r,attr"`
	Typ   string `xml:"t,attr"`
	Stil  int    `xml:"s,attr"`
	Wert  string `xml:"v"`
	Inner struct {
		Texte []string `xml:"t"`
		Laeufe []struct {
			T string `xml:"t"`
		} `xml:"r"`
	} `xml:"is"`
}

// xlsxBlatt streams a worksheet. Rows and cells carry their position, and
// gaps are filled so the columns line up.
func xlsxBlatt(roh []byte, texte []string, datumsStile map[int]bool, basis1904 bool) Blatt {
	var b Blatt
	d := xml.NewDecoder(bytes.NewReader(roh))
	zeile := -1
	naechsteZeile := 0
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "row":
			nr := naechsteZeile
			for _, a := range start.Attr {
				if a.Name.Local == "r" {
					if n, err := strconv.Atoi(a.Value); err == nil && n > 0 {
						nr = n - 1
					}
				}
			}
			if nr >= maxTabZeilen {
				b.Gekuerzt = true
				return trimmen(b)
			}
			for len(b.Zeilen) <= nr {
				b.Zeilen = append(b.Zeilen, nil)
			}
			zeile = nr
			naechsteZeile = nr + 1
		case "c":
			var z xlsxZelle
			if d.DecodeElement(&z, &start) != nil || zeile < 0 {
				continue
			}
			spalte := len(b.Zeilen[zeile])
			if z.Ref != "" {
				if s, ok := spalteAusRef(z.Ref); ok {
					spalte = s
				}
			}
			if spalte >= maxTabSpalten {
				b.Gekuerzt = true
				continue
			}
			wert := xlsxWert(z, texte, datumsStile, basis1904)
			for len(b.Zeilen[zeile]) <= spalte {
				b.Zeilen[zeile] = append(b.Zeilen[zeile], "")
			}
			b.Zeilen[zeile][spalte] = wert
		}
	}
	return trimmen(b)
}

func xlsxWert(z xlsxZelle, texte []string, datumsStile map[int]bool, basis1904 bool) string {
	switch z.Typ {
	case "s":
		if i, err := strconv.Atoi(strings.TrimSpace(z.Wert)); err == nil && i >= 0 && i < len(texte) {
			return texte[i]
		}
		return ""
	case "inlineStr":
		var s strings.Builder
		for _, t := range z.Inner.Texte {
			s.WriteString(t)
		}
		for _, l := range z.Inner.Laeufe {
			s.WriteString(l.T)
		}
		return s.String()
	case "b":
		if strings.TrimSpace(z.Wert) == "1" {
			return "WAHR"
		}
		return "FALSCH"
	case "str", "e", "d":
		return z.Wert
	}
	if z.Wert == "" {
		return ""
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(z.Wert), 64)
	if err != nil {
		return z.Wert
	}
	if datumsStile[z.Stil] {
		return excelDatum(f, basis1904)
	}
	return zahlText(f)
}

// excelDatum turns a serial day number into a readable date (and time, if
// the value has a fraction).
func excelDatum(f float64, basis1904 bool) string {
	basis := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	if basis1904 {
		basis = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	tage := math.Floor(f)
	sek := math.Round((f - tage) * 86400)
	t := basis.AddDate(0, 0, int(tage)).Add(time.Duration(sek) * time.Second)
	switch {
	case tage == 0 && sek > 0:
		return t.Format("15:04")
	case sek == 0:
		return t.Format("02.01.2006")
	default:
		return t.Format("02.01.2006 15:04")
	}
}

// zahlText prints a number without float noise (0.1+0.2) and with a German
// decimal comma.
func zahlText(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if strings.Contains(s, ".") && len(s) > 16 {
		s = strconv.FormatFloat(f, 'g', 15, 64)
	}
	return strings.Replace(s, ".", ",", 1)
}

// spalteAusRef turns "AB12" into the zero-based column 27.
func spalteAusRef(ref string) (int, bool) {
	n := 0
	i := 0
	for ; i < len(ref); i++ {
		c := ref[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c < 'A' || c > 'Z' {
			break
		}
		n = n*26 + int(c-'A'+1)
		if n > 1<<20 {
			return 0, false
		}
	}
	if i == 0 {
		return 0, false
	}
	return n - 1, true
}

// ---- ods ----

func ausOds(f *zip.File) ([]Blatt, error) {
	r, err := f.Open()
	if err != nil {
		return nil, errors.New("Inhalt der Tabelle nicht lesbar")
	}
	defer r.Close()
	d := xml.NewDecoder(io.LimitReader(r, maxTabTeilBytes))
	var aus []Blatt
	var b *Blatt
	var zeile []string
	zeilenWdh := 1
	var zelle strings.Builder
	zellenWdh := 1
	zellWert := ""
	inZelle := false
	absaetze := 0
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "table":
				if len(aus) >= maxBlaetter {
					return aus, nil
				}
				aus = append(aus, Blatt{Name: attr(t, "name")})
				b = &aus[len(aus)-1]
			case "table-row":
				zeile = nil
				zeilenWdh = wiederholung(attr(t, "number-rows-repeated"))
			case "table-cell", "covered-table-cell":
				inZelle = true
				zelle.Reset()
				absaetze = 0
				zellenWdh = wiederholung(attr(t, "number-columns-repeated"))
				zellWert = odsWert(t)
			case "p":
				if inZelle {
					if absaetze > 0 {
						zelle.WriteByte('\n')
					}
					absaetze++
				}
			case "s":
				if inZelle {
					n := wiederholung(attr(t, "c"))
					zelle.WriteString(strings.Repeat(" ", n))
				}
			case "tab":
				if inZelle {
					zelle.WriteByte('\t')
				}
			case "line-break":
				if inZelle {
					zelle.WriteByte('\n')
				}
			}
		case xml.CharData:
			if inZelle {
				zelle.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "table-cell", "covered-table-cell":
				inZelle = false
				wert := zelle.String()
				if zellWert != "" {
					wert = zellWert
				}
				// A trailing run of empty cells repeated 16000 times only pads
				// the row to the sheet's width; it is cut off below.
				for k := 0; k < zellenWdh && len(zeile) < maxTabSpalten; k++ {
					zeile = append(zeile, wert)
				}
				if len(zeile) >= maxTabSpalten && b != nil && wert != "" {
					b.Gekuerzt = true
				}
			case "table-row":
				if b == nil {
					continue
				}
				zeile = rechtsKuerzen(zeile)
				for k := 0; k < zeilenWdh; k++ {
					if len(b.Zeilen) >= maxTabZeilen {
						if len(zeile) > 0 {
							b.Gekuerzt = true
						}
						break
					}
					// Empty rows repeated a million times are padding as well.
					if len(zeile) == 0 && k > 0 {
						break
					}
					b.Zeilen = append(b.Zeilen, zeile)
				}
			case "table":
				if b != nil {
					*b = trimmen(*b)
				}
				b = nil
			}
		}
	}
	if len(aus) == 0 {
		return nil, errors.New("keine Tabellenblätter gefunden")
	}
	return aus, nil
}

func attr(t xml.StartElement, name string) string {
	for _, a := range t.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func wiederholung(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1
	}
	if n > 1<<20 {
		return 1 << 20
	}
	return n
}

// odsWert takes the typed value for dates; numbers keep the displayed text,
// which ODS already stores formatted the way the author set it.
func odsWert(t xml.StartElement) string {
	switch attr(t, "value-type") {
	case "date":
		v := attr(t, "date-value")
		for _, f := range []string{"2006-01-02T15:04:05", "2006-01-02"} {
			if d, err := time.Parse(f, v); err == nil {
				if f == "2006-01-02" || (d.Hour() == 0 && d.Minute() == 0) {
					return d.Format("02.01.2006")
				}
				return d.Format("02.01.2006 15:04")
			}
		}
		return v
	case "boolean":
		if attr(t, "boolean-value") == "true" {
			return "WAHR"
		}
		return "FALSCH"
	}
	return ""
}

func rechtsKuerzen(z []string) []string {
	for len(z) > 0 && z[len(z)-1] == "" {
		z = z[:len(z)-1]
	}
	return z
}

// trimmen drops empty trailing rows and empty trailing cells, and replaces
// nil rows with empty ones so the JSON is an array of arrays throughout.
func trimmen(b Blatt) Blatt {
	for i := range b.Zeilen {
		b.Zeilen[i] = rechtsKuerzen(b.Zeilen[i])
		if b.Zeilen[i] == nil {
			b.Zeilen[i] = []string{}
		}
	}
	for len(b.Zeilen) > 0 && len(b.Zeilen[len(b.Zeilen)-1]) == 0 {
		b.Zeilen = b.Zeilen[:len(b.Zeilen)-1]
	}
	if b.Zeilen == nil {
		b.Zeilen = [][]string{}
	}
	return b
}
