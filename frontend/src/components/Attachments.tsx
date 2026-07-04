import { useEffect, useRef, useState } from "react";
import { Attachment, api } from "../api/client";

interface Props {
  pageId: string;
  canEdit: boolean;
}

function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export default function Attachments({ pageId, canEdit }: Props) {
  const [items, setItems] = useState<Attachment[]>([]);
  const [busy, setBusy] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const refresh = () => api.listAttachments(pageId).then(setItems).catch(() => setItems([]));
  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageId]);

  const upload = async (files: FileList | null) => {
    if (!files || files.length === 0) return;
    setBusy(true);
    try {
      for (const f of Array.from(files)) await api.uploadAttachment(pageId, f);
      refresh();
    } finally {
      setBusy(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  };

  const remove = async (attId: string) => {
    await api.deleteAttachment(pageId, attId);
    refresh();
  };

  if (items.length === 0 && !canEdit) return null;

  return (
    <div className="attachments">
      <div className="attachments-head">
        <span className="section-label">Anhänge</span>
        {canEdit && (
          <>
            <input
              ref={fileRef}
              type="file"
              multiple
              style={{ display: "none" }}
              onChange={(e) => upload(e.target.files)}
            />
            <button className="btn" disabled={busy} onClick={() => fileRef.current?.click()}>
              {busy ? "Hochladen…" : "Datei hochladen"}
            </button>
          </>
        )}
      </div>
      <div className="attachment-list">
        {items.length === 0 && <div className="muted small">Keine Dateien angehängt.</div>}
        {items.map((a) => (
          <div key={a.id} className="attachment-row">
            <a href={api.attachmentUrl(pageId, a.id)} target="_blank" rel="noreferrer">
              {a.filename}
            </a>
            <span className="muted small">{humanSize(a.size)}</span>
            {canEdit && (
              <button className="icon-btn" title="Entfernen" onClick={() => remove(a.id)}>
                ✕
              </button>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
