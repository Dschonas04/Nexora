// The trash. Deleted pages live here until they are restored or purged.
import { useEffect, useState } from "react";
import { PapierkorbSeite, api } from "../api/client";
import { useRueckfrage } from "../components/Rueckfrage";

// onChange tells the workspace to reload its page tree, since restoring and
// purging both change what the sidebar should show.
export default function TrashView({ onChange }: { onChange: () => void }) {
  const frage = useRueckfrage();
  const [items, setItems] = useState<PapierkorbSeite[]>([]);

  const refresh = () => api.listTrash().then(setItems).catch(() => setItems([]));
  useEffect(() => {
    refresh();
  }, []);

  const restore = async (id: string) => {
    await api.restorePage(id);
    refresh();
    onChange();
  };
  // Purging cascades to the subpages and cannot be undone, so it asks first.
  const purge = async (id: string) => {
    if (
      !(await frage({
        titel: "Endgültig löschen",
        text: "Diese Seite und ihre Unterseiten werden endgültig entfernt, samt ihrer Anhänge. Das lässt sich nicht rückgängig machen.",
        bestaetigen: "Endgültig löschen",
        gefaehrlich: true,
      }))
    )
      return;
    await api.purgePage(id);
    refresh();
    onChange();
  };

  return (
    <div className="editor-scroll">
      <div className="page wide">
        <h1 className="view-title">Papierkorb</h1>
        <p className="muted">
          Gelöschte Seiten bleiben hier, bis du sie wiederherstellst oder endgültig entfernst.
          {items.some((p) => p.verfaelltAm)
            ? " Danach räumt der Papierkorb sich selbst; wie lange eine Seite noch liegt, steht neben ihr."
            : ""}
        </p>
        {items.length === 0 ? (
          <div className="muted" style={{ marginTop: 20 }}>
            Der Papierkorb ist leer.
          </div>
        ) : (
          <div className="list">
            {items.map((p) => (
              <div key={p.id} className="list-row">
                <span className="list-title">{p.title || "Ohne Titel"}</span>
                {/* Die verbleibende Zeit statt des Datums: "noch 3 Tage" ist
                    die Angabe, nach der man handelt. Der Tag steht im title,
                    für den Fall, dass jemand es genau wissen will. */}
                {p.verfaelltAm && (
                  <span
                    className={"muted small" + (restTage(p.verfaelltAm) <= 3 ? " bald" : "")}
                    title={"Verfällt am " + new Date(p.verfaelltAm).toLocaleDateString("de-DE")}
                  >
                    {restText(p.verfaelltAm)}
                  </span>
                )}
                <span className="row-actions">
                  <button className="btn" onClick={() => restore(p.id)}>
                    Wiederherstellen
                  </button>
                  <button className="btn danger" onClick={() => purge(p.id)}>
                    Endgültig löschen
                  </button>
                </span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

// restTage sagt, wie viele Tage noch bleiben, aufgerundet, weil "noch 0 Tage"
// bei einer Seite, die es morgen früh noch gibt, schlicht falsch wäre.
function restTage(verfaelltAm: string): number {
  const ms = new Date(verfaelltAm).getTime() - Date.now();
  return Math.ceil(ms / 86400000);
}

function restText(verfaelltAm: string): string {
  const t = restTage(verfaelltAm);
  if (t <= 0) return "wird beim nächsten Durchgang gelöscht";
  if (t === 1) return "noch 1 Tag";
  return `noch ${t} Tage`;
}
