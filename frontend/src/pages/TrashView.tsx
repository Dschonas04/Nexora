import { useEffect, useState } from "react";
import { PageMeta, api } from "../api/client";

export default function TrashView({ onChange }: { onChange: () => void }) {
  const [items, setItems] = useState<PageMeta[]>([]);

  const refresh = () => api.listTrash().then(setItems).catch(() => setItems([]));
  useEffect(() => {
    refresh();
  }, []);

  const restore = async (id: string) => {
    await api.restorePage(id);
    refresh();
    onChange();
  };
  const purge = async (id: string) => {
    if (!confirm("Diese Seite und ihre Unterseiten endgültig löschen? Das kann nicht rückgängig gemacht werden.")) return;
    await api.purgePage(id);
    refresh();
    onChange();
  };

  return (
    <div className="editor-scroll">
      <div className="page wide">
        <h1 className="view-title">Papierkorb</h1>
        <p className="muted">Gelöschte Seiten bleiben hier, bis du sie wiederherstellst oder endgültig entfernst.</p>
        {items.length === 0 ? (
          <div className="muted" style={{ marginTop: 20 }}>
            Der Papierkorb ist leer.
          </div>
        ) : (
          <div className="list">
            {items.map((p) => (
              <div key={p.id} className="list-row">
                <span className="list-title">{p.title || "Ohne Titel"}</span>
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
