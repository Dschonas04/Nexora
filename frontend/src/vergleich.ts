// What changed between two states of a page.
//
// A BlockNote document is a tree of JSON, and a diff of that JSON would show
// block ids and style objects nobody reads. So both states are first turned
// into lines the way a reader sees them -- "## Heading", "• item", "[x] task"
// -- and those lines are compared. Inside a changed line the words are
// compared once more, so a fixed typo shows as one word and not as a whole
// paragraph struck out and written again.
//
// Pure functions, no DOM: the probe in test/diff-check.ts runs them in Node.

export type Teil = { art: "gleich" | "weg" | "neu"; text: string };
export type Zeile =
  | { art: "gleich"; text: string }
  | { art: "weg"; text: string }
  | { art: "neu"; text: string }
  | { art: "geaendert"; teile: Teil[] };

type Block = {
  type?: string;
  props?: Record<string, unknown>;
  content?: unknown;
  children?: unknown;
};

function inline(inhalt: unknown): string {
  if (typeof inhalt === "string") return inhalt;
  if (!Array.isArray(inhalt)) return "";
  return inhalt
    .map((t) => {
      const o = t as { text?: string; content?: unknown };
      return (typeof o.text === "string" ? o.text : "") + (o.content ? inline(o.content) : "");
    })
    .join("");
}

/** The lines a reader sees, one per block, nested ones indented. */
export function zeilenAus(dokument: unknown, tiefe = 0): string[] {
  if (!Array.isArray(dokument)) return [];
  const raus: string[] = [];
  const einzug = "  ".repeat(tiefe);
  for (const roh of dokument) {
    const b = roh as Block;
    const p = b.props ?? {};
    const text = inline(b.content);
    switch (b.type) {
      case "heading":
        raus.push(einzug + "#".repeat(Number(p.level) || 1) + " " + text);
        break;
      case "bulletListItem":
        raus.push(einzug + "• " + text);
        break;
      case "numberedListItem":
        raus.push(einzug + "1. " + text);
        break;
      case "checkListItem":
        raus.push(einzug + (p.checked ? "[x] " : "[ ] ") + text);
        break;
      case "toggleListItem":
        raus.push(einzug + "▸ " + text);
        break;
      case "quote":
        raus.push(einzug + "> " + text);
        break;
      case "codeBlock":
        raus.push(einzug + "```" + String(p.language ?? ""));
        for (const z of text.split("\n")) raus.push(einzug + z);
        raus.push(einzug + "```");
        break;
      case "table": {
        const zeilen = (b.content as { rows?: { cells?: unknown[] }[] } | undefined)?.rows ?? [];
        for (const r of zeilen) {
          const zellen = (r.cells ?? []).map((c) => {
            const z = c as { content?: unknown };
            return inline(z && typeof z === "object" && "content" in z ? z.content : c);
          });
          raus.push(einzug + "| " + zellen.join(" | ") + " |");
        }
        break;
      }
      case "image":
      case "video":
      case "audio":
      case "file":
        raus.push(einzug + `[${b.type}: ${String(p.name || p.caption || p.url || "")}]`);
        break;
      default:
        if (text || b.type === "paragraph") raus.push(einzug + text);
        else raus.push(einzug + `[${b.type ?? "?"}]`);
    }
    if (Array.isArray(b.children) && b.children.length > 0) raus.push(...zeilenAus(b.children, tiefe + 1));
  }
  // Empty paragraphs at the end are the editor's, not the author's.
  while (raus.length > 0 && raus[raus.length - 1].trim() === "") raus.pop();
  return raus;
}

/**
 * Longest common subsequence, the textbook way: a table of lengths, then a
 * walk back through it. Quadratic, which is why the callers cap the input.
 */
function lcs<T>(a: T[], b: T[]): { art: "gleich" | "weg" | "neu"; wert: T }[] {
  const n = a.length;
  const m = b.length;
  const breite = m + 1;
  const tab = new Uint32Array((n + 1) * breite);
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      tab[i * breite + j] =
        a[i] === b[j] ? tab[(i + 1) * breite + j + 1] + 1 : Math.max(tab[(i + 1) * breite + j], tab[i * breite + j + 1]);
    }
  }
  const raus: { art: "gleich" | "weg" | "neu"; wert: T }[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      raus.push({ art: "gleich", wert: a[i] });
      i++;
      j++;
    } else if (tab[(i + 1) * breite + j] >= tab[i * breite + j + 1]) {
      raus.push({ art: "weg", wert: a[i++] });
    } else {
      raus.push({ art: "neu", wert: b[j++] });
    }
  }
  while (i < n) raus.push({ art: "weg", wert: a[i++] });
  while (j < m) raus.push({ art: "neu", wert: b[j++] });
  return raus;
}

/** The words of two versions of one line, compared. */
export function woerter(alt: string, neu: string): Teil[] {
  const a = alt.split(/(\s+)/).filter((t) => t !== "");
  const b = neu.split(/(\s+)/).filter((t) => t !== "");
  if (a.length * b.length > 250_000) {
    return [
      { art: "weg", text: alt },
      { art: "neu", text: neu },
    ];
  }
  // Neighbouring pieces of the same kind are merged, so "two words" removed
  // is one strike-through and not two with a gap.
  const raus: Teil[] = [];
  for (const s of lcs(a, b)) {
    const letzt = raus[raus.length - 1];
    if (letzt && letzt.art === s.art) letzt.text += s.wert;
    else raus.push({ art: s.art, text: s.wert });
  }
  return raus;
}

/** How much two lines have in common, from 0 to 1. */
function aehnlich(alt: string, neu: string): number {
  const teile = woerter(alt, neu);
  const gleich = teile.filter((t) => t.art === "gleich").reduce((n, t) => n + t.text.length, 0);
  return (2 * gleich) / Math.max(1, alt.length + neu.length);
}

/**
 * The comparison of two documents, line by line. A removed line directly
 * followed by an added one that resembles it is shown as one changed line.
 */
export function vergleiche(altDok: unknown, neuDok: unknown): Zeile[] {
  const a = zeilenAus(altDok);
  const b = zeilenAus(neuDok);
  let schritte: { art: "gleich" | "weg" | "neu"; wert: string }[];
  if (a.length * b.length > 4_000_000) {
    // Too large to compare line by line in the browser: say everything
    // changed rather than freeze the tab.
    schritte = [...a.map((w) => ({ art: "weg" as const, wert: w })), ...b.map((w) => ({ art: "neu" as const, wert: w }))];
  } else {
    schritte = lcs(a, b);
  }
  const raus: Zeile[] = [];
  for (let k = 0; k < schritte.length; ) {
    if (schritte[k].art === "gleich") {
      raus.push({ art: "gleich", text: schritte[k].wert });
      k++;
      continue;
    }
    // A run of removals and additions; pair them in order where they resemble
    // each other.
    const weg: string[] = [];
    const neu: string[] = [];
    while (k < schritte.length && schritte[k].art !== "gleich") {
      if (schritte[k].art === "weg") weg.push(schritte[k].wert);
      else neu.push(schritte[k].wert);
      k++;
    }
    const paare = Math.min(weg.length, neu.length);
    for (let p = 0; p < paare; p++) {
      if (aehnlich(weg[p], neu[p]) >= 0.4) raus.push({ art: "geaendert", teile: woerter(weg[p], neu[p]) });
      else {
        raus.push({ art: "weg", text: weg[p] });
        raus.push({ art: "neu", text: neu[p] });
      }
    }
    for (const w of weg.slice(paare)) raus.push({ art: "weg", text: w });
    for (const w of neu.slice(paare)) raus.push({ art: "neu", text: w });
  }
  return raus;
}

/** Counts for the summary line. */
export function zaehle(zeilen: Zeile[]): { neu: number; weg: number; geaendert: number } {
  let neu = 0;
  let weg = 0;
  let geaendert = 0;
  for (const z of zeilen) {
    if (z.art === "neu") neu++;
    else if (z.art === "weg") weg++;
    else if (z.art === "geaendert") geaendert++;
  }
  return { neu, weg, geaendert };
}
