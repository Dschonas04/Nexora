// Every text the interface shows must have a German/English pair in
// src/sprache/woerterbuch.ts, otherwise it silently stays in one language.
//
// The probe parses the TSX/TS sources and collects JSX text, user-facing
// attributes (title, placeholder, aria-label, alt and the labels our own
// components take) and string literals that stand as text in JSX. Each one has
// to appear on either side of a pair -- or as a pattern with {0} -- or be listed
// in test/texte-ausnahmen.txt (technical strings, names, units the probe cannot
// tell apart from prose).
//
// It parses with @babel/parser and not with the TypeScript compiler: TypeScript
// 7 is the native compiler and no longer ships a JavaScript API, and a second,
// older TypeScript next to it would bring its own `tsc` and fight over the name.
//
//   node test/texte-probe.cjs            check, exit 1 on missing texts
//   node test/texte-probe.cjs --liste    print the missing texts only
const fs = require("fs");
const path = require("path");
const { parse } = require("@babel/parser");

const WURZEL = path.join(__dirname, "..");
const SRC = path.join(WURZEL, "src");
const UI_ATTRIBUTE = new Set(["title", "placeholder", "aria-label", "alt", "titel", "unter", "text", "hinweis", "platzhalter", "bestaetigen", "label"]);
const UI_SCHLUESSEL = new Set(["titel", "unter", "text", "hinweis", "label"]);

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
  return true;
}
const funde = new Map(); // text -> first place
function merken(text, datei, knoten) {
  const t = normal(entities(text));
  if (!t || !sprachlich(t) || funde.has(t)) return;
  funde.set(t, `${path.relative(WURZEL, datei)}:${knoten.loc ? knoten.loc.start.line : "?"}`);
}
// A template literal becomes "text {0} text {1}", the form patterns have.
function vorlage(t) {
  return t.quasis.map((q, i) => q.value.cooked + (i < t.expressions.length ? `{${i}}` : "")).join("");
}
const schluesselName = (k) => (k.type === "Identifier" ? k.name : k.type === "StringLiteral" ? k.value : null);

function besuche(knoten, eltern, datei) {
  if (!knoten || typeof knoten.type !== "string") return;
  switch (knoten.type) {
    case "ImportDeclaration":
    case "ExportAllDeclaration":
      return;
    case "JSXText":
      merken(knoten.value, datei, knoten);
      break;
    case "JSXAttribute": {
      const name = knoten.name && knoten.name.name;
      const w = knoten.value;
      if (UI_ATTRIBUTE.has(name) && w) {
        if (w.type === "StringLiteral") merken(w.value, datei, knoten);
        else if (w.type === "JSXExpressionContainer") {
          const e = w.expression;
          if (e.type === "StringLiteral") merken(e.value, datei, knoten);
          else if (e.type === "TemplateLiteral") merken(e.expressions.length ? vorlage(e) : e.quasis[0].value.cooked, datei, knoten);
        }
      }
      break;
    }
    case "ObjectProperty": {
      const name = schluesselName(knoten.key);
      const w = knoten.value;
      if (UI_SCHLUESSEL.has(name)) {
        if (w.type === "StringLiteral") merken(w.value, datei, knoten);
        else if (w.type === "TemplateLiteral" && !w.expressions.length) merken(w.quasis[0].value.cooked, datei, knoten);
      }
      break;
    }
    case "JSXExpressionContainer":
      if (eltern && (eltern.type === "JSXElement" || eltern.type === "JSXFragment")) {
        const e = knoten.expression;
        if (e.type === "StringLiteral") merken(e.value, datei, knoten);
        else if (e.type === "ConditionalExpression") {
          for (const zweig of [e.consequent, e.alternate]) if (zweig.type === "StringLiteral") merken(zweig.value, datei, knoten);
        }
      }
      break;
  }
  for (const schluessel of Object.keys(knoten)) {
    if (schluessel === "loc" || schluessel === "start" || schluessel === "end" || schluessel === "extra" || schluessel.endsWith("Comments")) continue;
    const kind = knoten[schluessel];
    if (Array.isArray(kind)) for (const k of kind) besuche(k, knoten, datei);
    else if (kind && typeof kind.type === "string") besuche(kind, knoten, datei);
  }
}

function datei(p) {
  const ast = parse(fs.readFileSync(p, "utf8"), {
    sourceType: "module",
    plugins: p.endsWith("x") ? ["typescript", "jsx"] : ["typescript"],
  });
  besuche(ast.program, null, p);
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
