// The block editor, a thin wrapper around BlockNote. Everything specific to
// Nexora sits around it: [[wiki-links]] as clickable text, and an "@" menu that
// inserts one. BlockNote itself knows nothing about either.
import { useEffect, useRef } from "react";
import { DefaultReactSuggestionItem, SuggestionMenuController, useCreateBlockNote } from "@blocknote/react";
import { BlockNoteView } from "@blocknote/mantine";
import "@blocknote/mantine/style.css";
import { locales } from "@blocknote/core";
import type { Block, BlockNoteEditor, PartialBlock } from "@blocknote/core";
import { Extension } from "@tiptap/core";
import { Plugin } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";

// Shared pattern for [[Page title]]. It carries the g flag, so lastIndex has to
// be reset before each use; see onClickCapture below.
const WIKI_RE = /\[\[([^[\]]+)\]\]/g;

// caretInfo returns the text node and character offset under a screen point,
// bridging the two browser APIs. Used to tell whether a click landed inside a
// [[wiki-link]] token.
function caretInfo(x: number, y: number): { node: Node; offset: number } | null {
  const doc = document as Document & {
    caretRangeFromPoint?: (x: number, y: number) => Range | null;
    caretPositionFromPoint?: (x: number, y: number) => { offsetNode: Node; offset: number } | null;
  };
  if (doc.caretRangeFromPoint) {
    const r = doc.caretRangeFromPoint(x, y);
    if (r) return { node: r.startContainer, offset: r.startOffset };
  } else if (doc.caretPositionFromPoint) {
    const r = doc.caretPositionFromPoint(x, y);
    if (r) return { node: r.offsetNode, offset: r.offset };
  }
  return null;
}

// Hebt [[Verweise]] im Text hervor.
//
// Ein Verweis ist gewöhnlicher Text -- gerade das macht ihn haltbar, denn er
// überlebt jede Umbenennung der Zielseite. Nur sah er dadurch auch aus wie
// gewöhnlicher Text: die Klammern standen roh da, und dass man darauf klicken
// kann, sah man ihm nicht an.
//
// Also legt diese Erweiterung eine Auszeichnung über die Fundstellen, ohne den
// Text selbst anzurühren. Wer den Verweis auflöst, entscheidet die Seite: gibt
// es das Ziel, wird der Titel als Verweis gezeichnet, sonst bleibt er blass und
// unterstrichen -- ein Hinweis auf einen Titel, den es (noch) nicht gibt.
function verweisErweiterung(aufloesen: () => ((titel: string) => string | null) | undefined) {
  return Extension.create({
    name: "nexoraVerweise",
    addProseMirrorPlugins() {
      return [
        new Plugin({
          props: {
            decorations(zustand) {
              const loeser = aufloesen();
              if (!loeser) return DecorationSet.empty;
              const deko: Decoration[] = [];
              zustand.doc.descendants((knoten, pos) => {
                if (!knoten.isText || !knoten.text) return;
                const text = knoten.text;
                WIKI_RE.lastIndex = 0;
                let m: RegExpExecArray | null;
                while ((m = WIKI_RE.exec(text))) {
                  const von = pos + m.index;
                  const bis = von + m[0].length;
                  const kennung = loeser(m[1].trim());
                  deko.push(Decoration.inline(von, von + 2, { class: "verweis-klammer" }));
                  deko.push(
                    Decoration.inline(von + 2, bis - 2, {
                      class: kennung ? "verweis" : "verweis tot",
                    }),
                  );
                  deko.push(Decoration.inline(bis - 2, bis, { class: "verweis-klammer" }));
                }
              });
              return DecorationSet.create(zustand.doc, deko);
            },
          },
        }),
      ];
    },
  });
}

export default function Editor({
  initialContent,
  editable = true,
  onChange,
  onEditorReady,
  linkResolver,
  onOpenLink,
  mentionTargets,
}: {
  initialContent: unknown;
  editable?: boolean;
  onChange?: (blocks: Block[]) => void;
  onEditorReady?: (editor: BlockNoteEditor) => void;
  // Resolve a [[Title]] to a page id (or null); when both are given, clicking a
  // wiki-link in the text opens that page.
  linkResolver?: (title: string) => string | null;
  onOpenLink?: (id: string) => void;
  // Pages selectable via an "@" mention in running text. Picking one inserts a
  // [[Title]] wiki-link, so it feeds the graph/backlinks like any other link.
  mentionTargets?: { id: string; title: string }[];
}) {
  // BlockNote rejects an empty array as initialContent — use undefined instead.
  const content =
    Array.isArray(initialContent) && initialContent.length > 0
      ? (initialContent as PartialBlock[])
      : undefined;

  // The editor is created once. Its content is not reactive, which is why the
  // page view remounts this component with a new key when it changes the
  // document from the outside.
  // Über eine Referenz, nicht über den Wert: der Editor wird einmal gebaut, die
  // Liste der Seiten kommt aber nach und ändert sich weiter. Die Erweiterung
  // fragt deshalb bei jedem Zeichnen neu nach.
  const loeserRef = useRef(linkResolver);
  loeserRef.current = linkResolver;

  const editor = useCreateBlockNote({
    initialContent: content,
    dictionary: locales.de,
    _tiptapOptions: { extensions: [verweisErweiterung(() => loeserRef.current)] },
  });
  const wrapRef = useRef<HTMLDivElement>(null);

  // Read the latest page list on each "@" trigger without recreating the editor.
  const targetsRef = useRef(mentionTargets ?? []);
  targetsRef.current = mentionTargets ?? [];

  useEffect(() => {
    onEditorReady?.(editor);
  }, [editor, onEditorReady]);

  // Items shown in the "@" mention menu: filter pages by the typed query and,
  // on selection, insert a [[Title]] wiki-link at the cursor.
  const mentionItems = (query: string): DefaultReactSuggestionItem[] => {
    const q = query.toLowerCase().trim();
    return targetsRef.current
      .filter((t) => (t.title || "").toLowerCase().includes(q))
      .slice(0, 12)
      .map((t) => ({
        title: t.title || "Ohne Titel",
        onItemClick: () => editor.insertInlineContent(`[[${t.title}]] `),
      }));
  };

  // Navigate when a [[wiki-link]] token is clicked. Runs in the capture phase
  // and stops propagation so BlockNote doesn't just place the caret there.
  // Turn a click on a [[wiki-link]] into navigation. There is no link element to
  // hang a handler on, because the token is plain text, so the character under
  // the pointer is resolved instead and matched against the pattern.
  //
  // A click that is not inside a token, or points at a title that does not
  // exist, falls through and behaves like an ordinary click in the text.
  const onClickCapture = (e: React.MouseEvent) => {
    if (!linkResolver || !onOpenLink) return;

    // Der schnelle Weg: die Auszeichnung oben hat den Titel bereits in eine
    // eigene Umhüllung gelegt, ihr Text ist der Titel. Er muss zuerst kommen,
    // denn genau diese Umhüllung teilt den Text im Dokument in drei Stücke --
    // die Suche unten fände in "Ziel" keine Klammern mehr.
    const umhuellung = (e.target as HTMLElement | null)?.closest?.(".verweis");
    if (umhuellung) {
      const id = linkResolver((umhuellung.textContent || "").trim());
      if (id) {
        e.preventDefault();
        e.stopPropagation();
        onOpenLink(id);
      }
      return;
    }

    // Der Rückfallweg für Text ohne Auszeichnung: die Stelle unter dem Zeiger
    // im Absatz suchen.
    const ci = caretInfo(e.clientX, e.clientY);
    if (!ci || ci.node.nodeType !== Node.TEXT_NODE) return;
    const text = ci.node.textContent || "";
    // Reset because the pattern is global and shared: a leftover lastIndex from
    // an earlier click would make the search start in the middle of the line.
    WIKI_RE.lastIndex = 0;
    let m: RegExpExecArray | null;
    while ((m = WIKI_RE.exec(text))) {
      if (ci.offset >= m.index && ci.offset <= m.index + m[0].length) {
        const id = linkResolver(m[1].trim());
        if (id) {
          e.preventDefault();
          e.stopPropagation();
          onOpenLink(id);
        }
        return;
      }
    }
  };

  return (
    <div ref={wrapRef} className={linkResolver ? "editor-wraps has-wikilinks" : "editor-wraps"} onClickCapture={onClickCapture}>
      <BlockNoteView
        editor={editor}
        editable={editable}
        theme="light"
        onChange={() => onChange?.(editor.document)}
      >
        {mentionTargets && (
          <SuggestionMenuController
            triggerCharacter="@"
            getItems={async (query) => mentionItems(query)}
          />
        )}
      </BlockNoteView>
    </div>
  );
}
