// Checks the German/English layer without a browser: plain lookups in both
// directions, patterns with values, values that are words themselves, and that
// a pattern does not swallow whole sentences. Run like the other probes:
//
//   npx esbuild test/sprache-probe.ts --bundle --format=esm --platform=node --outfile=/tmp/sprache.mjs
//   node /tmp/sprache.mjs
import { uebersetze } from "../src/sprache/index";
import { PAARE } from "../src/sprache/woerterbuch";

const faelle: [string, "de" | "en", string | null][] = [
  // plain lookups, whitespace around the text is kept
  ["Save", "de", "Speichern"],
  ["Speichern", "en", "Save"],
  ["  Loading…  ", "de", "  Wird geladen…  "],
  // patterns, both directions
  ["3 people added.", "de", "3 Personen hinzugefügt."],
  ["Gruppe „Vertrieb“ angelegt.", "en", "Group “Vertrieb” created."],
  ["2 von 7 Konten", "en", "2 of 7 accounts"],
  // a value that is itself a word: units are translated, names are not
  ["Create 3 accounts", "de", "3 Konten anlegen"],
  ["Group “Vertrieb” created.", "de", "Gruppe „Vertrieb“ angelegt."],
  // the German dative after "vor"
  ["3 hours ago", "de", "vor 3 Stunden"],
  ["2 days ago", "de", "vor 2 Tagen"],
  // "{0} auf {1}" must not swallow a sentence that happens to contain "auf"
  ["Firefox auf Linux", "en", "Firefox on Linux"],
  ["Wirkt auf die Selbstregistrierung, nicht auf Konten, die ein Administrator anlegt.", "en", null],
  // unknown text and bare values stay untouched
  ["MeinText", "de", null],
  ["{0}", "de", null],
  ["42", "en", null],
];

const fehler: string[] = [];
for (const [text, sprache, erwartet] of faelle) {
  const ist = uebersetze(text, sprache);
  if (ist !== erwartet) fehler.push(`${JSON.stringify(text)} -> ${sprache}: expected ${JSON.stringify(erwartet)}, got ${JSON.stringify(ist)}`);
}

// Every pair must use the same placeholders on both sides.
for (const [de, en] of PAARE) {
  const a = (de.match(/\{\d\}/g) ?? []).sort().join();
  const b = (en.match(/\{\d\}/g) ?? []).sort().join();
  if (a !== b) fehler.push(`placeholders differ: ${JSON.stringify(de)} / ${JSON.stringify(en)}`);
}

if (fehler.length) {
  console.error("language probe failed:\n  " + fehler.join("\n  "));
  process.exit(1);
}
console.log(`language probe: ${faelle.length} cases and ${PAARE.length} pairs passed`);
