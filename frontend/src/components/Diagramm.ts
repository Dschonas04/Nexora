// Diagrams: a code block in the language "mermaid" gets its picture drawn
// right below it.
//
// Deliberately not a block type of its own. A code block survives every way a
// page travels -- the Markdown export writes it as ```mermaid, the import reads
// it back, PDF and Word show the source -- where a new block type would be
// lost on the first round trip, and every other Mermaid tool understands the
// text as it stands.
//
// The picture is a widget decoration after the block: ProseMirror draws it but
// it is not part of the document, so nothing of it is saved or sent to the
// people writing along. Mermaid itself is large and loaded only when the first
// diagram appears.
import { Extension } from "@tiptap/core";
import { Plugin } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";

type Mermaid = typeof import("mermaid").default;

let laden: Promise<Mermaid> | null = null;
let dunkel = false;
let eingerichtetDunkel: boolean | null = null;
let zaehler = 0;
// Finished pictures by theme and source text. Typing elsewhere on the page
// redraws the decorations, and without the cache every diagram would be
// rendered anew each time.
const fertig = new Map<string, { svg?: string; fehler?: string }>();

/** Called by the editor with the account's base tone. */
export function setzeDiagrammThema(istDunkel: boolean) {
  dunkel = istDunkel;
}

async function mermaidHolen(): Promise<Mermaid> {
  if (!laden) laden = import("mermaid").then((m) => m.default);
  const m = await laden;
  if (eingerichtetDunkel !== dunkel) {
    // "strict": no click handlers, no HTML labels -- the source text is page
    // content anybody with write access may have typed.
    m.initialize({ startOnLoad: false, securityLevel: "strict", theme: dunkel ? "dark" : "default" });
    eingerichtetDunkel = dunkel;
  }
  return m;
}

function zeichnen(ziel: HTMLElement, code: string) {
  const schluessel = (dunkel ? "d:" : "h:") + code;
  const zeigen = (e: { svg?: string; fehler?: string }) => {
    if (e.svg !== undefined) {
      ziel.classList.remove("fehler");
      // As an image and not as markup in the page: whatever the source text
      // contains, an SVG inside <img> runs no script and reaches no cookie.
      // Mermaid's own sanitising in "strict" mode is the first fence, this
      // is the second.
      const bild = document.createElement("img");
      bild.alt = "Diagramm";
      bild.src = "data:image/svg+xml;charset=utf-8," + encodeURIComponent(e.svg);
      ziel.replaceChildren(bild);
    } else {
      ziel.classList.add("fehler");
      ziel.textContent = e.fehler ?? "";
    }
  };
  const da = fertig.get(schluessel);
  if (da) return zeigen(da);
  if (!code.trim()) return;
  ziel.textContent = "…";
  void (async () => {
    let e: { svg?: string; fehler?: string };
    try {
      const m = await mermaidHolen();
      const { svg } = await m.render(`nexora-diagramm-${++zaehler}`, code);
      e = { svg };
    } catch (f) {
      // The first line of Mermaid's message says what is wrong and where; the
      // rest is a dump of its parser state.
      e = { fehler: String((f as Error)?.message ?? f).split("\n")[0] };
    }
    if (fertig.size > 200) fertig.clear();
    fertig.set(schluessel, e);
    zeigen(e);
  })();
}

export const diagrammErweiterung = Extension.create({
  name: "nexoraDiagramme",
  addProseMirrorPlugins() {
    return [
      new Plugin({
        props: {
          decorations(zustand) {
            const deko: Decoration[] = [];
            zustand.doc.descendants((knoten, pos) => {
              if (knoten.type.name !== "codeBlock") return;
              if (String(knoten.attrs.language ?? "").toLowerCase() !== "mermaid") return false;
              const code = knoten.textContent;
              deko.push(
                Decoration.widget(
                  pos + knoten.nodeSize,
                  () => {
                    const div = document.createElement("div");
                    div.className = "diagramm-vorschau notranslate";
                    div.contentEditable = "false";
                    zeichnen(div, code);
                    return div;
                  },
                  // The key decides whether ProseMirror keeps the drawn
                  // picture: same text and theme, same picture.
                  { side: 1, key: `diagramm:${dunkel ? "d" : "h"}:${code}`, ignoreSelection: true },
                ),
              );
              return false;
            });
            return DecorationSet.create(zustand.doc, deko);
          },
        },
      }),
    ];
  },
});
