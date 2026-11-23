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

export type Darstellungsart = "html" | "markdown" | "csv" | "tabelle";

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
  }
}
