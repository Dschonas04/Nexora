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

// Highlights [[links]] in the text.
//
// A link is ordinary text, and that is exactly what makes it durable, because it
// survives every renaming of the target page. Only it therefore also looked like
// ordinary text: the brackets stood there raw, and one could not tell it could
// be clicked.
//
// So this extension lays a decoration over the occurrences without touching the
// text itself. Whether the link resolves is decided by the page: if the target
// exists the title is drawn as a link, otherwise it stays pale and underlined, a
// hint at a title that does not exist (yet).
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


// siehtNachMarkdownAus decides whether a pasted text was meant as Markdown.
//
// The question is necessary because the answer is not always yes: whoever pastes
// a line from a log wants it as it is. So it does not look for single characters
// but for the shapes nobody types by accident: a heading at the start of a line,
// a bullet, a quote, a code fence, a link in brackets or a word between two pairs
// of asterisks.
const MARKDOWN_MUSTER = [
  /^#{1,6}\s+\S/m,
  /^\s*[-*+]\s+\S/m,
  /^\s*\d+\.\s+\S/m,
  /^\s*>\s+\S/m,
  /^```/m,
  /\*\*[^*\n]+\*\*/,
  /\[[^\]\n]+\]\([^)\n]+\)/,
];

function siehtNachMarkdownAus(text: string): boolean {
  return MARKDOWN_MUSTER.some((m) => m.test(text));
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
  //
  // Through a reference and not through the value: the editor is built once, but
  // the list of pages arrives later and keeps changing. The extension therefore
  // asks again on every draw.
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

    // The fast path: the decoration above has already put the title into a
    // wrapper of its own, and its text is the title. It has to come first,
    // because that very wrapper splits the text in the document into three
    // pieces, so the search below would find no brackets in "Ziel" any more.
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

    // The fallback path for text without decoration: look for the place under
    // the pointer inside the paragraph.
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

  // Take pasted Markdown over as Markdown.
  //
  // Without this "## Titel" would land as the four characters standing there,
  // and that is the usual way text arrives here: from a notes app, an answer, a
  // README. The editor knows the conversion already, it was only never hooked up
  // to the clipboard.
  //
  // Only plain text is touched. If HTML lies in the clipboard, BlockNote can do
  // that itself and better.
  const onPasteCapture = (e: React.ClipboardEvent) => {
    if (!editable) return;
    const zwischen = e.clipboardData;
    if (!zwischen || zwischen.getData("text/html")) return;
    const text = zwischen.getData("text/plain");
    if (!text || !siehtNachMarkdownAus(text)) return;

    e.preventDefault();
    e.stopPropagation();
    void (async () => {
      const bloecke = await editor.tryParseMarkdownToBlocks(text);
      if (bloecke.length === 0) return;
      const hier = editor.getTextCursorPosition().block;
      const leer =
        Array.isArray(hier.content) && hier.content.length === 0 && hier.type === "paragraph";
      // Into an empty paragraph instead of below it: otherwise an empty line
      // would remain above the pasted text.
      if (leer) {
        editor.replaceBlocks([hier], bloecke);
      } else {
        editor.insertBlocks(bloecke, hier, "after");
      }
      onChange?.(editor.document);
    })();
  };

  return (
    <div
      ref={wrapRef}
      className={linkResolver ? "editor-wraps has-wikilinks" : "editor-wraps"}
      onClickCapture={onClickCapture}
      onPasteCapture={onPasteCapture}
    >
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
