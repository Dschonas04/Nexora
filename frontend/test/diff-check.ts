// Checks the comparison of two page states (src/vergleich.ts) and the reading
// of task marks (src/aufgaben.ts). Run like the other probes:
//
//   npx esbuild test/diff-check.ts --bundle --format=esm --platform=node --outfile=/tmp/vergleich.mjs
//   node /tmp/vergleich.mjs
import { vergleiche, woerter, zaehle, zeilenAus } from "../src/vergleich";
import { isoDatum, ohneMarken } from "../src/aufgaben";

const fehler: string[] = [];
const absatz = (id: string, text: string, typ = "paragraph", props: Record<string, unknown> = {}) => ({
  id,
  type: typ,
  props,
  content: [{ type: "text", text, styles: {} }],
  children: [],
});

const alt = [
  absatz("1", "Plan", "heading", { level: 2 }),
  absatz("2", "Wir treffen uns am Montag im Büro."),
  absatz("3", "Angebot schicken", "checkListItem", { checked: false }),
  absatz("4", "Diese Zeile fällt weg."),
];
const neu = [
  absatz("1", "Plan", "heading", { level: 2 }),
  absatz("2", "Wir treffen uns am Dienstag im Büro."),
  absatz("3", "Angebot schicken", "checkListItem", { checked: true }),
  absatz("5", "Ganz neu dazu."),
];

const z = zeilenAus(alt);
if (z[0] !== "## Plan" || z[2] !== "[ ] Angebot schicken") fehler.push(`lines read wrong: ${JSON.stringify(z)}`);

const v = vergleiche(alt, neu);
const s = zaehle(v);
if (v[0].art !== "gleich") fehler.push("the unchanged heading is not 'gleich'");
const montag = v.find((x) => x.art === "geaendert" && x.teile.some((t) => t.art === "weg" && t.text.includes("Montag")));
if (!montag || montag.art !== "geaendert") fehler.push("Montag → Dienstag is not shown as a changed word");
else if (montag.teile.filter((t) => t.art !== "gleich").length !== 2) fehler.push(`more than the one word marked: ${JSON.stringify(montag.teile)}`);
if (s.geaendert < 2) fehler.push(`expected the date and the tick as changes, got ${JSON.stringify(s)}`);
if (!v.some((x) => x.art === "weg" && x.text.includes("fällt weg"))) fehler.push("the removed line is missing");
if (!v.some((x) => x.art === "neu" && x.text.includes("Ganz neu"))) fehler.push("the added line is missing");

if (vergleiche(alt, alt).some((x) => x.art !== "gleich")) fehler.push("a page compared with itself shows changes");

const w = woerter("eins zwei drei", "eins drei");
if (w.map((t) => t.art).join() !== "gleich,weg,gleich") fehler.push(`word diff: ${JSON.stringify(w)}`);

if (isoDatum("3.1.2027") !== "2027-01-03") fehler.push("German date not read");
if (isoDatum("2026-10-30") !== "2026-10-30") fehler.push("ISO date not kept");
if (ohneMarken("Angebot 📅 2026-10-30 @anna") !== "Angebot @anna") fehler.push("date mark not removed");

if (fehler.length) {
  console.error("diff probe failed:\n  " + fehler.join("\n  "));
  process.exit(1);
}
console.log("diff probe: all checks passed");
