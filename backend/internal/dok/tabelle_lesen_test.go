package dok

import (
	"archive/zip"
	"bytes"
	"testing"
)

func paket(t *testing.T, teile map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, inhalt := range teile {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(inhalt))
	}
	z.Close()
	return buf.Bytes()
}

func TestXlsx(t *testing.T) {
	roh := paket(t, map[string]string{
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Kosten" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst><si><t>Posten</t></si><si><r><t>Bet</t></r><r><t>rag</t></r></si></sst>`,
		"xl/styles.xml":              `<styleSheet><cellXfs><xf numFmtId="0"/><xf numFmtId="14"/></cellXfs></styleSheet>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData>
<row r="1"><c r="A1" t="s"><v>0</v></c><c r="C1" t="s"><v>1</v></c></row>
<row r="3"><c r="A3" t="inlineStr"><is><t>Miete</t></is></c><c r="B3" s="1"><v>45658</v></c><c r="C3"><f>1+1</f><v>0.30000000000000004</v></c><c r="D3" t="b"><v>1</v></c></row>
</sheetData></worksheet>`,
	})
	b, err := AusTabelle(roh)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 1 || b[0].Name != "Kosten" {
		t.Fatalf("Blätter: %+v", b)
	}
	z := b[0].Zeilen
	if len(z) != 3 || z[0][0] != "Posten" || z[0][1] != "" || z[0][2] != "Betrag" || len(z[1]) != 0 {
		t.Fatalf("Zeilen: %q", z)
	}
	if z[2][0] != "Miete" || z[2][1] != "01.01.2025" || z[2][2] != "0,3" || z[2][3] != "WAHR" {
		t.Fatalf("Zeile 3: %q", z[2])
	}
}

func TestOds(t *testing.T) {
	roh := paket(t, map[string]string{
		"content.xml": `<office:document-content xmlns:office="o" xmlns:table="t" xmlns:text="x"><office:body><office:spreadsheet>
<table:table table:name="Blatt1">
<table:table-row><table:table-cell><text:p>A</text:p></table:table-cell><table:table-cell table:number-columns-repeated="2"><text:p>x</text:p></table:table-cell><table:table-cell table:number-columns-repeated="16000"/></table:table-row>
<table:table-row><table:table-cell office:value-type="date" office:date-value="2026-11-23"><text:p>23.11.26</text:p></table:table-cell></table:table-row>
<table:table-row table:number-rows-repeated="1048000"><table:table-cell table:number-columns-repeated="16000"/></table:table-row>
</table:table></office:spreadsheet></office:body></office:document-content>`,
	})
	b, err := AusTabelle(roh)
	if err != nil {
		t.Fatal(err)
	}
	z := b[0].Zeilen
	if b[0].Name != "Blatt1" || len(z) != 2 || len(z[0]) != 3 || z[0][2] != "x" || z[1][0] != "23.11.2026" || b[0].Gekuerzt {
		t.Fatalf("%+v", b[0])
	}
}

func TestSpalteAusRef(t *testing.T) {
	for ref, want := range map[string]int{"A1": 0, "Z9": 25, "AA1": 26, "AB12": 27} {
		if got, ok := spalteAusRef(ref); !ok || got != want {
			t.Errorf("%s: %d", ref, got)
		}
	}
}

// A long shared string referenced from every cell must not blow up the result.
func TestXlsxAusgabeBudget(t *testing.T) {
	var zellen bytes.Buffer
	for r := 1; r <= 2000; r++ {
		zellen.WriteString(`<row>`)
		for c := 0; c < 100; c++ {
			zellen.WriteString(`<c t="s"><v>0</v></c>`)
		}
		zellen.WriteString(`</row>`)
	}
	roh := paket(t, map[string]string{
		"xl/workbook.xml":          `<workbook><sheets><sheet name="A"/><sheet name="B"/></sheets></workbook>`,
		"xl/sharedStrings.xml":     `<sst><si><t>` + string(bytes.Repeat([]byte("x"), 100000)) + `</t></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData>` + zellen.String() + `</sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml": `<worksheet><sheetData>` + zellen.String() + `</sheetData></worksheet>`,
	})
	b, err := AusTabelle(roh)
	if err != nil {
		t.Fatal(err)
	}
	gesamt := 0
	for _, bl := range b {
		for _, z := range bl.Zeilen {
			for _, w := range z {
				if len(w) > maxTabZelle {
					t.Fatalf("Zelle %d Bytes", len(w))
				}
				gesamt += len(w)
			}
		}
	}
	if gesamt > maxTabAusgabe || !b[0].Gekuerzt {
		t.Fatalf("gesamt %d, gekürzt %v", gesamt, b[0].Gekuerzt)
	}
}

// Repeated ODS rows and cells and text:s count against the budget too.
func TestOdsAusgabeBudget(t *testing.T) {
	zelle := `<table:table-cell table:number-columns-repeated="200"><text:p>` + string(bytes.Repeat([]byte("y"), 20000)) + `<text:s text:c="1000000"/></text:p></table:table-cell>`
	roh := paket(t, map[string]string{
		"content.xml": `<office:document-content xmlns:office="o" xmlns:table="t" xmlns:text="x"><table:table table:name="A"><table:table-row table:number-rows-repeated="5000">` + zelle + `</table:table-row></table:table></office:document-content>`,
	})
	b, err := AusTabelle(roh)
	if err != nil {
		t.Fatal(err)
	}
	gesamt := 0
	for _, z := range b[0].Zeilen {
		for _, w := range z {
			if len(w) > maxTabZelle {
				t.Fatalf("Zelle %d Bytes", len(w))
			}
			gesamt += len(w)
		}
	}
	if gesamt > maxTabAusgabe || !b[0].Gekuerzt {
		t.Fatalf("gesamt %d, gekürzt %v", gesamt, b[0].Gekuerzt)
	}
}
