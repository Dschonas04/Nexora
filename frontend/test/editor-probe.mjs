// Greifen die Eingaberegeln noch?
//
// "**fett**" wird beim Tippen zu fettem Text -- das entscheidet keine
// Typprüfung, sondern ProseMirror zur Laufzeit im Browser. Diese Probe stellt
// einen Browser nach (jsdom), hängt einen echten Editor hinein und tippt
// Zeichen für Zeichen, so wie ein Mensch es täte.
//
// Aufruf: node test/editor-probe.mjs <gebündelte editorprobe-eingang.mjs>
import { JSDOM } from "jsdom";

const fenster = new JSDOM("<!doctype html><html><body><div id=e></div></body></html>", {
  pretendToBeVisual: true,
  url: "http://localhost/",
});

// Vor dem Laden des Bündels: BlockNote sieht beim Einlesen nach, wo es ist.
globalThis.window = fenster.window;
globalThis.document = fenster.window.document;
Object.defineProperty(globalThis, "navigator", {
  value: fenster.window.navigator,
  configurable: true,
});
globalThis.HTMLElement = fenster.window.HTMLElement;
globalThis.Element = fenster.window.Element;
globalThis.Node = fenster.window.Node;
globalThis.DOMParser = fenster.window.DOMParser;
globalThis.MutationObserver = fenster.window.MutationObserver;
globalThis.getComputedStyle = fenster.window.getComputedStyle;
globalThis.ShadowRoot = fenster.window.ShadowRoot;
globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);
globalThis.cancelAnimationFrame = (h) => clearTimeout(h);

const pfad = process.argv[2];
if (!pfad) {
  console.error("Aufruf: node test/editor-probe.mjs <buendel.mjs>");
  process.exit(2);
}
const { BlockNoteEditor, deutsch, fettKursivErweiterung } = await import(pfad);

let fehler = 0;
const melde = (ok, was) => {
  console.log((ok ? "  ok    " : "  FEHLT ") + was);
  if (!ok) fehler++;
};

const editor = BlockNoteEditor.create({
  dictionary: deutsch,
  _tiptapOptions: { extensions: [fettKursivErweiterung] },
});
editor.mount(fenster.window.document.getElementById("e"));

// Das Einhaengen laeuft ueber einen Mikroschritt: gleich danach steht die
// Sicht noch nicht. Ein Wimpernschlag genuegt.
await new Promise((weiter) => setTimeout(weiter, 200));

const sicht = editor.prosemirrorView;
melde(!!sicht, "der Editor haengt in der Seite");

// Tippen heisst: jedes Zeichen einzeln durch dieselbe Tuer schicken, durch die
// auch eine Tastatur kommt. Genau dort sitzen die Eingaberegeln; wer den Text
// stattdessen in das Dokument schreibt, umgeht sie und prueft nichts.
function tippe(text) {
  for (const zeichen of text) {
    const von = sicht.state.selection.from;
    const behandelt = sicht.someProp("handleTextInput", (f) => f(sicht, von, von, zeichen));
    if (!behandelt) {
      sicht.dispatch(sicht.state.tr.insertText(zeichen, von, sicht.state.selection.to));
    }
  }
}

// Welche Auszeichnungen traegt der Text im ersten Block?
function stileVon(bloecke) {
  const teile = bloecke[0]?.content ?? [];
  return teile.map((t) => ({ text: t.text, ...(t.styles ?? {}) }));
}

console.log("== Beim Tippen ausgezeichnet");

tippe("**fett** danach");
let teile = stileVon(editor.document);
melde(
  teile.some((t) => t.bold && t.text.trim() === "fett"),
  'aus "**fett**" wird fetter Text',
);
melde(
  !teile.some((t) => (t.text ?? "").includes("**")),
  "die Sternchen bleiben nicht stehen",
);
melde(
  teile.some((t) => !t.bold && (t.text ?? "").includes("danach")),
  "der Text danach bleibt normal",
);

editor.replaceBlocks(editor.document, [{ type: "paragraph", content: [] }]);
editor.setTextCursorPosition(editor.document[0], "end");
tippe("*schraeg* danach");
teile = stileVon(editor.document);
melde(
  teile.some((t) => t.italic && t.text.trim() === "schraeg"),
  'aus "*schraeg*" wird schraeger Text',
);

editor.replaceBlocks(editor.document, [{ type: "paragraph", content: [] }]);
editor.setTextCursorPosition(editor.document[0], "end");
tippe("***beides*** danach");
teile = stileVon(editor.document);
melde(
  teile.some((t) => t.bold && t.italic && t.text.trim() === "beides"),
  'aus "***beides***" wird beides zugleich',
);

editor.replaceBlocks(editor.document, [{ type: "paragraph", content: [] }]);
editor.setTextCursorPosition(editor.document[0], "end");
tippe("`code` danach");
teile = stileVon(editor.document);
melde(
  teile.some((t) => t.code && t.text.trim() === "code"),
  'aus "`code`" wird feste Schrift',
);

editor.replaceBlocks(editor.document, [{ type: "paragraph", content: [] }]);
editor.setTextCursorPosition(editor.document[0], "end");
tippe("# Ueberschrift");
melde(editor.document[0]?.type === "heading", 'aus "# " wird eine Ueberschrift');

editor.replaceBlocks(editor.document, [{ type: "paragraph", content: [] }]);
editor.setTextCursorPosition(editor.document[0], "end");
tippe("- Punkt");
melde(editor.document[0]?.type === "bulletListItem", 'aus "- " wird ein Listenpunkt');

console.log(fehler === 0 ? "\nProbe bestanden." : `\n${fehler} Punkte fehlen.`);
process.exit(fehler === 0 ? 0 : 1);
