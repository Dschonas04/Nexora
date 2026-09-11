// Every text the interface shows must have a German/English pair in
// src/sprache/woerterbuch.ts, otherwise it silently stays in one language.
//
// The probe walks the TSX/TS sources with the TypeScript compiler and collects
// JSX text, user-facing attributes (title, placeholder, aria-label, alt and the
// labels our own components take) and string literals that read like a
// sentence. Each one has to appear on either side of a pair -- or as a pattern
// with {0} -- or be listed in test/texte-ausnahmen.txt (technical strings,
// names, units the probe cannot tell apart from prose).
//
//   node test/texte-probe.cjs            check, exit 1 on missing texts
//   node test/texte-probe.cjs --liste    print the missing texts only
const fs = require("fs");
const path = require("path");
const ts = require("typescript");

const WURZEL = path.join(__dirname, "..");
const SRC = path.join(WURZEL, "src");
const UI_ATTRIBUTE = new Set(["title", "placeholder", "aria-label", "alt", "titel", "unter", "text", "hinweis", "platzhalter", "bestaetigen", "label"]);

const normal = (s) => s.replace(/\s+/g, " ").trim();
const entities = (s) =>
  s.replace(/&nbsp;/g, " ").replace(/&mdash;/g, "—").replace(/&ndash;/g, "–").replace(/&hellip;/g, "…")
   .replace(/&amp;/g, "&").replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&#(\d+);/g, (_, n) => String.fromCharCode(+n));

// --- what the dictionary knows ------------------------------------------------
const quelle = fs.readFileSync(path.join(SRC, "sprache/woerterbuch.ts"), "utf8");
const bekannt = new Set();
const muster = [];
for (const m of quelle.matchAll(/^\s*\[("(?:[^"\\]|\\.)*"), ("(?:[^"\\]|\\.)*")\],$/gm)) {
  for (const seite of [JSON.parse(m[1]), JSON.parse(m[2])]) {
    if (/\{\d\}/.test(seite)) {
      const re = seite.split(/(\{\d\})/).map((t) => (/^\{\d\}$/.test(t) ? "(.+?)" : t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"))).join("");
      muster.push(new RegExp("^" + re + "$", "s"));
    }
    bekannt.add(normal(seite));
  }
}
const ausnahmen = new Set(
  fs.readFileSync(path.join(__dirname, "texte-ausnahmen.txt"), "utf8").split("\n").map((z) => z.trim()).filter((z) => z && !z.startsWith("#")),
);

// --- what the interface shows -------------------------------------------------
function sprachlich(s) {
  if (!/[A-Za-zÄÖÜäöüß]{2}/.test(s)) return false;             // no words at all
  if (/^[a-z0-9_.\-\/:#?=&%@]+$/.test(s)) return false;          // keys, paths, ids
  if (/^(https?:|\/|\.\/|#)/.test(s)) return false;              // addresses
  if (/^[a-z][a-z0-9-]*( [a-z][a-z0-9-]*)*$/.test(s) && !/ /.test(s)) return false; // single css-ish token
  return true;
}
const funde = new Map(); // text -> first place
function merken(text, datei, knoten, sf) {
  const t = normal(entities(text));
  if (!t || !sprachlich(t) || funde.has(t)) return;
  funde.set(t, `${path.relative(WURZEL, datei)}:${sf.getLineAndCharacterOfPosition(knoten.getStart()).line + 1}`);
}
function vorlage(knoten) {
  let s = knoten.head.text;
  knoten.templateSpans.forEach((sp, i) => (s += `{${i}}` + sp.literal.text));
  return s;
}
function datei(p) {
  const sf = ts.createSourceFile(p, fs.readFileSync(p, "utf8"), ts.ScriptTarget.Latest, true, p.endsWith("x") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  (function besuch(k) {
    if (ts.isImportDeclaration(k) || ts.isExportDeclaration(k)) return;
    if (ts.isJsxText(k)) merken(k.text, p, k, sf);
    else if (ts.isJsxAttribute(k) && UI_ATTRIBUTE.has(k.name.getText()) && k.initializer) {
      if (ts.isStringLiteral(k.initializer)) merken(k.initializer.text, p, k, sf);
      else if (ts.isJsxExpression(k.initializer) && k.initializer.expression) {
        const e = k.initializer.expression;
        if (ts.isStringLiteral(e) || ts.isNoSubstitutionTemplateLiteral(e)) merken(e.text, p, k, sf);
        else if (ts.isTemplateExpression(e)) merken(vorlage(e), p, k, sf);
      }
    } else if (ts.isPropertyAssignment(k) && ["titel", "unter", "text", "hinweis", "label"].includes(k.name.getText())) {
      if (ts.isStringLiteral(k.initializer) || ts.isNoSubstitutionTemplateLiteral(k.initializer)) merken(k.initializer.text, p, k, sf);
    } else if (ts.isJsxExpression(k) && k.expression && ts.isJsxElement(k.parent)) {
      const e = k.expression;
      if (ts.isStringLiteral(e)) merken(e.text, p, k, sf);
      else if (ts.isConditionalExpression(e)) {
        for (const zweig of [e.whenTrue, e.whenFalse]) if (ts.isStringLiteral(zweig)) merken(zweig.text, p, k, sf);
      }
    }
    ts.forEachChild(k, besuch);
  })(sf);
}
(function lauf(d) {
  for (const n of fs.readdirSync(d)) {
    const p = path.join(d, n);
    if (fs.statSync(p).isDirectory()) { if (n !== "sprache") lauf(p); }
    else if (/\.tsx?$/.test(n) && !/\.d\.ts$/.test(n)) datei(p);
  }
})(SRC);

// --- compare ------------------------------------------------------------------
const passt = (t) => bekannt.has(t) || ausnahmen.has(t) || muster.some((re) => re.test(t));
const fehlend = [...funde].filter(([t]) => !passt(t));

if (process.argv.includes("--liste")) {
  for (const [t] of fehlend) console.log(t);
  process.exit(0);
}
if (fehlend.length) {
  console.error(`text probe: ${fehlend.length} interface text(s) without a German/English pair.`);
  console.error("Add a pair to src/sprache/woerterbuch.ts, or the text to test/texte-ausnahmen.txt if it is not prose:\n");
  for (const [t, wo] of fehlend) console.error(`  ${wo}\n    ${JSON.stringify(t)}`);
  process.exit(1);
}
console.log(`text probe: ${funde.size} interface texts, all have a pair or an exception`);
