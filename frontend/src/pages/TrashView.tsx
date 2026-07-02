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
    if (!confirm("Permanently delete this page and its subpages? This cannot be undone.")) return;
    await api.purgePage(id);
    refresh();
    onChange();
  };

  return (
    <div className="editor-scroll">
      <div className="page wide">
        <h1 className="view-title">Trash</h1>
        <p className="muted">Deleted pages are kept here until you restore or permanently remove them.</p>
        {items.length === 0 ? (
          <div className="muted" style={{ marginTop: 20 }}>
            The trash is empty.
          </div>
        ) : (
          <div className="list">
            {items.map((p) => (
              <div key={p.id} className="list-row">
                <span className="list-title">{p.title || "Untitled"}</span>
                <span className="row-actions">
                  <button className="btn" onClick={() => restore(p.id)}>
                    Restore
                  </button>
                  <button className="btn danger" onClick={() => purge(p.id)}>
                    Delete forever
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
