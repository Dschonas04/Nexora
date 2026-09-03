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
        titel: "Version wiederherstellen",
        text: "Der Stand dieser Version wird auf die Seite zurückgeholt. Der aktuelle Stand geht nicht verloren, er wird vorher im Verlauf abgelegt.",
        bestaetigen: "Wiederherstellen",
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

  const fmt = (iso: string) => new Date(iso).toLocaleString("de-DE");

  return (
    <div className="side-panel">
      <div className="side-panel-header">
        <h3>Versionsverlauf</h3>
        <button className="icon-btn" onClick={onClose}>
          ✕
        </button>
      </div>
      <div className="side-panel-body">
        {versions.length === 0 && <div className="muted small">Noch keine früheren Versionen.</div>}
        {versions.map((v) => (
          <div key={v.id} className="version-row">
            <div>
              <div className="version-title">{v.title || "Ohne Titel"}</div>
              <div className="muted small">
                {fmt(v.createdAt)} · {v.authorName}
              </div>
            </div>
            {/* Read-only viewers see the history but cannot roll the page back. */}
        {canEdit && (
              <button className="btn" disabled={busy === v.id} onClick={() => restore(v.id)}>
                {busy === v.id ? "…" : "Wiederherstellen"}
              </button>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
