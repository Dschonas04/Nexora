// The choice of templates on an empty page.
//
// It stands where the writing starts, above the empty text, and only as long
// as the page is empty: whoever starts typing has decided. That way it works
// for every new page, wherever it was created -- in the sidebar, as a subpage,
// in a space -- without a second "new page" button anywhere.
import { useEffect, useState } from "react";
import type { BlockNoteEditor } from "@blocknote/core";

import { Vorlage, api } from "../api/client";
import { useSprache } from "../sprache";
import { eingebauteVorlagen } from "../vorlagen";
import { useAuth } from "../auth";
import { useRueckfrage } from "./Rueckfrage";

// Labels for the built-in ones; their titles carry a date or a colon and are
// meant for the page, not for a button.
const NAMEN: Record<string, string> = {
  besprechung: "Meeting notes",
  anleitung: "How-to",
  projekt: "Project page",
};

export type AngewandteVorlage = { title: string; icon: string; content: unknown };

/** Whether the document holds nothing yet, kept current while the editor changes. */
export function useLeer(editor: BlockNoteEditor | null, istLeer: (b: BlockNoteEditor["document"]) => boolean) {
  const [leer, setLeer] = useState(false);
  useEffect(() => {
    if (!editor) return;
    setLeer(istLeer(editor.document));
    const weg = editor.onChange(() => setLeer(istLeer(editor.document)));
    return () => {
      if (typeof weg === "function") weg();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editor]);
  return leer;
}

export default function VorlagenWahl({ onWahl }: { onWahl: (v: AngewandteVorlage) => void }) {
  const sprache = useSprache();
  const [eigene, setEigene] = useState<Vorlage[]>([]);
  const [verwalten, setVerwalten] = useState(false);

  const laden = () => api.vorlagen().then(setEigene).catch(() => setEigene([]));
  useEffect(() => {
    laden();
  }, []);

  return (
    <div className="vorlagen-wahl">
      <span className="muted small">Start from a template:</span>
      {eingebauteVorlagen(sprache).map((v) => (
        <button key={v.key} className="vorlage-chip" onClick={() => onWahl(v)}>
          {NAMEN[v.key]}
        </button>
      ))}
      {eigene.map((v) => (
        <button
          key={v.id}
          className="vorlage-chip eigen notranslate"
          title={v.own ? undefined : v.ownerName}
          onClick={() => onWahl(v)}
        >
          {v.icon ? v.icon + " " : ""}
          {v.title}
        </button>
      ))}
      {eigene.length > 0 && (
        <button className="link-btn small" onClick={() => setVerwalten(true)}>
          Manage
        </button>
      )}
      {verwalten && (
        <VorlagenVerwalten
          liste={eigene}
          onGeaendert={laden}
          onClose={() => setVerwalten(false)}
        />
      )}
    </div>
  );
}

function VorlagenVerwalten({
  liste,
  onGeaendert,
  onClose,
}: {
  liste: Vorlage[];
  onGeaendert: () => void;
  onClose: () => void;
}) {
  const { user } = useAuth();
  const frage = useRueckfrage();
  const loeschen = async (v: Vorlage) => {
    if (
      !(await frage({
        titel: "Delete template",
        text: "Pages made from this template stay as they are.",
        bestaetigen: "Delete",
        gefaehrlich: true,
      }))
    )
      return;
    await api.vorlageLoeschen(v.id).catch(() => {});
    onGeaendert();
  };
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>Templates</h3>
          <button className="icon-btn" onClick={onClose} aria-label="Close">
            ✕
          </button>
        </div>
        <div className="modal-section">
          {liste.map((v) => (
            <div key={v.id} className="version-row">
              <div>
                <div className="version-title notranslate">{v.title}</div>
                <div className="muted small">{v.forAll ? "For everyone on this instance" : "Only for you"}</div>
              </div>
              {(v.own || (v.forAll && user?.role === "admin")) && (
                <button className="btn" onClick={() => loeschen(v)}>
                  Delete
                </button>
              )}
            </div>
          ))}
          <p className="muted small">New templates: open a page and choose "Save as template" in its ⋯ menu.</p>
        </div>
      </div>
    </div>
  );
}
