// Pie, bar and line charts as a table.
//
// The diagram stays Mermaid text in a code block, that is what export, import
// and every other Mermaid tool understand. This module only translates in both
// directions: text -> rows for the form in the diagram dialog, rows -> text when
// the form changes.
//
// It reads only what it can write back unchanged. A chart with two series, a
// numeric axis or a horizontal layout is still drawn, it just opens as text:
// a form that silently dropped a second series on the first keystroke would
// destroy work.

export type Diagrammart = "kreis" | "balken" | "linie";

export interface Zeile {
  name: string;
  /** As typed: "1,5" or "1." must survive while someone is still typing. */
  wert: string;
}

export interface Diagrammdaten {
  art: Diagrammart;
  titel: string;
  xAchse: string;
  yAchse: string;
  zeilen: Zeile[];
}

const ZAHL = /^-?\d+(?:[.,]\d+)?$/;

/** "a, \"b, c\", d" -> ["a", "b, c", "d"] */
function liste(innen: string): string[] {
  const teile: string[] = [];
  for (const m of innen.matchAll(/\s*(?:"([^"]*)"|([^,]+))\s*(?:,|$)/g)) {
    if (m[0] === "") break;
    teile.push((m[1] ?? m[2] ?? "").trim());
  }
  return teile;
}

const ohneAnfuehrung = (s: string) => s.trim().replace(/^"(.*)"$/, "$1");

/** Reads a pie or a single-series xychart. Anything else: null. */
export function diagrammLesen(code: string): Diagrammdaten | null {
  const zeilen = code
    .split("\n")
    .map((z) => z.trim())
    .filter((z) => z && !z.startsWith("%%"));
  if (zeilen.length === 0) return null;
  const kopf = zeilen[0];
  let m: RegExpMatchArray | null;

  if (/^pie\b/i.test(kopf)) {
    const d: Diagrammdaten = { art: "kreis", titel: "", xAchse: "", yAchse: "", zeilen: [] };
    if ((m = kopf.match(/^pie(?:\s+showData)?\s+title\s+(.+)$/i))) d.titel = m[1].trim();
    else if (!/^pie(\s+showData)?$/i.test(kopf)) return null;
    for (const z of zeilen.slice(1)) {
      if ((m = z.match(/^title\s+(.+)$/i))) d.titel = m[1].trim();
      else if ((m = z.match(/^"([^"]*)"\s*:\s*(-?\d+(?:\.\d+)?)$/))) d.zeilen.push({ name: m[1], wert: m[2] });
      else return null;
    }
    return d;
  }

  if (/^xychart(-beta)?$/i.test(kopf)) {
    const d: Diagrammdaten = { art: "balken", titel: "", xAchse: "", yAchse: "", zeilen: [] };
    let namen: string[] | null = null;
    let werte: string[] | null = null;
    for (const z of zeilen.slice(1)) {
      if ((m = z.match(/^title\s+(.+)$/i))) d.titel = ohneAnfuehrung(m[1]);
      else if ((m = z.match(/^x-axis\s+(?:("[^"]*"|\S+)\s+)?\[(.*)\]$/i))) {
        d.xAchse = m[1] ? ohneAnfuehrung(m[1]) : "";
        namen = liste(m[2]);
      } else if ((m = z.match(/^y-axis(?:\s+("[^"]*"|[^\s"\d-][^\s"]*))?(?:\s+-?[\d.]+\s*-->\s*-?[\d.]+)?$/i))) {
        // The range is not kept: the form writes a fresh one from the values.
        d.yAchse = m[1] ? ohneAnfuehrung(m[1]) : "";
      } else if ((m = z.match(/^(bar|line)\s+\[(.*)\]$/i))) {
        // A second series cannot be shown as one column of values.
        if (werte) return null;
        d.art = m[1].toLowerCase() === "line" ? "linie" : "balken";
        werte = liste(m[2]);
        if (!werte.every((w) => ZAHL.test(w))) return null;
      } else return null;
    }
    if (!namen || !werte || namen.length !== werte.length) return null;
    d.zeilen = namen.map((name, i) => ({ name, wert: werte![i] }));
    return d;
  }
  return null;
}

/** A quote would end the label in Mermaid; it has no escape for it. */
const sauber = (s: string) => s.replace(/"/g, "'").replace(/[\r\n]+/g, " ").trim();

function zahl(wert: string): string {
  const n = Number(wert.trim().replace(",", "."));
  return Number.isFinite(n) ? String(n) : "0";
}

/** 60.5 -> 65, 210 -> 250, 0.8 -> 0.9: a round number just above. */
function obereGrenze(n: number): number {
  const stufe = Math.pow(10, Math.floor(Math.log10(n))) / 2;
  return Number((Math.ceil(n / stufe + 1e-9) * stufe).toPrecision(6));
}

/** Writes the rows back as Mermaid text. */
export function diagrammSchreiben(d: Diagrammdaten): string {
  const zeilen = d.zeilen.filter((z) => z.name.trim() || z.wert.trim());
  if (d.art === "kreis") {
    const kopf = d.titel.trim() ? `pie title ${sauber(d.titel)}` : "pie";
    return [kopf, ...zeilen.map((z) => `  "${sauber(z.name)}" : ${zahl(z.wert)}`)].join("\n");
  }
  const aus = ["xychart"];
  if (d.titel.trim()) aus.push(`  title "${sauber(d.titel)}"`);
  const kategorien = zeilen.map((z) => `"${sauber(z.name)}"`).join(", ");
  aus.push(`  x-axis ${d.xAchse.trim() ? `"${sauber(d.xAchse)}" ` : ""}[${kategorien}]`);
  // Mermaid starts the axis at the smallest value, so of 60 and 15 the second
  // bar is a sliver and looks like a quarter of nothing. With only positive
  // values a bar axis starts at zero and ends a little above the largest.
  const werte = zeilen.map((z) => Number(zahl(z.wert)));
  const groesster = Math.max(0, ...werte);
  // Bars only: a line shows a course, and a temperature between 14 and 21
  // drawn from zero would be a flat line at the top.
  const bereich =
    d.art === "balken" && werte.length && Math.min(...werte) >= 0 && groesster > 0
      ? ` 0 --> ${obereGrenze(groesster)}`
      : "";
  const yTitel = d.yAchse.trim() ? ` "${sauber(d.yAchse)}"` : "";
  if (yTitel || bereich) aus.push(`  y-axis${yTitel}${bereich}`);
  aus.push(`  ${d.art === "linie" ? "line" : "bar"} [${zeilen.map((z) => zahl(z.wert)).join(", ")}]`);
  return aus.join("\n");
}
