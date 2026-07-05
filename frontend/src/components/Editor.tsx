import { useEffect, useRef } from "react";
import { DefaultReactSuggestionItem, SuggestionMenuController, useCreateBlockNote } from "@blocknote/react";
import { BlockNoteView } from "@blocknote/mantine";
import "@blocknote/mantine/style.css";
import { locales } from "@blocknote/core";
import type { Block, BlockNoteEditor, PartialBlock } from "@blocknote/core";

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

  const editor = useCreateBlockNote({ initialContent: content, dictionary: locales.de });
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
  const onClickCapture = (e: React.MouseEvent) => {
    if (!linkResolver || !onOpenLink) return;
    const ci = caretInfo(e.clientX, e.clientY);
    if (!ci || ci.node.nodeType !== Node.TEXT_NODE) return;
    const text = ci.node.textContent || "";
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
