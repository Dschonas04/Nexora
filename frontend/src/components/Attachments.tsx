// The attachment list under a page. The preview overlay itself lives in
// QuickView, so anything else with a list of files can reuse it.
import { useEffect, useRef, useState } from "react";
import { Attachment, api } from "../api/client";
import QuickView, { istBild, istPdf, zeigbar } from "./QuickView";

interface Props {
  pageId: string;
  canEdit: boolean;
}

function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

// Which types can be shown in place is decided by the viewer, not here -- one
// list, so the thumbnail and the overlay can never disagree about what is
// previewable.

export default function Attachments({ pageId, canEdit }: Props) {
  const [items, setItems] = useState<Attachment[]>([]);
  const [busy, setBusy] = useState(false);
  // Index statt Objekt: der Viewer blättert durch die ganze Liste, dafür muss
  // er wissen, an welcher Stelle er steht -- nicht nur, welche Datei gemeint war.
  const [offenBei, setOffenBei] = useState<number | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const refresh = () => api.listAttachments(pageId).then(setItems).catch(() => setItems([]));
  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageId]);

  // Uploads run one after another, not in parallel: the size limit applies per
  // file, and sequential requests keep a large selection from saturating the
  // connection. The input value is cleared so the same file can be picked twice.
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

  // Readers see nothing at all when a page has no files, rather than an empty
  // heading for a section they could not use anyway.
  if (items.length === 0 && !canEdit) return null;

  return (
    <div className="attachments">
      <div className="attachments-head">
        <span className="section-label">Anhänge</span>
        {canEdit && (
          <>
            {/* The real file input is hidden and triggered by the button, so
                the control can be styled like the rest of the UI. */}
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
        {items.map((a) => {
          const url = api.attachmentUrl(pageId, a.id);
          const previewable = zeigbar(a.mime);
          return (
            <div key={a.id} className="attachment-row">
              {istBild(a.mime) ? (
                <img
                  className="attachment-thumb"
                  src={url}
                  alt={a.filename}
                  onClick={() => setOffenBei(items.indexOf(a))}
                />
              ) : (
                <span className="attachment-thumb attachment-thumb-file">
                  {istPdf(a.mime) ? "PDF" : (a.filename.split(".").pop() || "·").slice(0, 4).toUpperCase()}
                </span>
              )}
              {previewable ? (
                <button className="attachment-name" onClick={() => setOffenBei(items.indexOf(a))}>
                  {a.filename}
                </button>
              ) : (
                <a className="attachment-name" href={url} target="_blank" rel="noreferrer">
                  {a.filename}
                </a>
              )}
              <span className="muted small">{humanSize(a.size)}</span>
              <a className="icon-btn" title="Herunterladen" href={url} download={a.filename}>
                ↓
              </a>
              {canEdit && (
                <button className="icon-btn" title="Entfernen" onClick={() => remove(a.id)}>
                  ✕
                </button>
              )}
            </div>
          );
        })}
      </div>

      {offenBei !== null && (
        <QuickView
          dateien={items.map((a) => ({
            id: a.id,
            filename: a.filename,
            mime: a.mime,
            url: api.attachmentUrl(pageId, a.id),
          }))}
          start={offenBei}
          onClose={() => setOffenBei(null)}
        />
      )}
    </div>
  );
}
