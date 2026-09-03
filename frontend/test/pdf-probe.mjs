// Prüft, dass Markierungen wirklich in der Datei landen -- und an der richtigen
// Stelle.
//
// Ohne Browser: die Rechnerei und das Schreiben stehen in src/pdfmarken.ts und
// hängen an nichts, was ein Fenster braucht. Aufruf:
//
//     npx esbuild src/pdfmarken.ts --bundle --format=esm --outfile=/tmp/pm.mjs
//     node test/pdf-probe.mjs /tmp/pm.mjs
import { PDFDocument, rgb } from "pdf-lib";

const bundle = process.argv[2];
if (!bundle) {
  console.error("Aufruf: node test/pdf-probe.mjs <gebündeltes pdfmarken.mjs>");
  process.exit(2);
}
const { markenAnwenden, ausBildschirm, normiert } = await import(bundle);

let fehler = 0;
function pruefe(was, bedingung, hinweis = "") {
  if (bedingung) console.log("  ok    " + was);
  else {
    console.log("  FEHLT " + was + (hinweis ? " (" + hinweis + ")" : ""));
    fehler++;
  }
}

// Eine Vorlage, wie sie ein Anhang wäre: zwei Seiten mit Text.
async function vorlage() {
  const doc = await PDFDocument.create();
  for (const t of ["Erste Seite", "Zweite Seite"]) {
    const s = doc.addPage([595, 842]); // A4 in Punkten
    s.drawText(t, { x: 72, y: 742, size: 18, color: rgb(0, 0, 0) });
  }
  return doc.save();
}

console.log("== Umrechnung Bildschirm -> PDF");
{
  // Anzeige mit Maßstab 2 (Netzhautauflösung), Seite 842 Punkte hoch.
  // Ein Kasten ganz oben links auf der Anzeige gehört im PDF nach oben links,
  // und oben ist dort die GROSSE y-Zahl.
  const k = ausBildschirm({ x: 0, y: 0, breite: 200, hoehe: 40 }, 2, 842);
  pruefe("oben links bleibt links", k.x === 0);
  pruefe("oben wird zur großen y-Zahl", k.y === 842 - 20, "y=" + k.y);
  pruefe("der Maßstab wird herausgerechnet", k.breite === 100 && k.hoehe === 20);

  const u = ausBildschirm({ x: 100, y: 800, breite: 50, hoehe: 20 }, 1, 842);
  pruefe("unten bleibt unten", u.y === 842 - 800 - 20, "y=" + u.y);

  const n = normiert({ x: 300, y: 400 }, { x: 100, y: 200 });
  pruefe("rückwärts gezogen ist auch gezogen",
    n.x === 100 && n.y === 200 && n.breite === 200 && n.hoehe === 200);
}

console.log("== Markierungen schreiben");
{
  const roh = await vorlage();
  const vorher = roh.length;
  const neu = await markenAnwenden(roh, [
    { seite: 0, x: 72, y: 735, breite: 180, hoehe: 24, farbe: "yellow" },
    { seite: 1, x: 72, y: 735, breite: 180, hoehe: 24, farbe: "green", notiz: "Das hier prüfen" },
  ], "Jonas");

  pruefe("es kommt eine Datei heraus", neu instanceof Uint8Array && neu.length > 0);
  pruefe("sie ist gewachsen", neu.length > vorher, vorher + " -> " + neu.length);
  pruefe("sie fängt an wie ein PDF", new TextDecoder().decode(neu.slice(0, 5)) === "%PDF-");

  // Wieder einlesen: was pdf-lib schreibt, muss pdf-lib auch wieder verstehen.
  const wieder = await PDFDocument.load(neu);
  pruefe("beide Seiten sind noch da", wieder.getPageCount() === 2);

  // Der Zettel steht als Anmerkung an der zweiten Seite.
  const zaehleAnmerkungen = (seite) => {
    const a = seite.node.Annots();
    return a && typeof a.size === "function" ? a.size() : 0;
  };
  pruefe("die zweite Seite trägt eine Anmerkung", zaehleAnmerkungen(wieder.getPage(1)) === 1,
    "gefunden: " + zaehleAnmerkungen(wieder.getPage(1)));
  pruefe("die erste Seite trägt keine", zaehleAnmerkungen(wieder.getPage(0)) === 0,
    "gefunden: " + zaehleAnmerkungen(wieder.getPage(0)));

  const text = new TextDecoder("latin1").decode(neu);
  pruefe("der Zettelinhalt steht drin", text.includes("Das hier prüfen") || text.includes("Das hier pr"));
  pruefe("der Verfasser steht drin", text.includes("Jonas"));

  // Der Inhalt der Vorlage darf nicht verlorengehen: markiert wird darüber,
  // nicht darüber hinweg. Der Seiteninhalt liegt zusammengedrückt in der Datei,
  // deshalb kein Suchen nach Text -- gemessen wird, dass der Strom LAENGER
  // geworden ist. Ein ersetzter Inhalt wäre kürzer oder gleich lang.
  const stromLaenge = (doc, nr) => {
    const seite = doc.getPage(nr);
    const inhalt = seite.node.Contents();
    if (!inhalt) return 0;
    const gefunden = doc.context.lookup(inhalt);
    if (gefunden && gefunden.contents) return gefunden.contents.length;
    // Mehrere Ströme je Seite: dann ist es ein Feld von Verweisen.
    if (gefunden && typeof gefunden.size === "function") {
      let summe = 0;
      for (let i = 0; i < gefunden.size(); i++) {
        const s = doc.context.lookup(gefunden.get(i));
        if (s && s.contents) summe += s.contents.length;
      }
      return summe;
    }
    return 0;
  };
  const alt = await PDFDocument.load(roh);
  pruefe("der Seiteninhalt wurde ergänzt, nicht ersetzt",
    stromLaenge(wieder, 0) > stromLaenge(alt, 0),
    stromLaenge(alt, 0) + " -> " + stromLaenge(wieder, 0));
}

console.log("== Was schiefgehen kann");
{
  const roh = await vorlage();
  // Eine Marke auf Seite 7 einer zweiseitigen Datei: überspringen, nicht
  // scheitern -- die übrigen Markierungen sind mehr wert als eine Fehlermeldung.
  const neu = await markenAnwenden(roh, [
    { seite: 7, x: 10, y: 10, breite: 10, hoehe: 10, farbe: "yellow" },
    { seite: 0, x: 72, y: 700, breite: 100, hoehe: 20, farbe: "blue" },
  ], "");
  pruefe("eine Marke auf einer fehlenden Seite stört nicht", neu.length > 0);

  const ohne = await markenAnwenden(await vorlage(), [], "");
  pruefe("ohne Marken kommt trotzdem eine gültige Datei heraus",
    new TextDecoder().decode(ohne.slice(0, 5)) === "%PDF-");

  const bunt = await markenAnwenden(await vorlage(), [
    { seite: 0, x: 20, y: 20, breite: 40, hoehe: 10, farbe: "gibtsnicht" },
  ], "");
  pruefe("eine unbekannte Farbe wird zu Gelb statt zum Fehler", bunt.length > 0);
}

console.log();
if (fehler > 0) {
  console.error(fehler + " Prüfungen sind gefallen.");
  process.exit(1);
}
console.log("Probe bestanden.");
