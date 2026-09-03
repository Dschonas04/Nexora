// The attachment list under a page. The preview overlay itself lives in
// QuickView, so anything else with a list of files can reuse it.
import type { DragEvent } from "react";
import { useEffect, useRef, useState } from "react";
import { Attachment, api } from "../api/client";
import QuickView, { echterTyp, istBild, istPdf, zeigbar } from "./QuickView";

interface Props {
  pageId: string;
  canEdit: boolean;
  /**
   * Files dropped somewhere else on the page. The attachment list is a narrow
   * strip at the very bottom; whoever drags a file "onto the page" mostly does
   * not hit it. So the page takes the drop and passes it on to here, and the
   * upload happens in one place only.
   */
  eingeworfen?: FileList | null;
  onEingeworfenFertig?: () => void;
  /**
   * Counts up when somebody added a file elsewhere, an image in the editor for
   * instance. The list reads its own data and would otherwise show a state that
   * is one file behind until the page is opened again.
   */
  neuLaden?: number;
}

function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

// Which types can be shown in place is decided by the viewer, not here, one
// list, so the thumbnail and the overlay can never disagree about what is
// previewable.

export default function Attachments({
  pageId,
  canEdit,
  eingeworfen,
  onEingeworfenFertig,
  neuLaden,
}: Props) {
  const [items, setItems] = useState<Attachment[]>([]);
  const [busy, setBusy] = useState(false);
  // What went wrong during the last upload. Without this line a failed upload
  // could not be told apart from one that never happened.
  const [fehler, setFehler] = useState<string | null>(null);
  // An index instead of an object: the viewer pages through the whole list, and
  // for that it has to know where it stands, not only which file was meant.
  const [offenBei, setOffenBei] = useState<number | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  // What is going up right now: name, number in the row and fraction. Without a
  // display a large attachment looks like a hang.
  const [lauf, setLauf] = useState<{
    name: string;
    nummer: number;
    gesamt: number;
    anteil: number;
  } | null>(null);

  const refresh = () => api.listAttachments(pageId).then(setItems).catch(() => setItems([]));
  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageId, neuLaden]);

  // Uploads run one after another, not in parallel: the size limit applies per
  // file, and sequential requests keep a large selection from saturating the
  // connection. The input value is cleared so the same file can be picked twice.
  const upload = async (files: FileList | File[] | null) => {
    if (!files || files.length === 0) return;
    setBusy(true);
    setFehler(null);
    // Every file on its own, so that one that is too large or is refused does
    // not take the others with it. At the end stands what did not arrive; before
    // this an upload failed silently and the file was simply not there.
    const gescheitert: string[] = [];
    const alle = Array.from(files);
    try {
      for (const [i, f] of alle.entries()) {
        setLauf({ name: f.name, nummer: i + 1, gesamt: alle.length, anteil: 0 });
        try {
          await api.uploadAttachment(pageId, f, (anteil) =>
            setLauf({ name: f.name, nummer: i + 1, gesamt: alle.length, anteil }),
          );
        } catch (e) {
          gescheitert.push(`${f.name} (${e instanceof Error ? e.message : "Fehler"})`);
        }
      }
      await refresh();
    } finally {
      setBusy(false);
      setLauf(null);
      if (fileRef.current) fileRef.current.value = "";
      if (gescheitert.length > 0) {
        setFehler(
          gescheitert.length === 1
            ? `Nicht hochgeladen: ${gescheitert[0]}`
            : `${gescheitert.length} Dateien nicht hochgeladen: ${gescheitert.join(", ")}`,
        );
      }
    }
  };

  // Files dropped elsewhere on the page instead of onto the attachment list.
  useEffect(() => {
    if (!eingeworfen || eingeworfen.length === 0 || !canEdit) return;
    upload(eingeworfen).finally(() => onEingeworfenFertig?.());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [eingeworfen]);

  // Dragging and dropping files.
  //
  // The counter instead of a simple flag is necessary because dragleave also
  // fires when the pointer moves from one child element to another. With a bool
  // the highlight flickered on every movement.
  const [ueber, setUeber] = useState(0);

  const istDatei = (e: DragEvent) =>
    Array.from(e.dataTransfer.types).includes("Files");

  const abgelegt = async (e: DragEvent) => {
    if (!canEdit || !istDatei(e)) return;
    e.preventDefault();
    setUeber(0);
    await upload(e.dataTransfer.files);
  };

  const remove = async (attId: string) => {
    await api.deleteAttachment(pageId, attId);
    refresh();
  };

  // Readers see nothing at all when a page has no files, rather than an empty
  // heading for a section they could not use anyway.
  if (items.length === 0 && !canEdit) return null;

  return (
    <div
      className={"attachments" + (ueber > 0 ? " abwurf" : "")}
      // React only when files are really involved. Otherwise a page dragged out
      // of the tree would trigger the highlight too.
      onDragEnter={(e) => {
        if (canEdit && istDatei(e)) setUeber((n) => n + 1);
      }}
      onDragOver={(e) => {
        if (canEdit && istDatei(e)) {
          // Without preventDefault the browser opens the file in a new tab
          // instead of dropping it here.
          e.preventDefault();
          e.dataTransfer.dropEffect = "copy";
        }
      }}
      onDragLeave={() => setUeber((n) => Math.max(0, n - 1))}
      onDrop={abgelegt}
    >
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
      {lauf && (
        <div className="hochlauf">
          <div className="hochlauf-zeile">
            <span className="hochlauf-name">{lauf.name}</span>
            <span className="muted small">
              {lauf.gesamt > 1 && `${lauf.nummer} von ${lauf.gesamt} · `}
              {Math.round(lauf.anteil * 100)} %
            </span>
          </div>
            {/* The bar shows the fraction of the UPLOAD completed. At 100% the
              file has reached the server but may not be finalized yet, so the
              bar stays until the server confirms the operation. */}
          <div className="hochlauf-bahn">
            <div className="hochlauf-fortschritt" style={{ width: `${lauf.anteil * 100}%` }} />
          </div>
        </div>
      )}
      <div className="attachment-list">
        {items.length === 0 && (
          <div className="muted small">
            {canEdit ? "Keine Dateien angehängt. Dateien lassen sich hierher ziehen." : "Keine Dateien angehängt."}
          </div>
        )}
        {items.map((a) => {
          const url = api.attachmentUrl(pageId, a.id);
          // The same type determination as used by the viewer: otherwise the
          // list could show a preview icon that the viewer cannot actually
          // display, or vice versa.
          const typ = echterTyp(a.mime, a.filename);
          const previewable = zeigbar(typ);
          return (
            <div key={a.id} className="attachment-row">
              {istBild(typ) ? (
                <img
                  className="attachment-thumb"
                  src={url}
                  alt={a.filename}
                  onClick={() => setOffenBei(items.indexOf(a))}
                />
              ) : (
                <span className="attachment-thumb attachment-thumb-file">
                  {istPdf(typ) ? "PDF" : (a.filename.split(".").pop() || "·").slice(0, 4).toUpperCase()}
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

      {fehler && (
        <div className="anhang-fehler" role="alert">
          {fehler}
          <button className="icon-btn" title="Ausblenden" onClick={() => setFehler(null)}>
            ✕
          </button>
        </div>
      )}

      {ueber > 0 && canEdit && (
        <div className="abwurf-schleier">
          {busy ? "Wird hochgeladen…" : "Loslassen zum Anhängen"}
        </div>
      )}

      {offenBei !== null && (
        <QuickView
          dateien={items.map((a) => ({
            id: a.id,
            filename: a.filename,
            mime: a.mime,
            url: api.attachmentUrl(pageId, a.id),
            seiteId: pageId,
            darfSchreiben: canEdit,
          }))}
          start={offenBei}
          onClose={() => setOffenBei(null)}
        />
      )}
    </div>
  );
}
