// Rendered views for attachments the browser cannot show by itself, or must
// not show on the app's own origin: HTML pages, Markdown, CSV/TSV tables and
// spreadsheets (.xlsx, .ods).
//
// HTML runs in a sandboxed frame without scripts and without the app's
// origin: it is built from the fetched text (srcdoc), so the file is never
// served as text/html by the server, and the sandbox gives it an opaque origin
// that can neither read the session nor call the API. A policy inside the
// document additionally blocks every external request, so opening a page
// cannot ping a tracker either.

import { lazy, Suspense, useEffect, useMemo, useState } from "react";
import { api } from "../api/client";
import Fehlergrenze from "./Fehlergrenze";

const Editor = lazy(() => import("./Editor"));

export type Darstellungsart = "html" | "markdown" | "csv" | "tabelle" | "folien";

export function darstellungsart(typ: string, dateiname: string): Darstellungsart | null {
  const n = dateiname.toLowerCase();
  if (typ === "text/html" || typ === "application/xhtml+xml" || /\.x?html?$/.test(n)) return "html";
  if (typ === "text/markdown" || /\.(md|markdown|mdown)$/.test(n)) return "markdown";
  if (typ === "text/csv" || typ === "text/tab-separated-values" || /\.(csv|tsv)$/.test(n)) return "csv";
  if (
    typ === "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
    typ === "application/vnd.oasis.opendocument.spreadsheet" ||
    /\.(xlsx|xlsm|ods)$/.test(n)
  )
    return "tabelle";
  if (
    typ === "application/vnd.openxmlformats-officedocument.presentationml.presentation" ||
    /\.(pptx|ppsx)$/.test(n)
  )
    return "folien";
  return null;
}

// Shown in a table without virtualisation; beyond this the browser gets slow.
const MAX_ZEILEN = 5000;

function useText(url: string) {
  const [text, setText] = useState<string | null>(null);
  const [fehler, setFehler] = useState<string | null>(null);
  useEffect(() => {
    let weg = false;
    setText(null);
    setFehler(null);
    fetch(url, { credentials: "include", cache: "no-store" })
      .then((r) => {
        if (!r.ok) throw new Error("Datei nicht abrufbar (" + r.status + ")");
        return r.text();
      })
      .then((t) => !weg && setText(t))
      .catch((e: Error) => !weg && setFehler(e.message));
    return () => {
      weg = true;
    };
  }, [url]);
  return { text, fehler };
}

const HTML_RICHTLINIE =
  "default-src 'none'; img-src data: blob:; media-src data: blob:; style-src 'unsafe-inline'; font-src data:";

function HtmlAnsicht({ url, titel }: { url: string; titel: string }) {
  const { text, fehler } = useText(url);
  const doc = useMemo(() => {
    if (text == null) return "";
    // The policy goes first so it applies before anything in the file is
    // parsed. A meta tag in front of <html> is still taken into <head>.
    return `<meta http-equiv="Content-Security-Policy" content="${HTML_RICHTLINIE}"><meta charset="utf-8">` + text;
  }, [text]);
  if (fehler) return <div className="qv-none">{fehler}</div>;
  if (text == null) return <div className="qv-none">Wird geladen…</div>;
  return (
    <iframe
      className="qv-frame"
      title={titel}
      // Empty sandbox: no scripts, no forms, no popups, no top navigation and
      // an opaque origin.
      sandbox=""
      referrerPolicy="no-referrer"
      srcDoc={doc}
    />
  );
}

function MarkdownAnsicht({ url }: { url: string }) {
  const { text, fehler } = useText(url);
  const [bloecke, setBloecke] = useState<unknown[] | null>(null);
  const [parseFehler, setParseFehler] = useState<string | null>(null);
  useEffect(() => {
    if (text == null) return;
    let weg = false;
    setBloecke(null);
    (async () => {
      try {
        const { BlockNoteEditor } = await import("@blocknote/core");
        const ed = BlockNoteEditor.create();
        const b = await ed.tryParseMarkdownToBlocks(text);
        if (!weg) setBloecke(b);
      } catch (e) {
        if (!weg) setParseFehler((e as Error).message);
      }
    })();
    return () => {
      weg = true;
    };
  }, [text]);
  if (fehler || parseFehler) return <div className="qv-none">{fehler || parseFehler}</div>;
  if (!bloecke) return <div className="qv-none">Wird geladen…</div>;
  return (
    <div className="qv-word">
      <div className="qv-word-blatt">
        <Fehlergrenze text="Diese Datei lässt sich nicht darstellen.">
          <Suspense fallback={<div className="qv-none">Wird geladen…</div>}>
            <Editor initialContent={bloecke} editable={false} />
          </Suspense>
        </Fehlergrenze>
      </div>
    </div>
  );
}

// Splits CSV per RFC 4180: quoted fields may hold separators, line breaks and
// doubled quotes. The separator is guessed from the first line, since German
// Excel writes semicolons.
export function csvLesen(text: string, tabulator = false): string[][] {
  const t = text.replace(/^﻿/, "");
  let trenner = "\t";
  if (!tabulator) {
    const erste = t.slice(0, t.search(/\r?\n|$/));
    const zahl = (z: string) => erste.split(z).length;
    trenner = [";", ",", "\t", "|"].reduce((a, b) => (zahl(b) > zahl(a) ? b : a), ",");
  }
  const zeilen: string[][] = [];
  let zeile: string[] = [];
  let feld = "";
  let zitat = false;
  for (let i = 0; i < t.length; i++) {
    const c = t[i];
    if (zitat) {
      if (c === '"') {
        if (t[i + 1] === '"') {
          feld += '"';
          i++;
        } else zitat = false;
      } else feld += c;
      continue;
    }
    if (c === '"' && feld === "") zitat = true;
    else if (c === trenner) {
      zeile.push(feld);
      feld = "";
    } else if (c === "\n" || c === "\r") {
      if (c === "\r" && t[i + 1] === "\n") i++;
      zeile.push(feld);
      zeilen.push(zeile);
      zeile = [];
      feld = "";
      if (zeilen.length > MAX_ZEILEN) break;
    } else feld += c;
  }
  if (feld !== "" || zeile.length) {
    zeile.push(feld);
    zeilen.push(zeile);
  }
  return zeilen;
}

function Tabelle({ zeilen, gekuerzt }: { zeilen: string[][]; gekuerzt?: boolean }) {
  const breite = zeilen.reduce((m, z) => Math.max(m, z.length), 0);
  const zuViel = zeilen.length > MAX_ZEILEN;
  const zeigen = zuViel ? zeilen.slice(0, MAX_ZEILEN) : zeilen;
  if (zeilen.length === 0) return <div className="qv-none">Die Tabelle ist leer.</div>;
  return (
    <div className="qv-tabelle-rahmen">
      <table className="qv-tabelle">
        <thead>
          <tr>
            <th className="qv-tabelle-nr" />
            {Array.from({ length: breite }, (_, s) => (
              <th key={s}>{spaltenName(s)}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {zeigen.map((z, i) => (
            <tr key={i}>
              <th className="qv-tabelle-nr">{i + 1}</th>
              {Array.from({ length: breite }, (_, s) => (
                <td key={s}>{z[s] ?? ""}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      {(zuViel || gekuerzt) && (
        <div className="muted small qv-tabelle-hinweis">
          Gekürzt angezeigt. Die ganze Datei gibt es über „Herunterladen“.
        </div>
      )}
    </div>
  );
}

function spaltenName(n: number): string {
  let s = "";
  n++;
  while (n > 0) {
    const r = (n - 1) % 26;
    s = String.fromCharCode(65 + r) + s;
    n = Math.floor((n - 1) / 26);
  }
  return s;
}

function CsvAnsicht({ url, dateiname }: { url: string; dateiname: string }) {
  const { text, fehler } = useText(url);
  const zeilen = useMemo(
    () => (text == null ? null : csvLesen(text, /\.tsv$/i.test(dateiname))),
    [text, dateiname],
  );
  if (fehler) return <div className="qv-none">{fehler}</div>;
  if (!zeilen) return <div className="qv-none">Wird geladen…</div>;
  return <Tabelle zeilen={zeilen} />;
}

type Blatt = { name: string; rows: string[][]; truncated: boolean };

function TabellenAnsicht({ seiteId, anhangId }: { seiteId?: string; anhangId: string }) {
  const [blaetter, setBlaetter] = useState<Blatt[] | null>(null);
  const [fehler, setFehler] = useState<string | null>(null);
  const [aktiv, setAktiv] = useState(0);
  useEffect(() => {
    if (!seiteId) {
      setFehler("Vorschau hier nicht verfügbar. Über „Herunterladen“ lässt sich die Datei öffnen.");
      return;
    }
    let weg = false;
    setBlaetter(null);
    setFehler(null);
    setAktiv(0);
    api
      .tabelleLesen(seiteId, anhangId)
      .then((r) => !weg && setBlaetter(r.sheets))
      .catch((e: Error) => !weg && setFehler(e.message));
    return () => {
      weg = true;
    };
  }, [seiteId, anhangId]);
  if (fehler) return <div className="qv-none">{fehler}</div>;
  if (!blaetter) return <div className="qv-none">Wird geladen…</div>;
  const b = blaetter[Math.min(aktiv, blaetter.length - 1)];
  return (
    <div className="qv-mappe">
      <Tabelle zeilen={b.rows} gekuerzt={b.truncated} />
      {blaetter.length > 1 && (
        <div className="qv-blaetter" role="tablist">
          {blaetter.map((x, i) => (
            <button
              key={i}
              role="tab"
              aria-selected={i === aktiv}
              className={"btn" + (i === aktiv ? " aktiv" : "")}
              onClick={() => setAktiv(i)}
            >
              {x.name || "Blatt " + (i + 1)}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// ---- presentations ----

type FLauf = {
  text: string;
  size?: number;
  bold?: boolean;
  italic?: boolean;
  underline?: boolean;
  color?: string;
  br?: boolean;
  font?: string;
  href?: string;
  baseline?: number;
};
type FAbsatz = { align?: string; level?: number; bullet?: string; runs: FLauf[]; size?: number };
type FElement = {
  kind: "text" | "bild" | "tabelle";
  x: number;
  y: number;
  w: number;
  h: number;
  fill?: string;
  line?: string;
  anchor?: string;
  paragraphs?: FAbsatz[];
  image?: string;
  rows?: string[][];
  rotation?: number;
};
export type Praesentation = {
  width: number;
  height: number;
  slides: { background?: string; elements: FElement[] }[];
  images: Record<string, string>;
  truncated: boolean;
};

const EMU_PRO_PT = 12700;
const AUSRICHTUNG: Record<string, "left" | "center" | "right" | "justify"> = {
  l: "left",
  ctr: "center",
  r: "right",
  just: "justify",
  dist: "justify",
};

function Folie({ p, folie, nr }: { p: Praesentation; folie: Praesentation["slides"][number]; nr: number }) {
  // Sizes in the slide's own units, turned into a share of its width: with
  // container query units the text scales with the sheet like a picture.
  const breitePt = p.width / EMU_PRO_PT;
  const schrift = (hundertstel: number) => `${(hundertstel / 100 / breitePt) * 100}cqw`;
  const quer = (emu: number) => `${(emu / p.width) * 100}%`;
  const hoch = (emu: number) => `${(emu / p.height) * 100}%`;
  return (
    <div
      className="qv-folie"
      style={{ aspectRatio: `${p.width} / ${p.height}`, background: folie.background || "#fff" }}
      aria-label={"Folie " + nr}
    >
      {folie.elements.map((e, i) => {
        const rahmen: React.CSSProperties = {
          left: quer(e.x),
          top: hoch(e.y),
          width: quer(e.w),
          height: hoch(e.h),
          transform: e.rotation ? `rotate(${e.rotation}deg)` : undefined,
        };
        if (e.kind === "bild") {
          const src = e.image ? p.images?.[e.image] : undefined;
          return src ? <img key={i} className="qv-folie-el" style={rahmen} src={src} alt="" /> : null;
        }
        if (e.kind === "tabelle")
          return (
            <div key={i} className="qv-folie-el" style={rahmen}>
              <table className="qv-folie-tabelle" style={{ fontSize: schrift(1400) }}>
                <tbody>
                  {(e.rows ?? []).map((z, r) => (
                    <tr key={r}>
                      {z.map((c, k) => (r === 0 ? <th key={k}>{c}</th> : <td key={k}>{c}</td>))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          );
        return (
          <div
            key={i}
            className="qv-folie-el qv-folie-text"
            style={{
              ...rahmen,
              background: e.fill,
              border: e.line ? `1px solid ${e.line}` : undefined,
              justifyContent: e.anchor === "ctr" ? "center" : e.anchor === "b" ? "flex-end" : "flex-start",
              padding: `${hoch(45720)} ${quer(91440)}`,
            }}
          >
            {(e.paragraphs ?? []).map((a, k) => {
              const groesse = a.runs.find((l) => l.size)?.size ?? a.size ?? 1800;
              return (
                <p
                  key={k}
                  style={{
                    textAlign: AUSRICHTUNG[a.align ?? ""] ?? "left",
                    fontSize: schrift(groesse),
                    paddingLeft: a.level ? quer(a.level * 457200) : undefined,
                  }}
                >
                  {a.bullet && <span className="qv-folie-punkt">{a.bullet}</span>}
                  {a.runs.length === 0 && "\u00a0"}
                  {a.runs.map((l, j) => {
                    if (l.br) return <br key={j} />;
                    const stil: React.CSSProperties = {
                      fontSize: l.size ? schrift(l.size) : undefined,
                      fontWeight: l.bold ? 700 : undefined,
                      fontStyle: l.italic ? "italic" : undefined,
                      textDecoration: l.underline ? "underline" : undefined,
                      color: l.color,
                      fontFamily: l.font ? `"${l.font.replace(/["\\]/g, "")}", sans-serif` : undefined,
                      verticalAlign: l.baseline ? (l.baseline > 0 ? "super" : "sub") : undefined,
                    };
                    return l.href ? (
                      <a key={j} style={stil} href={l.href} target="_blank" rel="noreferrer noopener">
                        {l.text}
                      </a>
                    ) : (
                      <span key={j} style={stil}>
                        {l.text}
                      </span>
                    );
                  })}
                </p>
              );
            })}
          </div>
        );
      })}
    </div>
  );
}

function FolienAnsicht({ seiteId, anhangId }: { seiteId?: string; anhangId: string }) {
  const [p, setP] = useState<Praesentation | null>(null);
  const [fehler, setFehler] = useState<string | null>(null);
  useEffect(() => {
    if (!seiteId) {
      setFehler("Vorschau hier nicht verfügbar. Über „Herunterladen“ lässt sich die Datei öffnen.");
      return;
    }
    let weg = false;
    setP(null);
    setFehler(null);
    api
      .folienLesen(seiteId, anhangId)
      .then((r) => !weg && setP(r))
      .catch((e: Error) => !weg && setFehler(e.message));
    return () => {
      weg = true;
    };
  }, [seiteId, anhangId]);
  if (fehler) return <div className="qv-none">{fehler}</div>;
  if (!p) return <div className="qv-none">Wird geladen…</div>;
  return (
    <div className="qv-folien">
      {p.slides.map((f, i) => (
        <div key={i} className="qv-folie-rahmen">
          <Folie p={p} folie={f} nr={i + 1} />
          <div className="qv-folie-nr">{i + 1}</div>
        </div>
      ))}
      {p.truncated && (
        <div className="qv-folie-nr">
          Gekürzt angezeigt. Die ganze Datei gibt es über „Herunterladen“.
        </div>
      )}
    </div>
  );
}

export default function Darstellung(props: {
  art: Darstellungsart;
  url: string;
  dateiname: string;
  anhangId: string;
  seiteId?: string;
}) {
  switch (props.art) {
    case "html":
      return <HtmlAnsicht url={props.url} titel={props.dateiname} />;
    case "markdown":
      return <MarkdownAnsicht url={props.url} />;
    case "csv":
      return <CsvAnsicht url={props.url} dateiname={props.dateiname} />;
    case "tabelle":
      return <TabellenAnsicht seiteId={props.seiteId} anhangId={props.anhangId} />;
    case "folien":
      return <FolienAnsicht seiteId={props.seiteId} anhangId={props.anhangId} />;
  }
}
