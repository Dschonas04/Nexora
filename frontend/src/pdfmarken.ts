// Markierungen in eine vorhandene PDF-Datei schreiben.
//
// Getrennt von der Oberfläche, weil das hier die Stelle ist, an der etwas
// schiefgehen kann: Koordinaten. Ein PDF zählt von unten links, ein Bildschirm
// von oben links, und dazwischen liegt ein Maßstab. Wer das verwechselt, bekommt
// Markierungen, die am oberen Rand kleben statt auf dem Satz. Als eigene Datei
// lässt sich das ohne Browser prüfen, siehe test/pdf-probe.mjs.
//
// Zwei verschiedene Dinge werden geschrieben:
//
// Die Markierung selbst wird in den Seiteninhalt GEZEICHNET -- ein durchsichtiges
// Rechteck über dem Text. Gezeichnetes überlebt jeden Betrachter und jeden
// Ausdruck. Eine Anmerkung dagegen ist ein Zettel und wird als echte
// PDF-Anmerkung angehängt: sie hängt am Ort, lässt sich aufklappen und trägt
// einen Verfasser, und genau das erwartet man von einer Anmerkung.
import { PDFDocument, PDFName, PDFString, rgb } from "pdf-lib";

// Die Farben der Markierung, dieselben Namen wie im Editor. Blass, weil darauf
// gelesen wird.
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
 * markenAnwenden schreibt die Markierungen in die Datei und gibt die neue
 * zurück. Die Vorlage bleibt unangetastet -- ersetzt wird erst oben, wenn das
 * Ergebnis steht.
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
    // Eine Marke auf einer Seite, die es nicht gibt, wird übersprungen statt
    // die ganze Datei scheitern zu lassen: die übrigen Markierungen sind mehr
    // wert als eine Fehlermeldung.
    if (!seite) continue;

    const [r, g, b] = MARKIERFARBEN[m.farbe] ?? MARKIERFARBEN.yellow;
    seite.drawRectangle({
      x: m.x,
      y: m.y,
      width: m.breite,
      height: m.hoehe,
      color: rgb(r, g, b),
      // Durchsichtig, sonst wäre die Markierung ein Balken über dem Text statt
      // einer Markierung darauf.
      opacity: 0.35,
      borderWidth: 0,
    });

    if (!m.notiz) continue;

    // Der Zettel als echte Anmerkung. pdf-lib hat dafür keinen fertigen Weg,
    // also wird das Wörterbuch von Hand gebaut; die Namen stehen so im
    // PDF-Standard.
    const zettel = doc.context.obj({
      Type: "Annot",
      Subtype: "Text",
      Name: "Comment",
      // Oben rechts an der Markierung, damit er den markierten Text nicht
      // verdeckt.
      Rect: [m.x + m.breite, m.y + m.hoehe - 18, m.x + m.breite + 18, m.y + m.hoehe],
      Contents: PDFString.of(m.notiz),
      T: PDFString.of(verfasser || "Nexora"),
      C: [r, g, b],
      // 4 ist "Print": ein Zettel, den man nur am Bildschirm sieht, fehlt
      // ausgerechnet dem, der die Seite ausdruckt.
      F: 4,
    });
    seite.node.addAnnot(doc.context.register(zettel));
  }

  // Die Angabe, womit die Datei zuletzt bearbeitet wurde. Kostet nichts und
  // beantwortet später die Frage, woher die Markierungen kommen.
  doc.setProducer("Nexora");
  doc.setModificationDate(new Date());
  return doc.save({ useObjectStreams: false });
}

/**
 * ausBildschirm rechnet ein auf der Anzeige gezogenes Rechteck in
 * PDF-Koordinaten um.
 *
 * Der Bildschirm zählt von oben, das PDF von unten -- deshalb die Subtraktion.
 * Und der Maßstab: die Anzeige ist meist kleiner als die Seite in Punkten.
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
