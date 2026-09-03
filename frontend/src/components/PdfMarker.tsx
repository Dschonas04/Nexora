// Annotate a PDF by drawing highlights and write the result back to the file.
//
// The browser's built-in viewer is sufficient for reading and remains in the
// quick preview. It cannot, however, tell where the user is dragging on the
// page, so this UI draws overlays for marking: pdf.js renders a page to a
// canvas and we capture drag rectangles on top of it.
//
// This component only collects coordinates. Writing annotations into the
// binary is done by `src/pdfmarken.ts` and is testable outside the browser;
// this file is the user-facing hand, the other is the head.
import { useCallback, useEffect, useRef, useState } from "react";

import { api } from "../api/client";
import { useAuth } from "../auth";
import { MARKIERFARBEN, Marke, ausBildschirm, markenAnwenden, normiert } from "../pdfmarken";

// pdf.js is only loaded when someone actually marks something. The library
// is large compared to the rest of the bundle and the vast majority of page
// visits do not create annotations.
type PdfSeite = {
  getViewport: (o: { scale: number }) => { width: number; height: number };
  render: (o: { canvasContext: CanvasRenderingContext2D; viewport: unknown }) => { promise: Promise<void> };
};
type PdfDatei = { numPages: number; getPage: (n: number) => Promise<PdfSeite> };

async function pdfjsLaden() {
  const pdfjs = await import("pdfjs-dist");
  // The worker runs in a separate thread to avoid blocking the UI while a
  // large page is processed. The worker path is constructed via import.meta.url
  // so the bundler includes the file.
  pdfjs.GlobalWorkerOptions.workerSrc = new URL(
    "pdfjs-dist/build/pdf.worker.min.mjs",
    import.meta.url,
  ).href;
  return pdfjs;
}

/** A dragged rectangle in display coordinates, before it becomes an annotation. */
interface Zug {
  seite: number;
  von: { x: number; y: number };
  bis: { x: number; y: number };
}

interface Props {
  /** Where the source bytes come from. */
  url: string;
  seiteId: string;
  anhangId: string;
  dateiname: string;
  onFertig: () => void;
  onAbbruch: () => void;
}

// A scale at which an A4 page fits a typical screen without the text becoming
// too small to target.
const MASSSTAB = 1.4;

export default function PdfMarker({
  url,
  seiteId,
  anhangId,
  dateiname,
  onFertig,
  onAbbruch,
}: Props) {
  const [roh, setRoh] = useState<Uint8Array | null>(null);
  const [seiten, setSeiten] = useState<{ breite: number; hoehe: number }[]>([]);
  const [marken, setMarken] = useState<Marke[]>([]);
  const [farbe, setFarbe] = useState("yellow");
  const [zug, setZug] = useState<Zug | null>(null);
  // Which annotation is currently requesting a note. The prompt appears
  // immediately after the drag at the dragged location: users who want to add
  // a remark still have the passage in view; users who do not simply continue.
  const [notizBei, setNotizBei] = useState<number | null>(null);
  const [fehler, setFehler] = useState<string | null>(null);
  const [speichert, setSpeichert] = useState(false);
  const leinwaende = useRef<(HTMLCanvasElement | null)[]>([]);
  const { user } = useAuth();

  // The file bytes are fetched once and used twice: pdf.js renders them and
  // pdf-lib writes annotations to them. A second fetch would retrieve a
  // different revision if the file changed in the meantime.
  useEffect(() => {
    let weg = false;
    fetch(url, { credentials: "include" })
      .then((r) => {
        if (!r.ok) throw new Error("Die Datei ließ sich nicht laden (" + r.status + ").");
        return r.arrayBuffer();
      })
      .then((b) => !weg && setRoh(new Uint8Array(b)))
      .catch((e: Error) => !weg && setFehler(e.message));
    return () => {
      weg = true;
    };
  }, [url]);

  // Rendering. pdf.js consumes the passed bytes, so we pass it a copy; without
  // that the buffer would be empty when saving.
  useEffect(() => {
    if (!roh) return;
    let weg = false;
    (async () => {
      try {
        const pdfjs = await pdfjsLaden();
        const doc = (await pdfjs.getDocument({ data: roh.slice() }).promise) as unknown as PdfDatei;
        if (weg) return;
        const masse: { breite: number; hoehe: number }[] = [];
        for (let n = 1; n <= doc.numPages; n++) {
          const s = await doc.getPage(n);
          const blick = s.getViewport({ scale: MASSSTAB });
          masse.push({ breite: blick.width, hoehe: blick.height });
        }
        if (weg) return;
        setSeiten(masse);
        // Only paint after the state is in place: the canvas elements do not
        // exist before then. Give the browser a frame to settle, then render.
        requestAnimationFrame(async () => {
          for (let n = 1; n <= doc.numPages; n++) {
            if (weg) return;
            const flaeche = leinwaende.current[n - 1];
            if (!flaeche) continue;
            const s = await doc.getPage(n);
            const blick = s.getViewport({ scale: MASSSTAB });
            flaeche.width = blick.width;
            flaeche.height = blick.height;
            const stift = flaeche.getContext("2d");
            if (!stift) continue;
            await s.render({ canvasContext: stift, viewport: blick }).promise;
          }
        });
      } catch (e) {
        if (!weg) setFehler("Diese Datei ließ sich nicht anzeigen: " + (e as Error).message);
      }
    })();
    return () => {
      weg = true;
    };
  }, [roh]);

  const ortAuf = (e: React.MouseEvent, flaeche: HTMLElement) => {
    const k = flaeche.getBoundingClientRect();
    return { x: e.clientX - k.left, y: e.clientY - k.top };
  };

  // A drag under three pixels is considered a click, not a selection. Without
  // this threshold accidental clicks would produce tiny marks.
  const genugGezogen = (z: Zug) =>
    Math.abs(z.bis.x - z.von.x) > 3 && Math.abs(z.bis.y - z.von.y) > 3;

  const beenden = useCallback(
    (z: Zug) => {
      setZug(null);
      if (!genugGezogen(z)) return;
      const k = normiert(z.von, z.bis);
      const seite = seiten[z.seite];
      if (!seite) return;
      // The page height in typographic points, not screen pixels: the display
      // is enlarged by the scale factor.
      const punkt = ausBildschirm(k, MASSSTAB, seite.hoehe / MASSSTAB);
      setMarken((alt) => {
        setNotizBei(alt.length);
        return [...alt, { seite: z.seite, ...punkt, farbe }];
      });
    },
    [farbe, seiten],
  );

  const speichern = async () => {
    if (!roh || marken.length === 0) return;
    setSpeichert(true);
    setFehler(null);
    try {
      const neu = await markenAnwenden(roh, marken, user?.name || user?.email || "");
      await api.pdfErsetzen(seiteId, anhangId, neu);
      onFertig();
    } catch (e) {
      setFehler((e as Error).message);
      setSpeichert(false);
    }
  };

  return (
    <div className="pdfm">
      <div className="pdfm-leiste">
        <span className="pdfm-name" title={dateiname}>
          {dateiname}
        </span>
        <span className="muted small">Ziehen markiert</span>
        <div className="pdfm-farben">
          {Object.keys(MARKIERFARBEN).map((name) => {
            const [r, g, b] = MARKIERFARBEN[name];
            return (
              <button
                key={name}
                className={"pdfm-farbe" + (farbe === name ? " ist-aktiv" : "")}
                title={name}
                aria-label={"Farbe " + name}
                aria-pressed={farbe === name}
                style={{ background: `rgb(${r * 255},${g * 255},${b * 255})` }}
                onClick={() => setFarbe(name)}
              />
            );
          })}
        </div>
        <button
          className="btn"
          disabled={marken.length === 0 || speichert}
          onClick={() => {
            setNotizBei(null);
            setMarken((alt) => alt.slice(0, -1));
          }}
        >
          Zurücknehmen
        </button>
        <button className="btn" disabled={marken.length === 0 || speichert} onClick={speichern}>
          {speichert ? "Speichert…" : `Speichern (${marken.length})`}
        </button>
        <button className="btn" disabled={speichert} onClick={onAbbruch}>
          Abbrechen
        </button>
      </div>

        {/* The note is shown BEFORE saving as a reminder: the old version is
          replaced when you save. */}
      <div className="pdfm-hinweis muted small">
        Beim Speichern wird die vorhandene Datei ersetzt, nicht ergänzt. Die
        unmarkierte Fassung ist danach nur noch da, wo sie vorher heruntergeladen
        wurde.
      </div>
      {fehler && <div className="hinweis-fehler">{fehler}</div>}

      <div className="pdfm-blaetter">
        {seiten.length === 0 && !fehler && <div className="qv-none">Wird geladen…</div>}
        {seiten.map((s, nr) => (
          <div
            key={nr}
            className="pdfm-blatt"
            style={{ width: s.breite, height: s.hoehe }}
            onMouseDown={(e) => {
              const p = ortAuf(e, e.currentTarget);
              setZug({ seite: nr, von: p, bis: p });
            }}
            onMouseMove={(e) => {
              if (!zug || zug.seite !== nr) return;
              const p = ortAuf(e, e.currentTarget);
              setZug({ ...zug, bis: p });
            }}
            onMouseUp={() => zug && zug.seite === nr && beenden(zug)}
            // If the mouse leaves the page while a button is pressed consider
            // the drag finished. Otherwise a rectangle could remain stuck and
            // never complete.
            onMouseLeave={() => zug && zug.seite === nr && beenden(zug)}
          >
            <canvas
              className="pdfm-leinwand"
              ref={(el) => {
                leinwaende.current[nr] = el;
              }}
            />
            {/* The applied annotations, converted back to display coordinates. */}
            {marken
              .map((m, i) => ({ m, i }))
              .filter(({ m }) => m.seite === nr)
              .map(({ m, i }) => {
                const [r, g, b] = MARKIERFARBEN[m.farbe] ?? MARKIERFARBEN.yellow;
                return (
                  <div
                    key={i}
                    className="pdfm-marke"
                    style={{
                      left: m.x * MASSSTAB,
                      top: s.hoehe - (m.y + m.hoehe) * MASSSTAB,
                      width: m.breite * MASSSTAB,
                      height: m.hoehe * MASSSTAB,
                      background: `rgba(${r * 255},${g * 255},${b * 255},0.35)`,
                    }}
                  />
                );
              })}
            {/* The note for the most recently added annotation. */}
            {notizBei !== null && marken[notizBei] && marken[notizBei].seite === nr && (
              <input
                className="pdfm-notiz"
                autoFocus
                placeholder="Anmerkung (leer lassen: keine)"
                defaultValue={marken[notizBei].notiz ?? ""}
                style={{
                  left: marken[notizBei].x * MASSSTAB,
                  top: s.hoehe - marken[notizBei].y * MASSSTAB + 4,
                }}
                onMouseDown={(e) => e.stopPropagation()}
                onKeyDown={(e) => {
                  if (e.key === "Enter") (e.target as HTMLInputElement).blur();
                  if (e.key === "Escape") setNotizBei(null);
                }}
                onBlur={(e) => {
                  const t = e.target.value.trim();
                  setMarken((a) =>
                    a.map((m, k) => (k === notizBei ? { ...m, notiz: t || undefined } : m)),
                  );
                  setNotizBei(null);
                }}
              />
            )}
            {/* The rectangle while it is being dragged. */}
            {zug && zug.seite === nr && (
              <div
                className="pdfm-marke pdfm-zug"
                style={{
                  left: Math.min(zug.von.x, zug.bis.x),
                  top: Math.min(zug.von.y, zug.bis.y),
                  width: Math.abs(zug.bis.x - zug.von.x),
                  height: Math.abs(zug.bis.y - zug.von.y),
                }}
              />
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
