// The block editor, a thin wrapper around BlockNote. Everything specific to
// Nexora sits around it: [[wiki-links]] as clickable text, and an "@" menu that
// inserts one. BlockNote itself knows nothing about either.
import { useEffect, useRef } from "react";
import {
  DefaultReactSuggestionItem,
  SuggestionMenuController,
  useCreateBlockNote,
} from "@blocknote/react";
import { BlockNoteView } from "@blocknote/mantine";
import "@blocknote/mantine/style.css";
// Seit 0.5x liegen die Sprachdateien in einem eigenen Einstieg, und jede
// Sprache steht einzeln da statt in einem Sammelobjekt.
import { de as deutsch } from "@blocknote/core/locales";
import { withCollaboration } from "@blocknote/core/yjs";
import type { Block, BlockNoteEditor, PartialBlock } from "@blocknote/core";
import type { XmlFragment } from "yjs";
import { Extension, InputRule } from "@tiptap/core";
import { Plugin } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";

import { useDesign } from "../design";

// Shared pattern for [[Page title]]. It carries the g flag, so lastIndex has to
// be reset before each use; see onClickCapture below.
const WIKI_RE = /\[\[([^[\]]+)\]\]/g;

// `geradegeruecktes` repairs content before it reaches the editor.
//
// BlockNote reads a text block's styles using Object.entries. If the styles
// field is missing or null, that throws and the editor rejects the entire
// document instead of just the broken piece — resulting in an empty page.
// Older imports can contain such malformed blocks still stored in the DB,
// so we must repair them on read.
//
// The function deliberately returns a copy: repairing in-place would mutate
// the caller's state.
function geradegeruecktes(wert: unknown): unknown {
  if (Array.isArray(wert)) return wert.map(geradegeruecktes);
  if (wert === null || typeof wert !== "object") return wert;
  const alt = wert as Record<string, unknown>;
  const neu: Record<string, unknown> = {};
  for (const [schluessel, inhalt] of Object.entries(alt)) {
    neu[schluessel] = geradegeruecktes(inhalt);
  }
  if (
    alt.type === "text" &&
    (neu.styles === undefined || neu.styles === null)
  ) {
    neu.styles = {};
  }
  return neu;
}

// caretInfo returns the text node and character offset under a screen point,
// bridging two different browser APIs. Used to detect whether a click landed
// inside a [[wiki-link]] token.
function caretInfo(
  x: number,
  y: number,
): { node: Node; offset: number } | null {
  const doc = document as Document & {
    caretRangeFromPoint?: (x: number, y: number) => Range | null;
    caretPositionFromPoint?: (
      x: number,
      y: number,
    ) => { offsetNode: Node; offset: number } | null;
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
// Wiki-links are stored as plain text, which makes them resilient to target
// renames but also indistinguishable from normal text. This extension draws
// decorations over occurrences without modifying the text itself. The
// decoration style depends on whether the link resolves to an existing page.
function verweisErweiterung(
  aufloesen: () => ((titel: string) => string | null) | undefined,
) {
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
                  deko.push(
                    Decoration.inline(von, von + 2, {
                      class: "verweis-klammer",
                    }),
                  );
                  deko.push(
                    Decoration.inline(von + 2, bis - 2, {
                      class: kennung ? "verweis" : "verweis tot",
                    }),
                  );
                  deko.push(
                    Decoration.inline(bis - 2, bis, {
                      class: "verweis-klammer",
                    }),
                  );
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
// The editor has separate input rules for bold and italic, so combined
// sequences like "***both***" would not match either. That meant exports
// containing combined emphasis could be rendered incorrectly on re-import.
//
// These custom input rules detect combined emphasis spellings and apply both
// marks atomically, preserving round-trip behavior.
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

// Ausgefuehrt, damit die kopflose Probe denselben Editor bauen kann wie die
// Anwendung. Wer die Regel nur hier drin haette, pruefte in der Probe einen
// anderen Editor als den, den ein Benutzer vor sich hat.
export const fettKursivErweiterung = Extension.create({
  name: "fettKursiv",
  // Ahead of the built in rules for bold and italic. Otherwise the bold rule
  // grabs "**_both_**" first and what is left standing is a bold "_both_".
  priority: 200,
  addInputRules() {
    return BEIDES_MUSTER.map(beidesRegel);
  },
});

// `siehtNachMarkdownAus` decides whether pasted text appears to be Markdown.
//
// This is heuristic: a pasted log line should not be treated as Markdown,
// so the check looks for constructs unlikely to occur by accident (heading
// markers, bullets, code fences, links, emphasis patterns).
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
  mitschrift,
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
  // Collaborative editing. When present the text comes from the shared
  // document rather than `initialContent`: the page is merged character-by-
  // character instead of being saved and reloaded as a whole.
  mitschrift?: {
    fragment: XmlFragment;
    provider: unknown;
    user: { name: string; color: string };
  };
}) {
  const { design } = useDesign();
  const grundton = design.grundton;

  // BlockNote rejects an empty array as initialContent — use undefined instead.
  // Everything that does arrive is straightened out first: documents written by
  // an older import are missing the styles on their text pieces, and BlockNote
  // refuses the entire document over that.
  //
  // When collaborating the initial content should be empty: the shared
  // document contains the authoritative content and any provided initial
  // content would be duplicated across browsers.
  const content =
    !mitschrift && Array.isArray(initialContent) && initialContent.length > 0
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

  const grundeinstellungen = {
    initialContent: content,
    dictionary: deutsch,
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
      extensions: [
        verweisErweiterung(() => loeserRef.current),
        fettKursivErweiterung,
      ],
    },
  };

  // Gemeinsames Schreiben geht seit 0.5x nicht mehr als Einstellung mit,
  // sondern durch withCollaboration: die Erweiterung wird um die uebrigen
  // Einstellungen herumgelegt. Ohne Mitschrift bleibt sie ganz weg, sonst
  // haengte an jeder allein geoeffneten Seite ein Yjs-Dokument ohne Gegenüber.
  //
  // Die Umgehung des Typs bleibt: Nexora fuehrt Yjs 13, die Typen von
  // BlockNote nennen die Fassung 14. Am Draht sprechen beide dasselbe, und
  // y-prosemirror haelt beide Seiten aus -- deshalb ist es hier eine Frage der
  // Typen und keine des Verhaltens.
  const einstellungen: typeof grundeinstellungen = mitschrift
    ? (withCollaboration({
        ...grundeinstellungen,
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        collaboration: mitschrift as any,
      }) as typeof grundeinstellungen)
    : grundeinstellungen;
  const editor = useCreateBlockNote(einstellungen);
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
        Array.isArray(hier.content) &&
        hier.content.length === 0 &&
        hier.type === "paragraph";
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
