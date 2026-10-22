// The table of contents: the headings of the open page, as a list beside it.
//
// Read from the editor and not from the saved page, so it follows while one
// types -- a heading written a moment ago is in the list at once. Nothing is
// stored: the headings are the table of contents.
import { useEffect, useState } from "react";
import type { BlockNoteEditor } from "@blocknote/core";

export type Ueberschrift = { id: string; stufe: number; text: string };

function inline(inhalt: unknown): string {
  if (!Array.isArray(inhalt)) return "";
  return inhalt
    .map((t) => {
      const o = t as { text?: string; content?: unknown };
      return (typeof o.text === "string" ? o.text : "") + (o.content ? inline(o.content) : "");
    })
    .join("");
}

/** The headings of a document in reading order, nested ones included. */
export function ueberschriften(dokument: unknown): Ueberschrift[] {
  const raus: Ueberschrift[] = [];
  const gehe = (liste: unknown) => {
    if (!Array.isArray(liste)) return;
    for (const roh of liste) {
      const b = roh as { id: string; type: string; props?: { level?: number }; content?: unknown; children?: unknown };
      if (b.type === "heading") {
        const text = inline(b.content).trim();
        if (text) raus.push({ id: b.id, stufe: Number(b.props?.level) || 1, text });
      }
      gehe(b.children);
    }
  };
  gehe(dokument);
  return raus;
}

/** The headings, kept current while the editor changes. */
export function useUeberschriften(editor: BlockNoteEditor | null): Ueberschrift[] {
  const [liste, setListe] = useState<Ueberschrift[]>([]);
  useEffect(() => {
    if (!editor) {
      setListe([]);
      return;
    }
    let uhr = 0;
    const lesen = () => setListe(ueberschriften(editor.document));
    lesen();
    // Not on every keystroke: once the typing pauses for a moment.
    const weg = editor.onChange(() => {
      window.clearTimeout(uhr);
      uhr = window.setTimeout(lesen, 300);
    });
    return () => {
      window.clearTimeout(uhr);
      if (typeof weg === "function") weg();
    };
  }, [editor]);
  return liste;
}

export default function Inhaltsverzeichnis({
  liste,
  onClose,
}: {
  liste: Ueberschrift[];
  onClose: () => void;
}) {
  // The levels used on the page start at the left edge, whichever they are: a
  // page that only uses H2 and H3 should not be indented by one step for
  // nothing.
  const oben = Math.min(...liste.map((u) => u.stufe), 3);
  const springen = (id: string) => {
    const ziel = document.querySelector(`.bn-container [data-id="${CSS.escape(id)}"]`);
    ziel?.scrollIntoView({ behavior: "smooth", block: "start" });
  };
  return (
    <nav className="side-panel inhaltsverzeichnis" aria-label="Contents">
      <div className="side-panel-header">
        <h3>Contents</h3>
        <button className="icon-btn" onClick={onClose} aria-label="Close">
          ✕
        </button>
      </div>
      <div className="side-panel-body">
        {liste.length === 0 && (
          <div className="muted small">No headings on this page yet. Type # and a space to make one.</div>
        )}
        {liste.map((u) => (
          <button
            key={u.id}
            className="inhalt-eintrag notranslate"
            style={{ paddingLeft: 6 + (u.stufe - oben) * 14 }}
            onClick={() => springen(u.id)}
          >
            {u.text}
          </button>
        ))}
      </div>
    </nav>
  );
}
