import { useEffect, useState } from "react";
import { Page, PageVersion, api } from "../api/client";

interface Props {
  pageId: string;
  canEdit: boolean;
  onRestored: (page: Page) => void;
  onClose: () => void;
}

export default function VersionPanel({ pageId, canEdit, onRestored, onClose }: Props) {
  const [versions, setVersions] = useState<PageVersion[]>([]);
  const [busy, setBusy] = useState<string | null>(null);

  useEffect(() => {
    api.listVersions(pageId).then(setVersions).catch(() => setVersions([]));
  }, [pageId]);

  const restore = async (versionId: string) => {
    if (!confirm("Restore this version? The current state is saved to history first.")) return;
    setBusy(versionId);
    try {
      const page = await api.restoreVersion(pageId, versionId);
      onRestored(page);
    } finally {
      setBusy(null);
    }
  };

  const fmt = (iso: string) => new Date(iso).toLocaleString();

  return (
    <div className="side-panel">
      <div className="side-panel-header">
        <h3>Version history</h3>
        <button className="icon-btn" onClick={onClose}>
          ✕
        </button>
      </div>
      <div className="side-panel-body">
        {versions.length === 0 && <div className="muted small">No earlier versions yet.</div>}
        {versions.map((v) => (
          <div key={v.id} className="version-row">
            <div>
              <div className="version-title">{v.title || "Untitled"}</div>
              <div className="muted small">
                {fmt(v.createdAt)} · {v.authorName}
              </div>
            </div>
            {canEdit && (
              <button className="btn" disabled={busy === v.id} onClick={() => restore(v.id)}>
                {busy === v.id ? "…" : "Restore"}
              </button>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
