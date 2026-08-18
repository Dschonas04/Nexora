// The attachment list under a page, plus its preview overlay.
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

// Which types can be shown in place. Everything else is offered as a download
// only: rendering an arbitrary upload inline would mean trusting its content.
const isImage = (m: string) => m.startsWith("image/");
const isPdf = (m: string) => m === "application/pdf";
const isText = (m: string) => m.startsWith("text/") || m === "application/json";
const canPreview = (m: string) => isImage(m) || isPdf(m) || isText(m);

export default function Attachments({ pageId, canEdit }: Props) {
  const [items, setItems] = useState<Attachment[]>([]);
  const [busy, setBusy] = useState(false);
  const [preview, setPreview] = useState<Attachment | null>(null);
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
          const previewable = canPreview(a.mime);
          return (
            <div key={a.id} className="attachment-row">
              {isImage(a.mime) ? (
                <img
                  className="attachment-thumb"
                  src={url}
                  alt={a.filename}
                  onClick={() => setPreview(a)}
                />
              ) : (
                <span className="attachment-thumb attachment-thumb-file">
                  {isPdf(a.mime) ? "PDF" : (a.filename.split(".").pop() || "·").slice(0, 4).toUpperCase()}
                </span>
              )}
              {previewable ? (
                <button className="attachment-name" onClick={() => setPreview(a)}>
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

      {preview && (
        <QuickView
          att={preview}
          url={api.attachmentUrl(pageId, preview.id)}
          onClose={() => setPreview(null)}
        />
      )}
    </div>
  );
}

// QuickView renders an in-page preview (lightbox) for images, PDFs and text so
// files can be inspected without leaving the page or downloading them.
function QuickView({ att, url, onClose }: { att: Attachment; url: string; onClose: () => void }) {
  const [text, setText] = useState<string | null>(null);

  // Escape closes the overlay. The listener is removed on unmount, otherwise
  // every opened preview would leave one behind.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  // Text is fetched and rendered as plain text rather than shown in a frame, so
  // an uploaded HTML file cannot execute in the app's origin. The cut-off keeps
  // a huge log file from freezing the browser.
  useEffect(() => {
    if (isText(att.mime)) {
      fetch(url, { credentials: "include" })
        .then((r) => r.text())
        .then((t) => setText(t.slice(0, 20000)))
        .catch(() => setText("(Vorschau konnte nicht geladen werden)"));
    }
  }, [att.mime, url]);

  return (
    <div className="qv-overlay" onClick={onClose}>
      <div className="qv-box" onClick={(e) => e.stopPropagation()}>
        <div className="qv-head">
          <span className="qv-title">{att.filename}</span>
          <div className="qv-actions">
            <a className="btn" href={url} download={att.filename}>
              Herunterladen
            </a>
            <button className="btn" onClick={onClose}>
              Schließen
            </button>
          </div>
        </div>
        <div className="qv-body">
          {isImage(att.mime) && <img className="qv-image" src={url} alt={att.filename} />}
          {isPdf(att.mime) && <iframe className="qv-frame" src={url} title={att.filename} />}
          {isText(att.mime) && <pre className="qv-text">{text ?? "Lädt…"}</pre>}
          {!canPreview(att.mime) && (
            <div className="qv-none">
              Keine Vorschau für diesen Dateityp verfügbar.
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
