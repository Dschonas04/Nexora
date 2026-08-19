// A viewer for images, PDFs and text files that opens over the page instead of
// sending the reader off to a download.
//
// It is deliberately its own component rather than living inside the attachment
// list: anything that can produce a list of files can open it. The caller hands
// over the whole list and which entry to start on, so browsing between files
// happens in here and every caller gets it for free.
import { useCallback, useEffect, useRef, useState } from "react";

export interface Datei {
  id: string;
  filename: string;
  mime: string;
  /** Where the bytes live. The caller builds it -- the viewer never guesses. */
  url: string;
}

export const istBild = (m: string) => m.startsWith("image/");
export const istPdf = (m: string) => m === "application/pdf";
export const istText = (m: string) => m.startsWith("text/") || m === "application/json";
export const zeigbar = (m: string) => istBild(m) || istPdf(m) || istText(m);

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

  const box = useRef<HTMLDivElement>(null);
  // Remembered so focus can go back where it was when the viewer closes. Without
  // this a keyboard user is dropped at the top of the document.
  const vorher = useRef<HTMLElement | null>(null);

  const datei = dateien[i];

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
  }, [datei?.id]);

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
    if (!datei || !istText(datei.mime)) return;
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
  }, [datei?.id, datei?.mime, datei?.url]);

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
            {istBild(datei.mime) && (
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

          {istBild(datei.mime) && (
            <img className="qv-image" style={bildStil} src={datei.url} alt={datei.filename} />
          )}
          {istPdf(datei.mime) && (
            // The browser's own PDF viewer already brings paging, zoom and
            // search. Rebuilding that on top of a rendering library would be a
            // lot of code for a worse result.
            <iframe className="qv-frame" src={datei.url} title={datei.filename} />
          )}
          {istText(datei.mime) && (
            <pre className="qv-text">
              {textFehler ? "(Vorschau konnte nicht geladen werden)" : (text ?? "Lädt…")}
            </pre>
          )}
          {!zeigbar(datei.mime) && (
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
          {istBild(datei.mime) && ", + − zoomt, R dreht, 0 setzt zurück"}
        </div>
      </div>
    </div>
  );
}
