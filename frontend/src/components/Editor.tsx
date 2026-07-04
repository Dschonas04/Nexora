import { useEffect } from "react";
import { useCreateBlockNote } from "@blocknote/react";
import { BlockNoteView } from "@blocknote/mantine";
import "@blocknote/mantine/style.css";
import { locales } from "@blocknote/core";
import type { Block, BlockNoteEditor, PartialBlock } from "@blocknote/core";

export default function Editor({
  initialContent,
  editable = true,
  onChange,
  onEditorReady,
}: {
  initialContent: unknown;
  editable?: boolean;
  onChange?: (blocks: Block[]) => void;
  onEditorReady?: (editor: BlockNoteEditor) => void;
}) {
  // BlockNote rejects an empty array as initialContent — use undefined instead.
  const content =
    Array.isArray(initialContent) && initialContent.length > 0
      ? (initialContent as PartialBlock[])
      : undefined;

  const editor = useCreateBlockNote({ initialContent: content, dictionary: locales.de });

  useEffect(() => {
    onEditorReady?.(editor);
  }, [editor, onEditorReady]);

  return (
    <BlockNoteView
      editor={editor}
      editable={editable}
      theme="light"
      onChange={() => onChange?.(editor.document)}
    />
  );
}
