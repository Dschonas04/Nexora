// Zusammenfuehren einer Seite, die offline und auf dem Server geaendert wurde.
//
// Drei Staende: die Basis (so sah die Seite aus, als offline das erste Mal
// geschrieben wurde), die lokale Fassung und die des Servers. Verglichen wird
// je Block der obersten Ebene ueber seine Kennung, wie BlockNote sie vergibt.
//
// Was nur auf einer Seite geaendert wurde, kommt von dort. Ein Block, den
// beide geaendert haben, bleibt in der Fassung des Servers stehen, und die
// eigene steht als Kopie direkt darunter: lieber ein Absatz doppelt als einer
// verloren. Die Zahl dieser Faelle geht mit zurueck, damit die Anwendung sie
// nennen kann.
'use strict';

const gleich = (a, b) => JSON.stringify(a) === JSON.stringify(b);

function neueKennung() {
  return require('crypto').randomUUID();
}

/** Kopie eines Blocks mit frischen Kennungen, auch fuer die Kinder. */
function kopie(block) {
  const k = JSON.parse(JSON.stringify(block));
  const umbenennen = (b) => {
    if (b && typeof b === 'object') {
      if (typeof b.id === 'string') b.id = neueKennung();
      if (Array.isArray(b.children)) b.children.forEach(umbenennen);
    }
  };
  umbenennen(k);
  return k;
}

function blocke(inhalt) {
  return Array.isArray(inhalt) ? inhalt : [];
}

/**
 * Fuehrt den Inhalt zusammen.
 * @returns {{ inhalt: unknown, konflikte: number }}
 */
function inhaltZusammenfuehren(basis, lokal, server) {
  if (gleich(lokal, basis)) return { inhalt: server, konflikte: 0 };
  if (gleich(server, basis) || gleich(server, lokal)) return { inhalt: lokal, konflikte: 0 };
  // Kein Blockinhalt (alte Seiten, leere Seiten): dann gilt die eigene Fassung,
  // die des Servers bleibt im Verlauf der Seite erhalten.
  if (!Array.isArray(lokal) || !Array.isArray(server)) return { inhalt: lokal, konflikte: 1 };

  const b = new Map(blocke(basis).map((x) => [x.id, x]));
  const l = new Map(blocke(lokal).map((x) => [x.id, x]));
  const s = new Map(blocke(server).map((x) => [x.id, x]));
  let konflikte = 0;

  const ergebnis = [];
  // Erst die Reihenfolge des Servers, Block fuer Block entschieden.
  for (const sb of blocke(server)) {
    const bb = b.get(sb.id);
    const lb = l.get(sb.id);
    if (!lb) {
      // Lokal geloescht. Hat der Server ihn seit der Basis veraendert, bleibt
      // er: dann wusste die Loeschung nichts von der neuen Fassung.
      if (bb && gleich(sb, bb)) continue;
      ergebnis.push(sb);
      continue;
    }
    if (!bb) {
      // Auf beiden Seiten neu mit derselben Kennung: kann nur derselbe sein.
      ergebnis.push(gleich(lb, sb) ? sb : sb);
      if (!gleich(lb, sb)) {
        ergebnis.push(kopie(lb));
        konflikte++;
      }
      continue;
    }
    const lokalGeaendert = !gleich(lb, bb);
    const serverGeaendert = !gleich(sb, bb);
    if (lokalGeaendert && serverGeaendert && !gleich(lb, sb)) {
      ergebnis.push(sb, kopie(lb));
      konflikte++;
    } else {
      ergebnis.push(lokalGeaendert ? lb : sb);
    }
  }

  // Dann die lokal neuen Bloecke, jeweils hinter ihren lokalen Vorgaenger.
  const lokalListe = blocke(lokal);
  lokalListe.forEach((lb, i) => {
    if (s.has(lb.id)) return;
    if (b.has(lb.id)) {
      // Auf dem Server geloescht, lokal noch da. Lokal geaendert: behalten.
      if (gleich(lb, b.get(lb.id))) return;
    }
    let pos = 0;
    for (let j = i - 1; j >= 0; j--) {
      const k = ergebnis.findIndex((x) => x.id === lokalListe[j].id);
      if (k >= 0) {
        pos = k + 1;
        break;
      }
    }
    ergebnis.splice(pos, 0, lb);
  });

  return { inhalt: ergebnis, konflikte };
}

/** Ein einfaches Feld: geaendert gewinnt, beide geaendert: die eigene Fassung. */
function feldZusammenfuehren(basis, lokal, server) {
  if (gleich(lokal, basis)) return { wert: server, konflikt: false };
  if (gleich(server, basis) || gleich(server, lokal)) return { wert: lokal, konflikt: false };
  return { wert: lokal, konflikt: true };
}

module.exports = { inhaltZusammenfuehren, feldZusammenfuehren };
