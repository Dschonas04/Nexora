// The viewer's text editor: scripts, Markdown, configuration files and logs
// open as text with line numbers instead of "no preview". Whoever may write
// the page can edit and save; everybody else reads.
//
// Deliberately a plain text field and no code editor library: it loads
// instantly, never mangles a file (no auto-indent, no auto-close, no trailing
// whitespace cleanup) and behaves like the editors people already know.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";

interface Props {
  url: string;
  dateiname: string;
  anhangId: string;
  seiteId?: string;
  darfSchreiben?: boolean;
  /** Reports unsaved changes, so the viewer can ask before closing. */
  onUngespeichert?: (ja: boolean) => void;
}

// Above this the text field gets sluggish; such files are shown, not edited.
const MAX_BEARBEITEN = 2_000_000;
const MAX_ZEIGEN = 300_000;

const SPRACHEN: Record<string, string> = {
  sh: "Shell", bash: "Bash", zsh: "Zsh", fish: "Fish",
  bat: "Batch", cmd: "Batch", ps1: "PowerShell", psm1: "PowerShell", psd1: "PowerShell", vbs: "VBScript",
  md: "Markdown", markdown: "Markdown", txt: "Text", log: "Protokoll", csv: "CSV", tsv: "TSV",
  json: "JSON", jsonc: "JSON", xml: "XML", yml: "YAML", yaml: "YAML", toml: "TOML",
  ini: "INI", cfg: "Konfiguration", conf: "Konfiguration", env: "Umgebung", properties: "Properties",
  py: "Python", rb: "Ruby", pl: "Perl", php: "PHP", lua: "Lua", go: "Go", rs: "Rust",
  c: "C", h: "C", cpp: "C++", hpp: "C++", cs: "C#", java: "Java", kt: "Kotlin", swift: "Swift",
  js: "JavaScript", mjs: "JavaScript", ts: "TypeScript", tsx: "TypeScript", jsx: "JavaScript",
  css: "CSS", scss: "SCSS", html: "HTML", htm: "HTML", sql: "SQL", tf: "Terraform", hcl: "HCL",
  dockerfile: "Dockerfile", makefile: "Makefile",
};

function sprache(name: string): string {
  const basis = name.toLowerCase().split(/[\\/]/).pop() ?? "";
  if (SPRACHEN[basis]) return SPRACHEN[basis];
  const endung = basis.includes(".") ? basis.split(".").pop()! : "";
  return SPRACHEN[endung] ?? "Text";
}

// Windows scripts (.bat, .cmd, .ps1) keep CRLF line endings when they had
// them: cmd.exe misreads labels in a .bat with bare LF.
function zeilenende(text: string): "\r\n" | "\n" {
  return text.includes("\r\n") ? "\r\n" : "\n";
}

export default function TextEditor({ url, dateiname, anhangId, seiteId, darfSchreiben, onUngespeichert }: Props) {
  const [original, setOriginal] = useState<string | null>(null);
  const [inhalt, setInhalt] = useState("");
  const [fehler, setFehler] = useState<string | null>(null);
  const [zuGross, setZuGross] = useState(false);
  const [umbrechen, setUmbrechen] = useState(false);
  const [stand, setStand] = useState<"" | "speichert" | "gespeichert" | "kopiert">("");
  const [speicherFehler, setSpeicherFehler] = useState<string | null>(null);
  const feld = useRef<HTMLTextAreaElement>(null);
  const rand = useRef<HTMLPreElement>(null);
  const ende = useRef<"\r\n" | "\n">("\n");

  useEffect(() => {
    let weg = false;
    setOriginal(null);
    setFehler(null);
    setZuGross(false);
    setStand("");
    setSpeicherFehler(null);
    fetch(url, { credentials: "include" })
      .then((r) => {
        if (!r.ok) throw new Error(String(r.status));
        return r.text();
      })
      .then((t) => {
        if (weg) return;
        ende.current = zeilenende(t);
        const gross = t.length > MAX_BEARBEITEN;
        const sichtbar = (gross ? t.slice(0, MAX_ZEIGEN) : t).replace(/\r\n/g, "\n");
        setZuGross(gross);
        setOriginal(sichtbar);
        setInhalt(sichtbar);
      })
      .catch(() => !weg && setFehler("Die Datei konnte nicht geladen werden."));
    return () => {
      weg = true;
    };
  }, [url]);

  const bearbeitbar = !!darfSchreiben && !!seiteId && !zuGross && original !== null;
  const geaendert = original !== null && inhalt !== original;

  useEffect(() => {
    onUngespeichert?.(geaendert);
  }, [geaendert, onUngespeichert]);
  useEffect(() => () => onUngespeichert?.(false), [onUngespeichert]);

  // Warn before the tab closes with unsaved edits.
  useEffect(() => {
    if (!geaendert) return;
    const warnen = (e: BeforeUnloadEvent) => {
      e.preventDefault();
    };
    window.addEventListener("beforeunload", warnen);
    return () => window.removeEventListener("beforeunload", warnen);
  }, [geaendert]);

  const zeilen = useMemo(() => inhalt.split("\n").length, [inhalt]);
  const nummern = useMemo(
    () => Array.from({ length: zeilen }, (_, i) => i + 1).join("\n"),
    [zeilen],
  );

  const speichern = useCallback(async () => {
    if (!bearbeitbar || !geaendert || !seiteId) return;
    setStand("speichert");
    setSpeicherFehler(null);
    try {
      const raus = ende.current === "\r\n" ? inhalt.replace(/\n/g, "\r\n") : inhalt;
      await api.textErsetzen(seiteId, anhangId, raus);
      setOriginal(inhalt);
      setStand("gespeichert");
    } catch (e) {
      setSpeicherFehler((e as Error).message);
      setStand("");
    }
  }, [bearbeitbar, geaendert, seiteId, anhangId, inhalt]);

  const kopieren = async () => {
    try {
      await navigator.clipboard.writeText(inhalt);
      setStand("kopiert");
    } catch {
      feld.current?.select();
    }
  };

  const tasten = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
      e.preventDefault();
      void speichern();
      return;
    }
    // Tab indents instead of leaving the field, as in every editor.
    if (e.key === "Tab" && bearbeitbar && !e.ctrlKey && !e.altKey && !e.metaKey) {
      e.preventDefault();
      const f = e.currentTarget;
      const { selectionStart: a, selectionEnd: b, value } = f;
      const einzug = /\.(ya?ml|md|markdown)$/i.test(dateiname) ? "  " : "\t";
      if (a === b && !e.shiftKey) {
        const neu = value.slice(0, a) + einzug + value.slice(b);
        setInhalt(neu);
        requestAnimationFrame(() => {
          f.selectionStart = f.selectionEnd = a + einzug.length;
        });
        return;
      }
      // Several lines selected: indent or (with Shift) outdent all of them.
      const start = value.lastIndexOf("\n", a - 1) + 1;
      const block = value.slice(start, b);
      const neuBlock = e.shiftKey
        ? block.replace(/^(\t| {1,2})/gm, "")
        : block.replace(/^/gm, einzug);
      setInhalt(value.slice(0, start) + neuBlock + value.slice(b));
      requestAnimationFrame(() => {
        f.selectionStart = start;
        f.selectionEnd = start + neuBlock.length;
      });
    }
    if (e.key === "Escape") e.currentTarget.blur();
  };

  const scrollen = () => {
    if (rand.current && feld.current) rand.current.scrollTop = feld.current.scrollTop;
  };

  if (fehler) return <div className="qv-none">{fehler}</div>;
  if (original === null) return <div className="qv-none">Lädt…</div>;

  return (
    <div className="te">
      <div className="te-leiste">
        <span className="te-art">{sprache(dateiname)}</span>
        <span className="muted small">
          {zeilen.toLocaleString("de-DE")} Zeilen
          {ende.current === "\r\n" ? " · Windows-Zeilenenden" : ""}
        </span>
        <span className="te-platz" />
        {geaendert && <span className="te-geaendert small">nicht gespeichert</span>}
        {stand === "gespeichert" && !geaendert && <span className="hinweis-ok small">Gespeichert</span>}
        {stand === "kopiert" && <span className="muted small">Kopiert</span>}
        <label className="te-schalter small">
          <input type="checkbox" checked={umbrechen} onChange={(e) => setUmbrechen(e.target.checked)} />
          Umbrechen
        </label>
        <button className="btn" onClick={kopieren}>Kopieren</button>
        {bearbeitbar && (
          <>
            <button className="btn" disabled={!geaendert} onClick={() => setInhalt(original)}>
              Verwerfen
            </button>
            <button className="btn btn-primary" disabled={!geaendert || stand === "speichert"} onClick={() => void speichern()}>
              {stand === "speichert" ? "Speichert…" : "Speichern"}
            </button>
          </>
        )}
      </div>
      {speicherFehler && <div className="te-fehler small">Speichern ging nicht: {speicherFehler}</div>}
      {zuGross && (
        <div className="te-hinweis small muted">
          Die Datei ist sehr groß. Gezeigt wird der Anfang, bearbeiten lässt sie sich hier nicht.
        </div>
      )}
      {!bearbeitbar && !zuGross && darfSchreiben === false && (
        <div className="te-hinweis small muted">Nur lesen: Du hast auf dieser Seite keine Schreibrechte.</div>
      )}
      <div className={"te-flaeche" + (umbrechen ? " te-umbruch" : "")}>
        {!umbrechen && (
          <pre className="te-rand" ref={rand} aria-hidden="true">
            {nummern}
          </pre>
        )}
        <textarea
          ref={feld}
          className="te-feld"
          value={inhalt}
          readOnly={!bearbeitbar}
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          wrap={umbrechen ? "soft" : "off"}
          onChange={(e) => {
            setInhalt(e.target.value);
            if (stand !== "speichert") setStand("");
          }}
          onKeyDown={tasten}
          onScroll={scrollen}
          aria-label={`Inhalt von ${dateiname}`}
        />
      </div>
    </div>
  );
}
