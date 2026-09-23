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
        titel: "Delete permanently",
        text: "This page and its subpages are removed for good, attachments included. This cannot be undone.",
        bestaetigen: "Delete permanently",
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
        <h1 className="view-title">Trash</h1>
        <p className="muted">
          Deleted pages stay here until you restore them or remove them for good.
          {items.some((p) => p.verfaelltAm)
            ? " After that the trash empties itself; how long a page still has is shown next to it."
            : ""}
        </p>
        {items.length === 0 ? (
          <div className="muted" style={{ marginTop: 20 }}>
            The trash is empty.
          </div>
        ) : (
          <div className="list">
            {items.map((p) => (
              <div key={p.id} className="list-row">
                <span className="list-title">{p.title || "Untitled"}</span>
                {/* The remaining time instead of the date: "noch 3 Tage" is the
                    figure one acts on. The day stands in the title, in case
                    somebody wants to know exactly. */}
                {p.verfaelltAm && (
                  <span
                    className={"muted small" + (restTage(p.verfaelltAm) <= 3 ? " bald" : "")}
                    title={"Expires on " + new Date(p.verfaelltAm).toLocaleDateString()}
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

// restTage says how many days are left, rounded up, because "noch 0 Tage" would
// simply be wrong for a page that still exists tomorrow morning.
function restTage(verfaelltAm: string): number {
  const ms = new Date(verfaelltAm).getTime() - Date.now();
  return Math.ceil(ms / 86400000);
}

function restText(verfaelltAm: string): string {
  const t = restTage(verfaelltAm);
  if (t <= 0) return "will be deleted on the next pass";
  if (t === 1) return "1 day left";
  return `${t} days left`;
}
