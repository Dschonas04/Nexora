// German and English for the whole interface, without touching the components.
//
// The components keep their texts as they are, some in English, some in German.
// This module sits between React and the screen: a MutationObserver sees every
// text node and every title/placeholder/aria-label that React writes and, if the
// pair list knows it, puts the text in the chosen language in its place. React
// is not disturbed by it: it compares against its own props, not against the
// DOM, and writes a node again only when its value really changes -- which the
// observer then sees and translates anew.
//
// What is never translated: the editor, comment bodies, anything inside a
// translate="no" or .notranslate element (the standard HTML way of saying
// "this is content, not interface"), and form values.
//
// The choice lives in localStorage; without one the browser's language decides.
import { useSyncExternalStore } from "react";
import { PAARE } from "./woerterbuch";

export type Sprache = "de" | "en";

const SCHLUESSEL = "nexora.sprache";
const ATTRIBUTE = ["title", "placeholder", "aria-label", "alt"];
const AUSGENOMMEN =
  '[translate="no"], .notranslate, [contenteditable="true"], .bn-container, .ProseMirror, .kommentar-text, script, style, textarea, code, pre';

function gespeichert(): Sprache | null {
  try {
    const w = localStorage.getItem(SCHLUESSEL);
    return w === "de" || w === "en" ? w : null;
  } catch {
    return null;
  }
}

function ausDemBrowser(): Sprache {
  const liste = navigator.languages?.length ? navigator.languages : [navigator.language];
  return liste.some((l) => l?.toLowerCase().startsWith("de")) ? "de" : "en";
}

let aktiv: Sprache = gespeichert() ?? ausDemBrowser();

// ---------------------------------------------------------------------------
// The lookup: one map per target language, plus patterns for texts with {0}.

type Muster = { re: RegExp; ziel: string };
const tabellen: Record<Sprache, Map<string, string>> = { de: new Map(), en: new Map() };
const muster: Record<Sprache, Muster[]> = { de: [], en: [] };

const normal = (s: string) => s.replace(/\s+/g, " ").trim();
const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

function musterAus(quelle: string, ziel: string): Muster {
  const teile = quelle.split(/(\{\d\})/);
  const reihenfolge: number[] = [];
  const re = teile
    .map((t) => {
      const m = /^\{(\d)\}$/.exec(t);
      if (!m) return escape(t);
      reihenfolge.push(Number(m[1]));
      return "(.+?)";
    })
    .join("");
  // The target refers to the values by their number, the regex by position.
  const umgestellt = ziel.replace(/\{(\d)\}/g, (_, n) => `{${reihenfolge.indexOf(Number(n))}}`);
  return { re: new RegExp(`^${re}$`, "s"), ziel: umgestellt };
}

for (const [de, en] of PAARE) {
  if (/\{\d\}/.test(de)) {
    muster.de.push(musterAus(en, de));
    muster.en.push(musterAus(de, en));
  } else {
    tabellen.de.set(normal(en), de);
    tabellen.en.set(normal(de), en);
  }
}

/** The text in the chosen language, or null when the list does not know it. */
export function uebersetze(text: string, sprache: Sprache = aktiv): string | null {
  const kern = normal(text);
  if (!kern || !/[A-Za-zÄÖÜäöü]/.test(kern)) return null;
  let ziel = tabellen[sprache].get(kern);
  if (ziel === undefined) {
    for (const m of muster[sprache]) {
      const treffer = m.re.exec(kern);
      // A value is a number, a name or a short phrase. Without this limit
      // "{0} auf {1}" (Firefox auf Linux) would swallow every German sentence
      // that happens to contain the word "auf".
      if (treffer && treffer.slice(1).every((w) => (w.match(/ /g)?.length ?? 0) <= 3)) {
        // A value may itself be a word the list knows ("Create 3 accounts" →
        // "3 Konten anlegen"). Only lowercase single words, though: those are
        // units. Anything else is a name somebody gave -- a group called
        // "Vertrieb" must not come out as "Sales".
        ziel = m.ziel.replace(/\{(\d)\}/g, (_, i) => {
          const wert = treffer[Number(i) + 1] ?? "";
          return /^[a-zäöüß]+$/.test(wert) ? tabellen[sprache].get(wert) ?? wert : wert;
        });
        break;
      }
    }
  }
  if (ziel === undefined || ziel === kern) return null;
  // Keep the whitespace around the text: JSX puts spaces into separate nodes.
  const vorne = text.match(/^\s*/)![0];
  const hinten = text.match(/\s*$/)![0];
  return vorne + ziel + hinten;
}

// ---------------------------------------------------------------------------
// The DOM side. For every node the original text is remembered, so that a
// switch of language starts from what React wrote and not from a translation.

const texte = new WeakMap<Text, { orig: string; gesetzt: string }>();
const attrs = new WeakMap<Element, Map<string, { orig: string; gesetzt: string }>>();

function ausgenommen(el: Element | null): boolean {
  return !el || el.closest(AUSGENOMMEN) !== null;
}

function textNode(n: Text) {
  const wert = n.nodeValue ?? "";
  const merk = texte.get(n);
  if (merk && wert === merk.gesetzt) return; // our own write coming back
  if (ausgenommen(n.parentElement)) return;
  const neu = uebersetze(wert);
  if (neu === null) {
    if (merk) texte.delete(n);
    return;
  }
  texte.set(n, { orig: wert, gesetzt: neu });
  n.nodeValue = neu;
}

function attribut(el: Element, name: string) {
  const wert = el.getAttribute(name);
  if (wert === null) return;
  let karte = attrs.get(el);
  const merk = karte?.get(name);
  if (merk && wert === merk.gesetzt) return;
  if (ausgenommen(el)) return;
  const neu = uebersetze(wert);
  if (neu === null) {
    karte?.delete(name);
    return;
  }
  if (!karte) attrs.set(el, (karte = new Map()));
  karte.set(name, { orig: wert, gesetzt: neu });
  el.setAttribute(name, neu);
}

function baum(wurzel: Node) {
  if (wurzel.nodeType === TEXT_NODE) return textNode(wurzel as Text);
  if (!(wurzel instanceof Element)) return;
  if (ausgenommen(wurzel)) return;
  const gang = document.createTreeWalker(wurzel, SHOW_TEXT);
  for (let n = gang.nextNode(); n; n = gang.nextNode()) textNode(n as Text);
  const sel = ATTRIBUTE.map((a) => `[${a}]`).join(",");
  for (const a of ATTRIBUTE) if (wurzel.hasAttribute(a)) attribut(wurzel, a);
  wurzel.querySelectorAll(sel).forEach((el) => ATTRIBUTE.forEach((a) => attribut(el, a)));
}

/** Put every remembered node back to its original, so the next pass starts clean. */
function zurueck(wurzel: Element) {
  const gang = document.createTreeWalker(wurzel, SHOW_TEXT);
  for (let n = gang.nextNode(); n; n = gang.nextNode()) {
    const merk = texte.get(n as Text);
    if (merk) {
      texte.delete(n as Text);
      n.nodeValue = merk.orig;
    }
  }
  wurzel.querySelectorAll("*").forEach((el) => {
    const karte = attrs.get(el);
    if (!karte) return;
    karte.forEach((merk, name) => el.setAttribute(name, merk.orig));
    attrs.delete(el);
  });
}

// The numeric DOM constants instead of NodeFilter/Node: a test environment that
// imitates a browser (jsdom in the editor probe) does not always provide them.
const SHOW_TEXT = 4;
const TEXT_NODE = 3;

let beobachter: MutationObserver | null = null;

function starten() {
  document.documentElement.lang = aktiv;
  baum(document.documentElement);
  beobachter = new MutationObserver((liste) => {
    for (const m of liste) {
      if (m.type === "characterData") textNode(m.target as Text);
      else if (m.type === "attributes") attribut(m.target as Element, m.attributeName!);
      else m.addedNodes.forEach(baum);
    }
  });
  beobachter.observe(document.documentElement, {
    subtree: true,
    childList: true,
    characterData: true,
    attributes: true,
    attributeFilter: ATTRIBUTE,
  });
}

// ---------------------------------------------------------------------------
// The choice.

const zuhoerer = new Set<() => void>();

export function sprache(): Sprache {
  return aktiv;
}

export function setzeSprache(neu: Sprache) {
  if (neu === aktiv) return;
  try {
    localStorage.setItem(SCHLUESSEL, neu);
  } catch {
    /* private mode: the choice holds for this tab */
  }
  beobachter?.disconnect();
  zurueck(document.documentElement);
  aktiv = neu;
  starten();
  zuhoerer.forEach((f) => f());
}

/** The current language, re-rendering the component when it changes. */
export function useSprache(): Sprache {
  return useSyncExternalStore(
    (f) => {
      zuhoerer.add(f);
      return () => zuhoerer.delete(f);
    },
    () => aktiv,
  );
}

// Only where there is a document to watch; a probe may load this module without one.
if (typeof document !== "undefined" && typeof MutationObserver !== "undefined") starten();
