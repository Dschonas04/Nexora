// Side panel listing a page's history. The list carries metadata only; the
// content of a version is fetched when one is restored.
import { useEffect, useState } from "react";
import { Page, PageVersion, api } from "../api/client";
import { useRueckfrage } from "./Rueckfrage";

interface Props {
  pageId: string;
  canEdit: boolean;
  onRestored: (page: Page) => void;
  onClose: () => void;
}

export default function VersionPanel({ pageId, canEdit, onRestored, onClose }: Props) {
  const frage = useRueckfrage();
  const [versions, setVersions] = useState<PageVersion[]>([]);
  const [busy, setBusy] = useState<string | null>(null);

  useEffect(() => {
    api.listVersions(pageId).then(setVersions).catch(() => setVersions([]));
  }, [pageId]);

  // Restoring is safe because the backend snapshots the current state first;
  // the confirmation explains this so nobody fears losing their work.
  const restore = async (versionId: string) => {
    if (
      !(await frage({
        titel: "Restore version",
        text: "This version is brought back onto the page. The current state is not lost \u2014 it is filed in the history first.",
        bestaetigen: "Restore",
      }))
    )
      return;
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
            {/* Read-only viewers see the history but cannot roll the page back. */}
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
