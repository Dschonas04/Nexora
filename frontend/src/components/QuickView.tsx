// A viewer for images, PDFs and text files that opens over the page instead of
// sending the reader off to a download.
//
// It is deliberately its own component rather than living inside the attachment
// list: anything that can produce a list of files can open it. The caller hands
// over the whole list and which entry to start on, so browsing between files
// happens in here and every caller gets it for free.
import { useCallback, useEffect, useRef, useState } from "react";

import { api } from "../api/client";
import Editor from "./Editor";

export interface Datei {
  id: string;
  filename: string;
  mime: string;
  /** Where the bytes live. The caller builds it, the viewer never guesses. */
  url: string;
  /** For Word files: without both the content cannot be fetched. */
  seiteId?: string;
  darfSchreiben?: boolean;
}

// The file type the browser states on upload is a claim and sometimes not even
// that: with an unknown extension it sends nothing or
// "application/octet-stream". Then the extension of the file name decides.
// Without that a PDF file stands in the list reporting "no preview" although it
// would have one.
const NACH_ENDUNG: Record<string, string> = {
  pdf: "application/pdf",
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  gif: "image/gif",
  webp: "image/webp",
  svg: "image/svg+xml",
  bmp: "image/bmp",
  txt: "text/plain",
  md: "text/markdown",
  csv: "text/csv",
  log: "text/plain",
  json: "application/json",
  xml: "text/xml",
  yml: "text/plain",
  yaml: "text/plain",
  docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
};

export function echterTyp(mime: string, dateiname = ""): string {
  const brauchbar = mime && mime !== "application/octet-stream" && mime !== "binary/octet-stream";
  if (brauchbar) return mime;
  const endung = dateiname.includes(".") ? dateiname.split(".").pop()!.toLowerCase() : "";
  return NACH_ENDUNG[endung] ?? mime ?? "";
}

export const istBild = (m: string) => m.startsWith("image/");
export const istPdf = (m: string) => m === "application/pdf";
export const istText = (m: string) => m.startsWith("text/") || m === "application/json";
export const istWord = (m: string) =>
  m === "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
export const zeigbar = (m: string) => istBild(m) || istPdf(m) || istText(m) || istWord(m);

// Zoom bounds. Below a quarter nothing is recognisable, above eight times the
// browser starts to struggle with large images.
const ZOOM_MIN = 0.25;
const ZOOM_MAX = 8;

interface Props {
  dateien: Datei[];
  /** Index into dateien. Out-of-range values are clamped, not an error. */
  start: number;
  onClose: () => void;
}

export default function QuickView({ dateien, start, onClose }: Props) {
  const [i, setI] = useState(() => Math.min(Math.max(start, 0), dateien.length - 1));
  const [zoom, setZoom] = useState(1);
  const [drehung, setDrehung] = useState(0);
  const [text, setText] = useState<string | null>(null);
  const [textFehler, setTextFehler] = useState(false);

  // Word: content as editor blocks, plus the title and the editing state.
  const [word, setWord] = useState<{ titel: string; bloecke: unknown[] } | null>(null);
  const [wordFehler, setWordFehler] = useState<string | null>(null);
  const [bearbeiten, setBearbeiten] = useState(false);
  const [gespeichert, setGespeichert] = useState<string | null>(null);
  // Der jeweils letzte Stand aus dem Editor. Als Referenz, damit jeder
  // Tastendruck nicht die ganze Ansicht neu zeichnet.
  const wordStand = useRef<{ titel: string; bloecke: unknown } | null>(null);

  const box = useRef<HTMLDivElement>(null);
  // Remembered so focus can go back where it was when the viewer closes. Without
  // this a keyboard user is dropped at the top of the document.
  const vorher = useRef<HTMLElement | null>(null);

  const datei = dateien[i];
  // Determined once in one place and only that size used afterwards: otherwise
  // the header would come to a different conclusion than the content.
  const typ = datei ? echterTyp(datei.mime, datei.filename) : "";

  const weiter = useCallback(
    (schritt: number) => {
      setI((alt) => {
        const neu = alt + schritt;
        // Wrap around: from the last file forward lands on the first. With one
        // file the arrows simply do nothing.
        if (dateien.length === 0) return alt;
        return (neu + dateien.length) % dateien.length;
      });
    },
    [dateien.length],
  );

  // Reset per file. Carrying a zoom of 6 from a small image to the next one
  // would leave the reader staring at a corner of it.
  useEffect(() => {
    setZoom(1);
    setDrehung(0);
    setText(null);
    setTextFehler(false);
    setWord(null);
    setWordFehler(null);
    setBearbeiten(false);
    setGespeichert(null);
    wordStand.current = null;
  }, [datei?.id]);

  // Fetch a Word attachment. Not the bytes, which the browser cannot display,
  // but the content as blocks, which the server reads out of the file.
  useEffect(() => {
    if (!datei || !istWord(typ) || !datei.seiteId) return;
    let weg = false;
    api
      .wordLesen(datei.seiteId, datei.id)
      .then((w) => {
        if (weg) return;
        setWord(w);
        wordStand.current = { titel: w.titel, bloecke: w.bloecke };
      })
      .catch((e: Error) => !weg && setWordFehler(e.message));
    return () => {
      weg = true;
    };
  }, [datei, typ]);

  // Keyboard: the shortcuts a reader expects from any image viewer.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      switch (e.key) {
        case "Escape":
          onClose();
          break;
        case "ArrowRight":
          weiter(1);
          break;
        case "ArrowLeft":
          weiter(-1);
          break;
        case "+":
        case "=":
          setZoom((z) => Math.min(z * 1.25, ZOOM_MAX));
          break;
        case "-":
          setZoom((z) => Math.max(z / 1.25, ZOOM_MIN));
          break;
        case "0":
          setZoom(1);
          setDrehung(0);
          break;
        case "r":
       	case "R":
          setDrehung((d) => (d + 90) % 360);
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, weiter]);

  // Hold the page still behind the overlay and give focus to the viewer, so
  // Tab stays inside it and the shortcuts above arrive.
  useEffect(() => {
    vorher.current = document.activeElement as HTMLElement | null;
    const vorherigerOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    box.current?.focus();
    return () => {
      document.body.style.overflow = vorherigerOverflow;
      vorher.current?.focus?.();
    };
  }, []);

  // Text is fetched and rendered as text rather than shown in a frame, so an
  // uploaded HTML file cannot execute in the app's origin. The cut-off keeps a
  // huge log file from freezing the browser.
  useEffect(() => {
    if (!datei || !istText(typ)) return;
    let abgebrochen = false;
    fetch(datei.url, { credentials: "include" })
      .then((r) => {
        if (!r.ok) throw new Error(String(r.status));
        return r.text();
      })
      .then((t) => {
        if (!abgebrochen) setText(t.slice(0, 20000));
      })
      .catch(() => {
        if (!abgebrochen) setTextFehler(true);
      });
    // Guards against a slow response for a file the reader already left.
    return () => {
      abgebrochen = true;
    };
  }, [datei?.id, typ, datei?.url]);

  if (!datei) return null;

  const mehrere = dateien.length > 1;
  const bildStil = {
    transform: `scale(${zoom}) rotate(${drehung}deg)`,
    // Zoom is a transform, not a width change, so the layout does not reflow on
    // every step and the image stays sharp.
    transition: "transform 120ms ease-out",
  };

  return (
    <div className="qv-overlay" onClick={onClose}>
      <div
        className="qv-box"
        ref={box}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-label={datei.filename}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="qv-head">
          <span className="qv-title" title={datei.filename}>
            {datei.filename}
          </span>
          {mehrere && (
            <span className="qv-zaehler muted small">
              {i + 1} von {dateien.length}
            </span>
          )}
          <div className="qv-actions">
            {istBild(typ) && (
              <>
                <button
                  className="btn"
                  title="Verkleinern (-)"
                  onClick={() => setZoom((z) => Math.max(z / 1.25, ZOOM_MIN))}
                >
                  −
                </button>
                <button className="btn" title="Zurücksetzen (0)" onClick={() => { setZoom(1); setDrehung(0); }}>
                  {Math.round(zoom * 100)} %
                </button>
                <button
                  className="btn"
                  title="Vergrößern (+)"
                  onClick={() => setZoom((z) => Math.min(z * 1.25, ZOOM_MAX))}
                >
                  +
                </button>
                <button className="btn" title="Drehen (R)" onClick={() => setDrehung((d) => (d + 90) % 360)}>
                  ⟳
                </button>
              </>
            )}
            <a className="btn" href={datei.url} download={datei.filename}>
              Herunterladen
            </a>
            <button className="btn" onClick={onClose} title="Schließen (Esc)">
              Schließen
            </button>
          </div>
        </div>

        <div className="qv-body">
          {mehrere && (
            <button className="qv-nav qv-nav-links" title="Vorheriges (←)" onClick={() => weiter(-1)}>
              ‹
            </button>
          )}

          {istBild(typ) && (
            <img className="qv-image" style={bildStil} src={datei.url} alt={datei.filename} />
          )}
          {istPdf(typ) && (
            // The browser's built in PDF viewer brings paging, zoom and search
            // along already. Rebuilding that on a rendering library would be a
            // lot of code for a worse result.
            //
            // <object> instead of <iframe>: browsers that cannot display PDF
            // themselves, and mobile browsers mostly cannot, then show the
            // content between the tags instead of an empty white area one sits
            // in front of at a loss.
            <object className="qv-frame" data={datei.url} type="application/pdf" aria-label={datei.filename}>
              <div className="qv-none">
                Dieser Browser zeigt PDF-Dateien nicht selbst an.
                <a className="btn" href={datei.url} target="_blank" rel="noreferrer">
                  In neuem Tab öffnen
                </a>
              </div>
            </object>
          )}
          {istText(typ) && (
            <pre className="qv-text">
              {textFehler ? "(Vorschau konnte nicht geladen werden)" : (text ?? "Lädt…")}
            </pre>
          )}
          {istWord(typ) && (
            <div className="qv-word">
              {wordFehler ? (
                <div className="qv-none">{wordFehler}</div>
              ) : !word ? (
                <div className="qv-none">Wird gelesen…</div>
              ) : (
                <div className="qv-word-blatt">
                  <div className="qv-word-kopf">
                    <strong>{word.titel}</strong>
                    {datei.darfSchreiben && !bearbeiten && (
                      <button className="btn" onClick={() => setBearbeiten(true)}>
                        Bearbeiten
                      </button>
                    )}
                    {bearbeiten && (
                      <button
                        className="btn"
                        onClick={async () => {
                          if (!datei.seiteId || !wordStand.current) return;
                          setGespeichert("speichert");
                          try {
                            await api.wordSchreiben(
                              datei.seiteId,
                              datei.id,
                              wordStand.current.titel,
                              wordStand.current.bloecke,
                            );
                            setGespeichert("gespeichert");
                            setBearbeiten(false);
                          } catch (e) {
                            setWordFehler((e as Error).message);
                            setGespeichert(null);
                          }
                        }}
                      >
                        {gespeichert === "speichert" ? "Speichert…" : "Speichern"}
                      </button>
                    )}
                  </div>
                  {/* The note stands there BEFORE editing, not afterwards as an
                      excuse: whoever changes a Word file here gets a clean
                      document with its content, not the same file with one line
                      changed. */}
                  {bearbeiten && (
                    <div className="qv-word-hinweis muted small">
                      Beim Speichern wird die Datei neu geschrieben. Text, Überschriften,
                      Listen und Tabellen bleiben; Kopfzeilen, Formatvorlagen, Kommentare,
                      Bilder und Schriftarten gehen verloren.
                    </div>
                  )}
                  <Editor
                    key={datei.id + (bearbeiten ? ":schreiben" : ":lesen")}
                    initialContent={word.bloecke}
                    editable={!!bearbeiten}
                    onChange={(bloecke) => {
                      wordStand.current = { titel: word.titel, bloecke };
                    }}
                  />
                  {gespeichert === "gespeichert" && (
                    <div className="hinweis-ok">Gespeichert.</div>
                  )}
                </div>
              )}
            </div>
          )}
          {!zeigbar(typ) && (
            <div className="qv-none">
              Keine Vorschau für diesen Dateityp. Über „Herunterladen“ lässt er sich öffnen.
            </div>
          )}

          {mehrere && (
            <button className="qv-nav qv-nav-rechts" title="Nächstes (→)" onClick={() => weiter(1)}>
              ›
            </button>
          )}
        </div>

        <div className="qv-fuss muted small">
          Esc schließt{mehrere && ", ← → blättert"}
          {istBild(typ) && ", + − zoomt, R dreht, 0 setzt zurück"}
        </div>
      </div>
    </div>
  );
}
