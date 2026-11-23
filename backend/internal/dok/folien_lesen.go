// Reading presentations (.pptx) for the viewer.
//
// A slide is returned as positioned boxes: text frames with their paragraphs
// and runs, pictures and tables, each with position and size in EMU (the unit
// of the format, 914400 per inch). The viewer lays them out on a sheet of the
// slide's proportions, so a slide looks roughly like it does in PowerPoint:
// right places, right sizes, right colours. Animations, shapes other than
// rectangles, gradients, charts and SmartArt are not drawn.
//
// Placeholders (title, body) usually carry no position of their own; it comes
// from the slide layout or, failing that, the slide master. Font sizes not set
// on the text come from the master's text styles. Theme colours are resolved
// against the theme of the master.
//
// Everything is bounded like the spreadsheet reader: number of slides, bytes
// unpacked, bytes of pictures embedded and bytes of text handed out.
package dok

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"path"
	"strconv"
	"strings"
)

// Praesentation is a whole .pptx.
type Praesentation struct {
	Breite int64   `json:"width"`
	Hoehe  int64   `json:"height"`
	Folien []Folie `json:"slides"`
	// Gekuerzt is set when slides, pictures or text beyond the limits were
	// left out.
	Gekuerzt bool `json:"truncated"`
}

// Folie is one slide.
type Folie struct {
	Hintergrund string    `json:"background,omitempty"`
	Elemente    []Element `json:"elements"`
}

// Element is one box on a slide.
type Element struct {
	Art string `json:"kind"` // "text", "bild", "tabelle"
	X   int64  `json:"x"`
	Y   int64  `json:"y"`
	B   int64  `json:"w"`
	H   int64  `json:"h"`
	// Fuellung and Linie are CSS colours, empty for none.
	Fuellung string `json:"fill,omitempty"`
	Linie    string `json:"line,omitempty"`
	// Anker is the vertical alignment of text: "t", "ctr" or "b".
	Anker    string         `json:"anchor,omitempty"`
	Absaetze []FolienAbsatz `json:"paragraphs,omitempty"`
	Bild     string         `json:"src,omitempty"`
	Zeilen   [][]string     `json:"rows,omitempty"`
	Drehung  float64        `json:"rotation,omitempty"`
}

// FolienAbsatz is a paragraph of a text box.
type FolienAbsatz struct {
	Ausrichtung string       `json:"align,omitempty"` // "l", "ctr", "r", "just"
	Ebene       int          `json:"level,omitempty"`
	Aufzaehlung string       `json:"bullet,omitempty"`
	Laeufe      []FolienLauf `json:"runs"`
	// Groesse is the size of an empty paragraph, in hundredths of a point,
	// so blank lines keep their height.
	Groesse int `json:"size,omitempty"`
}

// FolienLauf is a piece of text with one formatting.
type FolienLauf struct {
	Text     string `json:"text"`
	Groesse  int    `json:"size,omitempty"` // hundredths of a point
	Fett     bool   `json:"bold,omitempty"`
	Kursiv   bool   `json:"italic,omitempty"`
	Unter    bool   `json:"underline,omitempty"`
	Farbe    string `json:"color,omitempty"`
	Umbruch  bool   `json:"br,omitempty"`
	Schrift  string `json:"font,omitempty"`
	Verweis  string `json:"href,omitempty"`
	Hochtief int    `json:"baseline,omitempty"`
}

const (
	maxFolien        = 300
	maxFolienEingabe = 256 << 20
	maxFolienBilder  = 40 << 20
	maxFolienText    = 8 << 20
	maxFolienTeil    = 16 << 20
)

// ---- XML shapes ----

type aFarbe struct {
	Srgb *struct {
		Val string `xml:"val,attr"`
	} `xml:"srgbClr"`
	Schema *struct {
		Val string `xml:"val,attr"`
	} `xml:"schemeClr"`
	Sys *struct {
		Last string `xml:"lastClr,attr"`
	} `xml:"sysClr"`
	Preset *struct {
		Val string `xml:"val,attr"`
	} `xml:"prstClr"`
}

type aXfrm struct {
	Rot int64 `xml:"rot,attr"`
	Off struct {
		X int64 `xml:"x,attr"`
		Y int64 `xml:"y,attr"`
	} `xml:"off"`
	Ext struct {
		Cx int64 `xml:"cx,attr"`
		Cy int64 `xml:"cy,attr"`
	} `xml:"ext"`
	ChOff struct {
		X int64 `xml:"x,attr"`
		Y int64 `xml:"y,attr"`
	} `xml:"chOff"`
	ChExt struct {
		Cx int64 `xml:"cx,attr"`
		Cy int64 `xml:"cy,attr"`
	} `xml:"chExt"`
}

type pSpPr struct {
	Xfrm      *aXfrm    `xml:"xfrm"`
	SolidFill *aFarbe   `xml:"solidFill"`
	NoFill    *struct{} `xml:"noFill"`
	Ln        *struct {
		SolidFill *aFarbe   `xml:"solidFill"`
		NoFill    *struct{} `xml:"noFill"`
	} `xml:"ln"`
}

type pPlatz struct {
	Type string `xml:"type,attr"`
	Idx  string `xml:"idx,attr"`
}

type aRPr struct {
	Sz        int     `xml:"sz,attr"`
	B         string  `xml:"b,attr"`
	I         string  `xml:"i,attr"`
	U         string  `xml:"u,attr"`
	Baseline  int     `xml:"baseline,attr"`
	SolidFill *aFarbe `xml:"solidFill"`
	Latin     *struct {
		Typeface string `xml:"typeface,attr"`
	} `xml:"latin"`
	Link *struct {
		ID string `xml:"id,attr"`
	} `xml:"hlinkClick"`
}

type aPPr struct {
	Algn   string    `xml:"algn,attr"`
	Lvl    int       `xml:"lvl,attr"`
	BuNone *struct{} `xml:"buNone"`
	BuChar *struct {
		Char string `xml:"char,attr"`
	} `xml:"buChar"`
	BuAuto *struct {
		Type    string `xml:"type,attr"`
		StartAt int    `xml:"startAt,attr"`
	} `xml:"buAutoNum"`
	DefRPr *aRPr `xml:"defRPr"`
}

type aStueck struct {
	XMLName xml.Name
	RPr     *aRPr  `xml:"rPr"`
	T       string `xml:"t"`
}

type aP struct {
	PPr     *aPPr     `xml:"pPr"`
	Stuecke []aStueck `xml:",any"`
	EndeRPr *aRPr     `xml:"endParaRPr"`
}

type aTxBody struct {
	BodyPr struct {
		Anchor string `xml:"anchor,attr"`
	} `xml:"bodyPr"`
	LstStyle *aLstStyle `xml:"lstStyle"`
	Ps       []aP       `xml:"p"`
}

type aLstStyle struct {
	L1 *aPPr `xml:"lvl1pPr"`
	L2 *aPPr `xml:"lvl2pPr"`
	L3 *aPPr `xml:"lvl3pPr"`
	L4 *aPPr `xml:"lvl4pPr"`
	L5 *aPPr `xml:"lvl5pPr"`
}

func (l *aLstStyle) groesse(ebene int) int {
	if l == nil {
		return 0
	}
	p := [...]*aPPr{l.L1, l.L2, l.L3, l.L4, l.L5}[min(max(ebene, 0), 4)]
	if p != nil && p.DefRPr != nil {
		return p.DefRPr.Sz
	}
	return 0
}

func (l *aLstStyle) ausrichtung(ebene int) string {
	if l == nil {
		return ""
	}
	p := [...]*aPPr{l.L1, l.L2, l.L3, l.L4, l.L5}[min(max(ebene, 0), 4)]
	if p != nil {
		return p.Algn
	}
	return ""
}

type pSp struct {
	Nv struct {
		NvPr struct {
			Ph *pPlatz `xml:"ph"`
		} `xml:"nvPr"`
	} `xml:"nvSpPr"`
	SpPr   pSpPr    `xml:"spPr"`
	TxBody *aTxBody `xml:"txBody"`
}

type pPic struct {
	Nv struct {
		NvPr struct {
			Ph *pPlatz `xml:"ph"`
		} `xml:"nvPr"`
	} `xml:"nvPicPr"`
	BlipFill struct {
		Blip struct {
			// Without namespace on purpose: shapes are decoded from the inner
			// XML of the shape tree, where the r: prefix is no longer declared.
			Embed string `xml:"embed,attr"`
		} `xml:"blip"`
	} `xml:"blipFill"`
	SpPr pSpPr `xml:"spPr"`
}

type pRahmen struct {
	Nv struct {
		NvPr struct {
			Ph *pPlatz `xml:"ph"`
		} `xml:"nvPr"`
	} `xml:"nvGraphicFramePr"`
	Xfrm    *aXfrm `xml:"xfrm"`
	Graphic struct {
		Data struct {
			Tbl *struct {
				Tr []struct {
					Tc []struct {
						TxBody *aTxBody `xml:"txBody"`
					} `xml:"tc"`
				} `xml:"tr"`
			} `xml:"tbl"`
		} `xml:"graphicData"`
	} `xml:"graphic"`
}

type pGruppe struct {
	GrpSpPr pSpPr  `xml:"grpSpPr"`
	Inhalt  []byte `xml:",innerxml"`
}

type pHintergrund struct {
	BgPr *struct {
		SolidFill *aFarbe `xml:"solidFill"`
	} `xml:"bgPr"`
	BgRef *struct {
		aFarbe
	} `xml:"bgRef"`
}

type pFolienDatei struct {
	CSld struct {
		Bg     *pHintergrund `xml:"bg"`
		SpTree pGruppe       `xml:"spTree"`
	} `xml:"cSld"`
	ClrMap   *pFarbZuordnung `xml:"clrMap"`
	TxStyles *struct {
		Title *aLstStyle `xml:"titleStyle"`
		Body  *aLstStyle `xml:"bodyStyle"`
		Other *aLstStyle `xml:"otherStyle"`
	} `xml:"txStyles"`
}

type pFarbZuordnung struct {
	Bg1 string `xml:"bg1,attr"`
	Tx1 string `xml:"tx1,attr"`
	Bg2 string `xml:"bg2,attr"`
	Tx2 string `xml:"tx2,attr"`
}

// ---- reading ----

type folienPaket struct {
	dateien     map[string]*zip.File
	eingabe     int
	bilder      int
	text        int
	gekuerzt    bool
	bilderCache map[string]string
}

func (p *folienPaket) lesen(name string) []byte {
	f, ok := p.dateien[name]
	if !ok || p.eingabe <= 0 {
		return nil
	}
	roh := teilLesen(f, int64(min(maxFolienTeil, p.eingabe)))
	p.eingabe -= len(roh)
	return roh
}

// beziehungen reads the .rels file next to a part and returns id → target
// (resolved to a path inside the package; external targets as they are).
func (p *folienPaket) beziehungen(teil string) map[string]string {
	rels := path.Join(path.Dir(teil), "_rels", path.Base(teil)+".rels")
	var bz struct {
		Liste []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	_ = xml.Unmarshal(p.lesen(rels), &bz)
	aus := map[string]string{}
	for _, b := range bz.Liste {
		switch {
		case b.Mode == "External":
			aus[b.ID] = b.Target
		case strings.HasPrefix(b.Target, "/"):
			aus[b.ID] = strings.TrimPrefix(b.Target, "/")
		default:
			aus[b.ID] = path.Join(path.Dir(teil), b.Target)
		}
	}
	return aus
}

// vorlage is what a layout or master contributes: placeholder positions,
// sizes from their list styles, and the background.
type vorlage struct {
	plaetze     map[string]platzVorgabe
	hintergrund string
}

type platzVorgabe struct {
	xfrm  *aXfrm
	stil  *aLstStyle
	anker string
}

func platzSchluessel(ph *pPlatz) []string {
	typ := ph.Type
	switch typ {
	case "", "obj":
		typ = "body"
	case "ctrTitle":
		typ = "title"
	case "subTitle":
		typ = "body"
	}
	var aus []string
	if ph.Idx != "" {
		aus = append(aus, "idx:"+ph.Idx)
	}
	return append(aus, "type:"+typ)
}

// AusFolien reads a .pptx.
func AusFolien(roh []byte) (Praesentation, error) {
	leser, err := zip.NewReader(bytes.NewReader(roh), int64(len(roh)))
	if err != nil {
		return Praesentation{}, errors.New("keine lesbare PowerPoint-Datei")
	}
	p := &folienPaket{dateien: map[string]*zip.File{}, eingabe: maxFolienEingabe, bilderCache: map[string]string{}}
	for _, f := range leser.File {
		p.dateien[f.Name] = f
	}
	const praes = "ppt/presentation.xml"
	var kopf struct {
		Groesse struct {
			Cx int64 `xml:"cx,attr"`
			Cy int64 `xml:"cy,attr"`
		} `xml:"sldSz"`
		Folien []struct {
			RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sldIdLst>sldId"`
	}
	if err := xml.Unmarshal(p.lesen(praes), &kopf); err != nil {
		return Praesentation{}, errors.New("keine Präsentation in der Datei. Ist es wirklich eine .pptx?")
	}
	aus := Praesentation{Breite: kopf.Groesse.Cx, Hoehe: kopf.Groesse.Cy}
	if aus.Breite <= 0 || aus.Hoehe <= 0 {
		aus.Breite, aus.Hoehe = 12192000, 6858000
	}
	ziele := p.beziehungen(praes)

	// Layouts and masters are shared by many slides; read each once.
	layouts := map[string]*vorlageMitMaster{}
	masters := map[string]*masterInfo{}

	gelesen := map[string]bool{}
	for _, f := range kopf.Folien {
		if len(aus.Folien) >= maxFolien {
			aus.Gekuerzt = true
			break
		}
		teil := ziele[f.RID]
		if teil == "" || gelesen[teil] {
			continue
		}
		gelesen[teil] = true
		folie, ok := p.folie(teil, layouts, masters)
		if !ok {
			continue
		}
		aus.Folien = append(aus.Folien, folie)
	}
	if len(aus.Folien) == 0 {
		return Praesentation{}, errors.New("keine Folien gefunden")
	}
	aus.Gekuerzt = aus.Gekuerzt || p.gekuerzt
	return aus, nil
}

type masterInfo struct {
	vorlage
	farben map[string]string
	zuord  pFarbZuordnung
	titel  *aLstStyle
	text   *aLstStyle
	andere *aLstStyle
}

type vorlageMitMaster struct {
	vorlage
	master *masterInfo
}

func (p *folienPaket) master(teil string, masters map[string]*masterInfo) *masterInfo {
	if m, ok := masters[teil]; ok {
		return m
	}
	m := &masterInfo{farben: standardFarben()}
	masters[teil] = m
	var datei pFolienDatei
	if xml.Unmarshal(p.lesen(teil), &datei) != nil {
		return m
	}
	rels := p.beziehungen(teil)
	for _, z := range rels {
		if strings.Contains(z, "/theme/") {
			for k, v := range themaFarben(p.lesen(z)) {
				m.farben[k] = v
			}
			break
		}
	}
	if datei.ClrMap != nil {
		m.zuord = *datei.ClrMap
	}
	if datei.TxStyles != nil {
		m.titel, m.text, m.andere = datei.TxStyles.Title, datei.TxStyles.Body, datei.TxStyles.Other
	}
	m.vorlage = p.vorlageLesen(datei, m)
	return m
}

func (p *folienPaket) layout(teil string, layouts map[string]*vorlageMitMaster, masters map[string]*masterInfo) *vorlageMitMaster {
	if l, ok := layouts[teil]; ok {
		return l
	}
	l := &vorlageMitMaster{}
	layouts[teil] = l
	rels := p.beziehungen(teil)
	for _, z := range rels {
		if strings.Contains(z, "slideMaster") {
			l.master = p.master(z, masters)
			break
		}
	}
	if l.master == nil {
		l.master = &masterInfo{farben: standardFarben()}
	}
	var datei pFolienDatei
	if xml.Unmarshal(p.lesen(teil), &datei) == nil {
		l.vorlage = p.vorlageLesen(datei, l.master)
	}
	return l
}

// vorlageLesen collects placeholder positions from a layout or master.
func (p *folienPaket) vorlageLesen(datei pFolienDatei, m *masterInfo) vorlage {
	v := vorlage{plaetze: map[string]platzVorgabe{}}
	if datei.CSld.Bg != nil {
		v.hintergrund = hintergrundFarbe(datei.CSld.Bg, m)
	}
	d := xml.NewDecoder(bytes.NewReader(datei.CSld.SpTree.Inhalt))
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "sp" {
			_ = d.Skip()
			continue
		}
		var sp pSp
		if d.DecodeElement(&sp, &start) != nil || sp.Nv.NvPr.Ph == nil {
			continue
		}
		vg := platzVorgabe{xfrm: sp.SpPr.Xfrm}
		if sp.TxBody != nil {
			vg.stil = sp.TxBody.LstStyle
			vg.anker = sp.TxBody.BodyPr.Anchor
		}
		for _, k := range platzSchluessel(sp.Nv.NvPr.Ph) {
			if _, schon := v.plaetze[k]; !schon {
				v.plaetze[k] = vg
			}
		}
	}
	return v
}

// umrechnung maps coordinates of a group's children onto the slide:
// x_slide = ax*x + bx.
type umrechnung struct{ ax, bx, ay, by float64 }

func (u umrechnung) in(x aXfrm) (int64, int64, int64, int64) {
	return int64(u.ax*float64(x.Off.X) + u.bx), int64(u.ay*float64(x.Off.Y) + u.by),
		int64(u.ax * float64(x.Ext.Cx)), int64(u.ay * float64(x.Ext.Cy))
}

func (p *folienPaket) folie(teil string, layouts map[string]*vorlageMitMaster, masters map[string]*masterInfo) (Folie, bool) {
	roh := p.lesen(teil)
	var datei pFolienDatei
	if xml.Unmarshal(roh, &datei) != nil {
		return Folie{}, false
	}
	rels := p.beziehungen(teil)
	var l *vorlageMitMaster
	for _, z := range rels {
		if strings.Contains(z, "slideLayout") {
			l = p.layout(z, layouts, masters)
			break
		}
	}
	if l == nil {
		l = &vorlageMitMaster{master: &masterInfo{farben: standardFarben()}}
	}
	f := Folie{Elemente: []Element{}}
	switch {
	case datei.CSld.Bg != nil:
		f.Hintergrund = hintergrundFarbe(datei.CSld.Bg, l.master)
	case l.hintergrund != "":
		f.Hintergrund = l.hintergrund
	default:
		f.Hintergrund = l.master.hintergrund
	}
	if f.Hintergrund == "" {
		f.Hintergrund = l.master.farbe("bg1")
	}
	ctx := folienKontext{p: p, rels: rels, l: l}
	ctx.baum(datei.CSld.SpTree.Inhalt, umrechnung{ax: 1, ay: 1}, &f, 0)
	return f, true
}

type folienKontext struct {
	p    *folienPaket
	rels map[string]string
	l    *vorlageMitMaster
}

func (c folienKontext) vorgabe(ph *pPlatz) (platzVorgabe, platzVorgabe) {
	var ausLayout, ausMaster platzVorgabe
	if ph == nil {
		return ausLayout, ausMaster
	}
	for _, k := range platzSchluessel(ph) {
		if v, ok := c.l.plaetze[k]; ok && ausLayout.xfrm == nil {
			ausLayout = v
		}
	}
	for _, k := range platzSchluessel(ph) {
		if v, ok := c.l.master.plaetze[k]; ok && ausMaster.xfrm == nil {
			ausMaster = v
		}
	}
	return ausLayout, ausMaster
}

func (c folienKontext) baum(inhalt []byte, u umrechnung, f *Folie, tiefe int) {
	if tiefe > 8 {
		return
	}
	d := xml.NewDecoder(bytes.NewReader(inhalt))
	for {
		tok, err := d.Token()
		if err != nil {
			return
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "sp", "cxnSp":
			var sp pSp
			if d.DecodeElement(&sp, &start) == nil {
				c.form(sp, u, f)
			}
		case "pic":
			var pic pPic
			if d.DecodeElement(&pic, &start) == nil {
				c.bild(pic, u, f)
			}
		case "graphicFrame":
			var r pRahmen
			if d.DecodeElement(&r, &start) == nil {
				c.tabelle(r, u, f)
			}
		case "grpSp":
			var g pGruppe
			if d.DecodeElement(&g, &start) != nil {
				continue
			}
			x := g.GrpSpPr.Xfrm
			nu := u
			if x != nil && x.ChExt.Cx > 0 && x.ChExt.Cy > 0 {
				kx := float64(x.Ext.Cx) / float64(x.ChExt.Cx)
				ky := float64(x.Ext.Cy) / float64(x.ChExt.Cy)
				nu = umrechnung{
					ax: u.ax * kx, bx: u.ax*(float64(x.Off.X)-float64(x.ChOff.X)*kx) + u.bx,
					ay: u.ay * ky, by: u.ay*(float64(x.Off.Y)-float64(x.ChOff.Y)*ky) + u.by,
				}
			}
			c.baum(g.Inhalt, nu, f, tiefe+1)
		default:
			_ = d.Skip()
		}
	}
}

func (c folienKontext) form(sp pSp, u umrechnung, f *Folie) {
	ph := sp.Nv.NvPr.Ph
	ausLayout, ausMaster := c.vorgabe(ph)
	x := sp.SpPr.Xfrm
	if x == nil {
		x = ausLayout.xfrm
	}
	if x == nil {
		x = ausMaster.xfrm
	}
	if x == nil {
		return
	}
	m := c.l.master
	e := Element{Art: "text", Drehung: float64(x.Rot) / 60000}
	e.X, e.Y, e.B, e.H = u.in(*x)
	if sp.SpPr.SolidFill != nil {
		e.Fuellung = m.aufloesen(sp.SpPr.SolidFill)
	}
	if sp.SpPr.Ln != nil && sp.SpPr.Ln.SolidFill != nil {
		e.Linie = m.aufloesen(sp.SpPr.Ln.SolidFill)
	}
	if sp.TxBody != nil {
		e.Anker = sp.TxBody.BodyPr.Anchor
		if e.Anker == "" {
			e.Anker = ausLayout.anker
		}
		if e.Anker == "" {
			e.Anker = ausMaster.anker
		}
		// Where an unset size comes from, most specific first.
		var stile []*aLstStyle
		stile = append(stile, sp.TxBody.LstStyle, ausLayout.stil, ausMaster.stil)
		switch {
		case ph != nil && (ph.Type == "title" || ph.Type == "ctrTitle"):
			stile = append(stile, m.titel)
		case ph != nil:
			stile = append(stile, m.text)
		default:
			stile = append(stile, m.andere)
		}
		e.Absaetze = c.absaetze(sp.TxBody, stile, ph != nil && ph.Type != "title" && ph.Type != "ctrTitle" && ph.Type != "subTitle")
	}
	if len(e.Absaetze) == 0 && e.Fuellung == "" && e.Linie == "" {
		return
	}
	f.Elemente = append(f.Elemente, e)
}

func (c folienKontext) absaetze(tb *aTxBody, stile []*aLstStyle, aufzaehlungStandard bool) []FolienAbsatz {
	m := c.l.master
	var aus []FolienAbsatz
	nummer := map[int]int{}
	for _, p := range tb.Ps {
		a := FolienAbsatz{}
		if p.PPr != nil {
			a.Ausrichtung = p.PPr.Algn
			a.Ebene = p.PPr.Lvl
		}
		for _, s := range stile {
			if a.Ausrichtung != "" {
				break
			}
			a.Ausrichtung = s.ausrichtung(a.Ebene)
		}
		standard := 0
		for _, s := range stile {
			if g := s.groesse(a.Ebene); g > 0 {
				standard = g
				break
			}
		}
		for _, st := range p.Stuecke {
			switch st.XMLName.Local {
			case "r", "fld":
				l := FolienLauf{Text: st.T, Groesse: standard}
				if st.RPr != nil {
					if st.RPr.Sz > 0 {
						l.Groesse = st.RPr.Sz
					}
					l.Fett = st.RPr.B == "1" || st.RPr.B == "true"
					l.Kursiv = st.RPr.I == "1" || st.RPr.I == "true"
					l.Unter = st.RPr.U != "" && st.RPr.U != "none"
					l.Hochtief = st.RPr.Baseline
					if st.RPr.SolidFill != nil {
						l.Farbe = m.aufloesen(st.RPr.SolidFill)
					}
					if st.RPr.Latin != nil && !strings.HasPrefix(st.RPr.Latin.Typeface, "+") {
						l.Schrift = st.RPr.Latin.Typeface
					}
					if st.RPr.Link != nil {
						if z := c.rels[st.RPr.Link.ID]; strings.HasPrefix(z, "http://") || strings.HasPrefix(z, "https://") || strings.HasPrefix(z, "mailto:") {
							l.Verweis = z
						}
					}
				}
				if !c.textBuchen(len(l.Text)) {
					return aus
				}
				a.Laeufe = append(a.Laeufe, l)
			case "br":
				a.Laeufe = append(a.Laeufe, FolienLauf{Umbruch: true})
			}
		}
		if a.Laeufe == nil {
			a.Laeufe = []FolienLauf{}
			a.Groesse = standard
			if p.EndeRPr != nil && p.EndeRPr.Sz > 0 {
				a.Groesse = p.EndeRPr.Sz
			}
		}
		// Bullets: explicit on the paragraph, otherwise body placeholders
		// have them from the master (level 1 and below), others do not.
		hatText := len(a.Laeufe) > 0
		switch {
		case p.PPr != nil && p.PPr.BuNone != nil:
		case p.PPr != nil && p.PPr.BuChar != nil:
			a.Aufzaehlung = p.PPr.BuChar.Char
		case p.PPr != nil && p.PPr.BuAuto != nil:
			start := p.PPr.BuAuto.StartAt
			if start == 0 {
				start = 1
			}
			n := nummer[a.Ebene]
			if n == 0 {
				n = start
			}
			nummer[a.Ebene] = n + 1
			a.Aufzaehlung = nummerText(p.PPr.BuAuto.Type, n)
		case aufzaehlungStandard && hatText:
			a.Aufzaehlung = "•"
		}
		aus = append(aus, a)
	}
	// A box whose paragraphs are all empty draws nothing.
	for _, a := range aus {
		for _, l := range a.Laeufe {
			if strings.TrimSpace(l.Text) != "" {
				return aus
			}
		}
	}
	return nil
}

func nummerText(typ string, n int) string {
	switch {
	case strings.HasPrefix(typ, "alphaLc"):
		return string(rune('a'+(n-1)%26)) + endung(typ)
	case strings.HasPrefix(typ, "alphaUc"):
		return string(rune('A'+(n-1)%26)) + endung(typ)
	case strings.HasPrefix(typ, "romanLc"):
		return strings.ToLower(roemisch(n)) + endung(typ)
	case strings.HasPrefix(typ, "romanUc"):
		return roemisch(n) + endung(typ)
	}
	return strconv.Itoa(n) + endung(typ)
}

func endung(typ string) string {
	switch {
	case strings.HasSuffix(typ, "ParenR"):
		return ")"
	case strings.HasSuffix(typ, "ParenBoth"):
		return ")"
	case strings.HasSuffix(typ, "Plain"):
		return ""
	}
	return "."
}

func roemisch(n int) string {
	if n <= 0 || n > 3999 {
		return strconv.Itoa(n)
	}
	werte := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	zeichen := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var s strings.Builder
	for i, w := range werte {
		for n >= w {
			s.WriteString(zeichen[i])
			n -= w
		}
	}
	return s.String()
}

func (c folienKontext) textBuchen(n int) bool {
	if c.p.text+n > maxFolienText {
		c.p.gekuerzt = true
		return false
	}
	c.p.text += n
	return true
}

// Pictures the browser can show; EMF/WMF and TIFF are left out.
var folienBildTypen = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif",
	".webp": "image/webp", ".bmp": "image/bmp", ".svg": "image/svg+xml",
}

func (c folienKontext) bild(pic pPic, u umrechnung, f *Folie) {
	ausLayout, ausMaster := c.vorgabe(pic.Nv.NvPr.Ph)
	x := pic.SpPr.Xfrm
	if x == nil {
		x = ausLayout.xfrm
	}
	if x == nil {
		x = ausMaster.xfrm
	}
	if x == nil {
		return
	}
	ziel := c.rels[pic.BlipFill.Blip.Embed]
	mime := folienBildTypen[strings.ToLower(path.Ext(ziel))]
	if ziel == "" || mime == "" {
		return
	}
	src, ok := c.p.bilderCache[ziel]
	if !ok {
		datei, da := c.p.dateien[ziel]
		if !da {
			return
		}
		rest := maxFolienBilder - c.p.bilder
		if int64(rest) < int64(datei.UncompressedSize64) || rest <= 0 {
			c.p.gekuerzt = true
			return
		}
		roh := teilLesen(datei, int64(rest))
		c.p.bilder += len(roh)
		src = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(roh)
		c.p.bilderCache[ziel] = src
	}
	e := Element{Art: "bild", Bild: src, Drehung: float64(x.Rot) / 60000}
	e.X, e.Y, e.B, e.H = u.in(*x)
	f.Elemente = append(f.Elemente, e)
}

func (c folienKontext) tabelle(r pRahmen, u umrechnung, f *Folie) {
	if r.Xfrm == nil || r.Graphic.Data.Tbl == nil {
		return
	}
	e := Element{Art: "tabelle"}
	e.X, e.Y, e.B, e.H = u.in(*r.Xfrm)
	for _, tr := range r.Graphic.Data.Tbl.Tr {
		var zeile []string
		for _, tc := range tr.Tc {
			var s strings.Builder
			if tc.TxBody != nil {
				for i, p := range tc.TxBody.Ps {
					if i > 0 {
						s.WriteByte('\n')
					}
					for _, st := range p.Stuecke {
						s.WriteString(st.T)
					}
				}
			}
			t := kappen(s.String(), maxTabZelle)
			if !c.textBuchen(len(t)) {
				break
			}
			zeile = append(zeile, t)
		}
		e.Zeilen = append(e.Zeilen, zeile)
	}
	f.Elemente = append(f.Elemente, e)
}

// ---- colours ----

func standardFarben() map[string]string {
	return map[string]string{
		"dk1": "000000", "lt1": "FFFFFF", "dk2": "44546A", "lt2": "E7E6E6",
		"accent1": "4472C4", "accent2": "ED7D31", "accent3": "A5A5A5",
		"accent4": "FFC000", "accent5": "5B9BD5", "accent6": "70AD47",
		"hlink": "0563C1", "folHlink": "954F72",
	}
}

func themaFarben(roh []byte) map[string]string {
	aus := map[string]string{}
	d := xml.NewDecoder(bytes.NewReader(roh))
	in := false
	for {
		tok, err := d.Token()
		if err != nil {
			return aus
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "clrScheme" {
				in = true
				continue
			}
			if !in {
				continue
			}
			switch t.Name.Local {
			case "dk1", "lt1", "dk2", "lt2", "accent1", "accent2", "accent3", "accent4", "accent5", "accent6", "hlink", "folHlink":
				var fb aFarbe
				if d.DecodeElement(&fb, &t) == nil {
					if fb.Srgb != nil {
						aus[t.Name.Local] = fb.Srgb.Val
					} else if fb.Sys != nil && fb.Sys.Last != "" {
						aus[t.Name.Local] = fb.Sys.Last
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "clrScheme" {
				return aus
			}
		}
	}
}

func (m *masterInfo) farbe(name string) string {
	ziel := name
	switch name {
	case "bg1":
		ziel = oder(m.zuord.Bg1, "lt1")
	case "tx1":
		ziel = oder(m.zuord.Tx1, "dk1")
	case "bg2":
		ziel = oder(m.zuord.Bg2, "lt2")
	case "tx2":
		ziel = oder(m.zuord.Tx2, "dk2")
	}
	if v, ok := m.farben[ziel]; ok && hexFarbe(v) {
		return "#" + v
	}
	return ""
}

func (m *masterInfo) aufloesen(f *aFarbe) string {
	switch {
	case f == nil:
		return ""
	case f.Srgb != nil && hexFarbe(f.Srgb.Val):
		return "#" + f.Srgb.Val
	case f.Schema != nil:
		return m.farbe(f.Schema.Val)
	case f.Sys != nil && hexFarbe(f.Sys.Last):
		return "#" + f.Sys.Last
	case f.Preset != nil:
		// Preset names are CSS colour names in most cases ("black", "red").
		v := strings.ToLower(f.Preset.Val)
		for _, r := range v {
			if r < 'a' || r > 'z' {
				return ""
			}
		}
		return v
	}
	return ""
}

func hintergrundFarbe(bg *pHintergrund, m *masterInfo) string {
	if bg.BgPr != nil && bg.BgPr.SolidFill != nil {
		return m.aufloesen(bg.BgPr.SolidFill)
	}
	if bg.BgRef != nil {
		return m.aufloesen(&bg.BgRef.aFarbe)
	}
	return ""
}

func hexFarbe(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

func oder(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
