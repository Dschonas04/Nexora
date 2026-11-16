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
import { uebersetze } from "../sprache";

type Mermaid = typeof import("mermaid").default;

let laden: Promise<Mermaid> | null = null;
let dunkel = false;
let eingerichtetDunkel: boolean | null = null;
let zaehler = 0;
// Nexora is a wiki, not a project tool: Gantt charts and timelines are not
// drawn. Comments (%%) and a front-matter block may come before the type.
const PROJEKTPLAN = /^\s*(---\n[\s\S]*?\n---\s*)?(%%[^\n]*\n\s*)*(gantt|timeline)\b/i;

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
    m.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      theme: dunkel ? "dark" : "default",
      // Mermaid's own bar and line colour is a pale lavender: a line chart on
      // white was barely there. The same series colours as the graph view.
      themeVariables: {
        xyChart: {
          plotColorPalette: dunkel
            ? "#5aa9f0, #f0a35a, #6fcf83, #c39af7, #f28b82"
            : "#2383e2, #e8833a, #2ea043, #8957e5, #cf222e",
        },
      },
    });
    eingerichtetDunkel = dunkel;
  }
  return m;
}

/**
 * Draws one diagram and returns the SVG or the first line of Mermaid's error.
 * Shared by the picture under the code block and the live preview in the
 * diagram dialog, so both show exactly the same thing and share the cache.
 */
export async function diagrammZeichnen(code: string): Promise<{ svg?: string; fehler?: string }> {
  const schluessel = (dunkel ? "d:" : "h:") + code;
  const da = fertig.get(schluessel);
  if (da) return da;
  if (!code.trim()) return { fehler: "" };
  if (PROJEKTPLAN.test(code)) {
    const fehler = "Gantt-Diagramme und Zeitleisten gibt es in Nexora nicht.";
    return { fehler: uebersetze(fehler) ?? fehler };
  }
  let e: { svg?: string; fehler?: string };
  try {
    const m = await mermaidHolen();
    // Mermaid draws into the page for a moment to measure the text. Without
    // a box of its own the interface translation sees that draft and turns
    // "Anmelden" into "Sign in" before the picture is taken.
    const ablage = document.createElement("div");
    ablage.className = "notranslate";
    ablage.setAttribute("translate", "no");
    ablage.style.cssText = "position:absolute;left:-10000px;top:0;";
    document.body.appendChild(ablage);
    let svg: string;
    try {
      ({ svg } = await m.render(`nexora-diagramm-${++zaehler}`, code, ablage));
    } finally {
      ablage.remove();
    }
    e = { svg };
  } catch (f) {
    // The whole message; the picture under the block shows only its first
    // line, the dialog shows the part with the pointer to the spot.
    e = { fehler: String((f as Error)?.message ?? f) };
  }
  if (fertig.size > 200) fertig.clear();
  fertig.set(schluessel, e);
  return e;
}

/** Puts a finished picture (or the error) into a box. */
export function diagrammZeigen(ziel: HTMLElement, e: { svg?: string; fehler?: string }) {
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
    // The first line says what is wrong and where; the rest is a dump of the
    // parser state.
    ziel.textContent = (e.fehler ?? "").split("\n")[0];
  }
}

function zeichnen(ziel: HTMLElement, code: string) {
  if (!code.trim()) return;
  const da = fertig.get((dunkel ? "d:" : "h:") + code);
  if (da) return diagrammZeigen(ziel, da);
  ziel.textContent = "…";
  void diagrammZeichnen(code).then((e) => diagrammZeigen(ziel, e));
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
                    // A click opens the diagram dialog (Editor.tsx); the
                    // editor's interface translation does not reach in here.
                    const hinweis = "Diagramm öffnen";
                    div.title = uebersetze(hinweis) ?? hinweis;
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
