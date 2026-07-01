import { useCreateBlockNote } from "@blocknote/react";
import { BlockNoteView } from "@blocknote/mantine";
import "@blocknote/mantine/style.css";
import type { Block, PartialBlock } from "@blocknote/core";

export default function Editor({
  initialContent,
  editable = true,
  onChange,
}: {
  initialContent: unknown;
  editable?: boolean;
  onChange?: (blocks: Block[]) => void;
}) {
  // BlockNote rejects an empty array as initialContent — use undefined instead.
  const content =
    Array.isArray(initialContent) && initialContent.length > 0
      ? (initialContent as PartialBlock[])
      : undefined;

  const editor = useCreateBlockNote({ initialContent: content });

  return (
    <BlockNoteView
      editor={editor}
      editable={editable}
      theme="light"
      onChange={() => onChange?.(editor.document)}
    />
  );
}
