// Eine PDF-Datei anstreichen und das Ergebnis an die Stelle der alten schreiben.
//
// Zum Lesen genügt der Betrachter des Browsers, und deshalb bleibt er in der
// Schnellansicht auch stehen. Zum Markieren genügt er nicht: er gibt nicht
// heraus, wo auf der Seite jemand gerade gezogen hat. Also wird für diesen einen
// Zweck selbst gezeichnet -- pdf.js malt die Seite auf eine Leinwand, darüber
// liegt eine Fläche, auf der die Maus Rechtecke aufzieht.
//
// Was hier entsteht, sind nur Koordinaten. Das Schreiben in die Datei steht in
// src/pdfmarken.ts und ist ohne Browser prüfbar; diese Datei ist die Hand, jene
// der Kopf.
import { useCallback, useEffect, useRef, useState } from "react";

import { api } from "../api/client";
import { useAuth } from "../auth";
import { MARKIERFARBEN, Marke, ausBildschirm, markenAnwenden, normiert } from "../pdfmarken";

// pdf.js wird erst geladen, wenn jemand wirklich markiert. Die Bibliothek wiegt
// mehr als der halbe übrige Bestand, und die allermeisten Besuche einer Seite
// markieren nichts.
type PdfSeite = {
  getViewport: (o: { scale: number }) => { width: number; height: number };
  render: (o: { canvasContext: CanvasRenderingContext2D; viewport: unknown }) => { promise: Promise<void> };
};
type PdfDatei = { numPages: number; getPage: (n: number) => Promise<PdfSeite> };

async function pdfjsLaden() {
  const pdfjs = await import("pdfjs-dist");
  // Der Arbeiter läuft in einem eigenen Faden, sonst stünde die Oberfläche
  // still, während eine große Seite gerechnet wird. Die Adresse muss über
  // import.meta.url gehen, damit der Bündler die Datei mitnimmt.
  pdfjs.GlobalWorkerOptions.workerSrc = new URL(
    "pdfjs-dist/build/pdf.worker.min.mjs",
    import.meta.url,
  ).href;
  return pdfjs;
}

/** Ein aufgezogenes Rechteck in Anzeigekoordinaten, bevor es eine Marke wird. */
interface Zug {
  seite: number;
  von: { x: number; y: number };
  bis: { x: number; y: number };
}

interface Props {
  /** Woher die Vorlage kommt. */
  url: string;
  seiteId: string;
  anhangId: string;
  dateiname: string;
  onFertig: () => void;
  onAbbruch: () => void;
}

// Ein Maßstab, bei dem A4 auf einen gewöhnlichen Bildschirm passt, ohne dass
// der Satz zu klein zum Zielen wird.
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
  // Welche Marke gerade nach einem Zettel fragt. Die Frage kommt sofort nach dem
  // Ziehen und an der Stelle, an der gezogen wurde: wer eine Anmerkung schreiben
  // will, hat den Satz noch vor Augen, und wer keine will, tippt nichts und
  // klickt weiter.
  const [notizBei, setNotizBei] = useState<number | null>(null);
  const [fehler, setFehler] = useState<string | null>(null);
  const [speichert, setSpeichert] = useState(false);
  const leinwaende = useRef<(HTMLCanvasElement | null)[]>([]);
  const { user } = useAuth();

  // Die Bytes werden einmal geholt und dann zweimal gebraucht: pdf.js zeigt
  // damit an, pdf-lib schreibt darauf. Ein zweiter Abruf wäre eine zweite
  // Fassung -- und wenn zwischendurch jemand anders die Datei ersetzt hätte,
  // markierte man auf einer und überschriebe eine andere.
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

  // Anzeigen. pdf.js verbraucht die übergebenen Bytes, deshalb bekommt es eine
  // Abschrift; sonst stünde beim Speichern ein leerer Puffer bereit.
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
        // Erst nach dem Zustand malen: vorher gibt es die Leinwände noch nicht.
        // Ein Rahmen Verzögerung, dann stehen sie.
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

  // Ein Zug unter drei Pixeln ist ein Klick und keine Markierung. Ohne diese
  // Grenze bliebe bei jedem versehentlichen Klick ein Punkt in der Datei.
  const genugGezogen = (z: Zug) =>
    Math.abs(z.bis.x - z.von.x) > 3 && Math.abs(z.bis.y - z.von.y) > 3;

  const beenden = useCallback(
    (z: Zug) => {
      setZug(null);
      if (!genugGezogen(z)) return;
      const k = normiert(z.von, z.bis);
      const seite = seiten[z.seite];
      if (!seite) return;
      // Die Höhe der Seite in Punkten, nicht in Bildpunkten: die Anzeige ist um
      // den Maßstab größer.
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

      {/* Der Hinweis steht VOR dem Speichern da, nicht danach als Ausrede: die
          alte Fassung ist anschließend fort. */}
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
            // Verlässt die Maus das Blatt mit gedrückter Taste, gilt der Zug als
            // beendet. Ohne das bliebe ein Rechteck hängen, das nie fertig wird.
            onMouseLeave={() => zug && zug.seite === nr && beenden(zug)}
          >
            <canvas
              className="pdfm-leinwand"
              ref={(el) => {
                leinwaende.current[nr] = el;
              }}
            />
            {/* Die gesetzten Marken, zurückgerechnet auf die Anzeige. */}
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
            {/* Der Zettel zu der zuletzt gesetzten Marke. */}
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
            {/* Das Rechteck, während es gezogen wird. */}
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
