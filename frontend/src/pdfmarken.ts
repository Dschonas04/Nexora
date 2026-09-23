// Writing highlights into an existing PDF file.
//
// Separate from the interface, because this is the spot where something can go
// wrong: coordinates. A PDF counts from the bottom left, a screen from the top
// left, and between them lies a scale. Confuse that and one gets highlights
// clinging to the top edge instead of sitting on the type. As a file of its own
// this can be checked without a browser, see test/pdf-probe.mjs.
//
// Two different things are written:
//
// The highlight itself is DRAWN into the page content -- a transparent
// rectangle over the text. What is drawn survives every viewer and every
// printout. A note, by contrast, is a slip of paper and is attached as a real
// PDF annotation: it hangs in place, can be opened and carries an author, and
// that is exactly what one expects of a note.
import { PDFDocument, PDFName, PDFString, rgb } from "pdf-lib";

// The colours of the highlight, the same names as in the editor. Pale, because
// they are read on top of.
export const MARKIERFARBEN: Record<string, [number, number, number]> = {
  yellow: [1.0, 0.93, 0.35],
  green: [0.55, 0.9, 0.6],
  blue: [0.55, 0.78, 0.98],
  pink: [0.98, 0.62, 0.78],
  orange: [1.0, 0.75, 0.4],
};

export interface Marke {
  /** Seitenzahl, von 0 an. */
  seite: number;
  /** In PDF-Punkten, Ursprung unten links. */
  x: number;
  y: number;
  breite: number;
  hoehe: number;
  farbe: string;
  /** Optionaler Zettel an dieser Stelle. */
  notiz?: string;
}

/**
 * markenAnwenden writes the highlights into the file and returns the new one.
 * The template stays untouched -- it is only replaced further up, once the
 * result is there.
 */
export async function markenAnwenden(
  vorlage: ArrayBuffer | Uint8Array,
  marken: Marke[],
  verfasser: string,
): Promise<Uint8Array> {
  const doc = await PDFDocument.load(vorlage, { ignoreEncryption: true });
  const seiten = doc.getPages();

  for (const m of marken) {
    const seite = seiten[m.seite];
    // A mark on a page that does not exist is skipped instead of letting the
    // whole file fail: the remaining highlights are worth more than an error
    // message.
    if (!seite) continue;

    const [r, g, b] = MARKIERFARBEN[m.farbe] ?? MARKIERFARBEN.yellow;
    seite.drawRectangle({
      x: m.x,
      y: m.y,
      width: m.breite,
      height: m.hoehe,
      color: rgb(r, g, b),
      // Transparent, otherwise the highlight would be a bar over the text
      // instead of a highlight on it.
      opacity: 0.35,
      borderWidth: 0,
    });

    if (!m.notiz) continue;

    // The slip as a real annotation. pdf-lib has no ready-made way for it, so
    // the dictionary is built by hand; the names stand like this in the PDF
    // standard.
    const zettel = doc.context.obj({
      Type: "Annot",
      Subtype: "Text",
      Name: "Comment",
      // Top right of the highlight, so it does not cover the highlighted
      // text.
      Rect: [m.x + m.breite, m.y + m.hoehe - 18, m.x + m.breite + 18, m.y + m.hoehe],
      Contents: PDFString.of(m.notiz),
      T: PDFString.of(verfasser || "Nexora"),
      C: [r, g, b],
      // 4 is "Print": a slip one only sees on the screen is missing for
      // precisely the person printing the page.
      F: 4,
    });
    seite.node.addAnnot(doc.context.register(zettel));
  }

  // The note of what the file was last edited with. Costs nothing and answers
  // the later question of where the highlights come from.
  doc.setProducer("Nexora");
  doc.setModificationDate(new Date());
  return doc.save({ useObjectStreams: false });
}

/**
 * ausBildschirm converts a rectangle dragged on the display into PDF
 * coordinates.
 *
 * The screen counts from the top, the PDF from the bottom -- hence the
 * subtraction. And the scale: the display is usually smaller than the page in
 * points.
 */
export function ausBildschirm(
  kasten: { x: number; y: number; breite: number; hoehe: number },
  massstab: number,
  seitenHoehePunkte: number,
): { x: number; y: number; breite: number; hoehe: number } {
  const breite = kasten.breite / massstab;
  const hoehe = kasten.hoehe / massstab;
  return {
    x: kasten.x / massstab,
    y: seitenHoehePunkte - kasten.y / massstab - hoehe,
    breite,
    hoehe,
  };
}

/** normiert ein gezogenes Rechteck: negativ gezogen ist auch gezogen. */
export function normiert(
  von: { x: number; y: number },
  bis: { x: number; y: number },
): { x: number; y: number; breite: number; hoehe: number } {
  return {
    x: Math.min(von.x, bis.x),
    y: Math.min(von.y, bis.y),
    breite: Math.abs(bis.x - von.x),
    hoehe: Math.abs(bis.y - von.y),
  };
}
