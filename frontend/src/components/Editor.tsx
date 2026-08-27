// The block editor, a thin wrapper around BlockNote. Everything specific to
// Nexora sits around it: [[wiki-links]] as clickable text, and an "@" menu that
// inserts one. BlockNote itself knows nothing about either.
import { useEffect, useRef } from "react";
import { DefaultReactSuggestionItem, SuggestionMenuController, useCreateBlockNote } from "@blocknote/react";
import { BlockNoteView } from "@blocknote/mantine";
import "@blocknote/mantine/style.css";
import { locales } from "@blocknote/core";
import type { Block, BlockNoteEditor, PartialBlock } from "@blocknote/core";
import { Extension, InputRule } from "@tiptap/core";
import { Plugin } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";

import { useDesign } from "../design";

// Shared pattern for [[Page title]]. It carries the g flag, so lastIndex has to
// be reset before each use; see onClickCapture below.
const WIKI_RE = /\[\[([^[\]]+)\]\]/g;

// geradegeruecktes repairs content before it reaches the editor.
//
// BlockNote reads the styles of a piece of text with Object.entries. If the
// field is missing or null that throws, and the editor then rejects the whole
// document instead of that one piece — the page stays empty. Older imports
// wrote exactly such pieces, and they lie in the database to this day, so it is
// not enough to write them correctly from now on.
//
// Deliberately a copy: the original is state of the calling view, and repairing
// it in place would change something that view never handed over for changing.
function geradegeruecktes(wert: unknown): unknown {
  if (Array.isArray(wert)) return wert.map(geradegeruecktes);
  if (wert === null || typeof wert !== "object") return wert;
  const alt = wert as Record<string, unknown>;
  const neu: Record<string, unknown> = {};
  for (const [schluessel, inhalt] of Object.entries(alt)) {
    neu[schluessel] = geradegeruecktes(inhalt);
  }
  if (alt.type === "text" && (neu.styles === undefined || neu.styles === null)) {
    neu.styles = {};
  }
  return neu;
}

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


// Bold AND italic in one go.
//
// The editor knows "**bold**" and "*italic*" on their own, each through an input
// rule of its own. Both at once it does not know: the rules look for a run
// without further asterisks in it, so "***both***" matches neither of them and
// stays standing as the characters one typed. Written this way in every Markdown
// file, and read back that way by the import, it was the one spot where the
// editor understood less than its own export.
//
// So one rule per spelling, and each sets both marks. The stored marks are taken
// back afterwards: what follows the closing asterisks is ordinary text again.
const BEIDES_MUSTER = [
  /(?:^|\s)(\*\*\*([^*\n]+)\*\*\*)$/,
  /(?:^|\s)(___([^_\n]+)___)$/,
  /(?:^|\s)(\*\*_([^_\n]+)_\*\*)$/,
  /(?:^|\s)(_\*\*([^*\n]+)\*\*_)$/,
];

function beidesRegel(muster: RegExp) {
  return new InputRule({
    find: muster,
    handler: ({ state, range, match }) => {
      const text = match[2];
      const fett = state.schema.marks.bold;
      const kursiv = state.schema.marks.italic;
      if (!text || !fett || !kursiv) return null;
      // The pattern may have swallowed a leading space; it belongs to the text
      // and not to the styling.
      const von = range.from + (match[0].length - match[1].length);
      const tr = state.tr;
      tr.replaceWith(von, range.to, state.schema.text(text));
      tr.addMark(von, von + text.length, fett.create());
      tr.addMark(von, von + text.length, kursiv.create());
      tr.removeStoredMark(fett);
      tr.removeStoredMark(kursiv);
      // No return value: a rule returning null counts as "did not apply", and
      // the whole transaction would be dropped again.
    },
  });
}

const fettKursivErweiterung = Extension.create({
  name: "fettKursiv",
  // Ahead of the built in rules for bold and italic. Otherwise the bold rule
  // grabs "**_both_**" first and what is left standing is a bold "_both_".
  priority: 200,
  addInputRules() {
    return BEIDES_MUSTER.map(beidesRegel);
  },
});

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
  dateiHochladen,
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
  // Puts a dropped or chosen file somewhere it can be reached from and returns
  // its address. Without it the image block asks for a URL and nothing else,
  // which means an image has to live somewhere before it can be used here.
  dateiHochladen?: (datei: File) => Promise<string>;
}) {
  const { design } = useDesign();
  const grundton = design.grundton;

  // BlockNote rejects an empty array as initialContent — use undefined instead.
  // Everything that does arrive is straightened out first: documents written by
  // an older import are missing the styles on their text pieces, and BlockNote
  // refuses the entire document over that.
  const content =
    Array.isArray(initialContent) && initialContent.length > 0
      ? (geradegeruecktes(initialContent) as PartialBlock[])
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

  // Through a reference for the same reason as the link resolver: the editor is
  // built once, the page around it keeps changing.
  const ladenRef = useRef(dateiHochladen);
  ladenRef.current = dateiHochladen;

  const editor = useCreateBlockNote({
    initialContent: content,
    dictionary: locales.de,
    // The image, video and file blocks get their own upload from this. It is
    // the same path an attachment takes, so a picture in the text lies where
    // every other file of this page lies: on the disk or in the bucket, and
    // reachable only for whoever may read the page.
    uploadFile: async (datei: File) => {
      const laden = ladenRef.current;
      if (!laden) throw new Error("Hochladen ist hier nicht eingerichtet");
      return laden(datei);
    },
    _tiptapOptions: {
      extensions: [verweisErweiterung(() => loeserRef.current), fettKursivErweiterung],
    },
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
      {/* BlockNote brings its own two themes and knows nothing of the base
          tones here. Fixed on "light" it kept its dark letters on the dark
          ground -- the text of a page was then barely there, and the menus of
          the editor stood as light boxes in a dark window. */}
      <BlockNoteView
        editor={editor}
        editable={editable}
        theme={grundton === "dunkel" ? "dark" : "light"}
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
