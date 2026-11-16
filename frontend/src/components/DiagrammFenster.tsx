// The diagram dialog: Mermaid text on the left, the picture on the right, and
// for pie, bar and line charts a table instead of the text.
//
// Before this, a diagram was edited inside the code block with the picture
// below it, and a typo left only Mermaid's first error line. Here the picture
// follows the text while typing, and an error keeps the last good picture in
// view so the place one is looking at does not jump.
//
// Opened by a click on the picture under a Mermaid block (Editor.tsx). Without
// write access the dialog only shows: a large picture and the text to copy.
import { useEffect, useMemo, useRef, useState } from "react";

import Fenster from "./Fenster";
import { diagrammZeichnen } from "./Diagramm";
import { Diagrammart, Diagrammdaten, diagrammLesen, diagrammSchreiben } from "./DiagrammDaten";

const ARTEN: { art: Diagrammart; name: string }[] = [
  { art: "kreis", name: "Kreis" },
  { art: "balken", name: "Balken" },
  { art: "linie", name: "Linie" },
];

export default function DiagrammFenster({
  code,
  editierbar,
  schliessen,
  uebernehmen,
}: {
  code: string;
  editierbar: boolean;
  schliessen: () => void;
  uebernehmen: (code: string) => void;
}) {
  const [text, setText] = useState(code);
  const lesbar = useMemo(() => diagrammLesen(text), [text]);
  // The table keeps its own rows: written back to text and read again, a
  // half-typed "1," would turn into "1" under the cursor.
  const [daten, setDaten] = useState<Diagrammdaten | null>(() => (editierbar ? diagrammLesen(code) : null));
  const [ansicht, setAnsicht] = useState<"tabelle" | "text">(daten ? "tabelle" : "text");
  const [bild, setBild] = useState<string | null>(null);
  const [fehler, setFehler] = useState("");
  const nummer = useRef(0);

  // Live preview, a moment after the last keystroke. A counter drops answers
  // that arrive after a newer text was already sent.
  useEffect(() => {
    const meine = ++nummer.current;
    const t = window.setTimeout(() => {
      void diagrammZeichnen(text).then((e) => {
        if (meine !== nummer.current) return;
        if (e.svg !== undefined) {
          setBild("data:image/svg+xml;charset=utf-8," + encodeURIComponent(e.svg));
          setFehler("");
        } else {
          // Line, excerpt and caret, without Mermaid's list of expected tokens.
          setFehler((e.fehler ?? "").split("\n").slice(0, 3).join("\n"));
        }
      });
    }, 250);
    return () => window.clearTimeout(t);
  }, [text]);

  const aendern = (neu: Diagrammdaten) => {
    setDaten(neu);
    setText(diagrammSchreiben(neu));
  };

  const zurTabelle = () => {
    if (!lesbar) return;
    setDaten(lesbar);
    setAnsicht("tabelle");
  };

  const zeileAendern = (i: number, feld: "name" | "wert", wert: string) => {
    if (!daten) return;
    aendern({ ...daten, zeilen: daten.zeilen.map((z, j) => (j === i ? { ...z, [feld]: wert } : z)) });
  };

  const fuss = editierbar ? (
    <>
      <button className="btn" onClick={schliessen}>
        Abbrechen
      </button>
      <button className="btn btn-primary" onClick={() => uebernehmen(text)} disabled={text === code}>
        Übernehmen
      </button>
    </>
  ) : (
    <button className="btn" onClick={schliessen}>
      Schließen
    </button>
  );

  return (
    <Fenster titel="Diagramm" breit schliessen={schliessen} fuss={fuss}>
      <div className="diagramm-fenster">
        <div className="diagramm-fenster-quelle">
          {editierbar && (
            <div className="diagramm-reiter" role="tablist">
              <button
                role="tab"
                aria-selected={ansicht === "tabelle"}
                className={ansicht === "tabelle" ? "aktiv" : ""}
                disabled={!lesbar && ansicht !== "tabelle"}
                title={lesbar ? undefined : "Nur Kreis-, Balken- und Liniendiagramme mit einer Datenreihe"}
                onClick={zurTabelle}
              >
                Tabelle
              </button>
              <button
                role="tab"
                aria-selected={ansicht === "text"}
                className={ansicht === "text" ? "aktiv" : ""}
                onClick={() => setAnsicht("text")}
              >
                Text
              </button>
              <a
                className="diagramm-hilfe"
                href="https://mermaid.js.org/intro/syntax-reference.html"
                target="_blank"
                rel="noreferrer"
              >
                Mermaid-Syntax
              </a>
            </div>
          )}

          {ansicht === "tabelle" && daten ? (
            <div className="diagramm-tabelle">
              <div className="diagramm-arten">
                {ARTEN.map((a) => (
                  <button
                    key={a.art}
                    className={"btn" + (daten.art === a.art ? " btn-primary" : "")}
                    onClick={() => aendern({ ...daten, art: a.art })}
                  >
                    {a.name}
                  </button>
                ))}
              </div>
              <label>
                <span>Titel</span>
                <input value={daten.titel} onChange={(e) => aendern({ ...daten, titel: e.target.value })} />
              </label>
              {daten.art !== "kreis" && (
                <div className="diagramm-achsen">
                  <label>
                    <span>X-Achse</span>
                    <input value={daten.xAchse} onChange={(e) => aendern({ ...daten, xAchse: e.target.value })} />
                  </label>
                  <label>
                    <span>Y-Achse</span>
                    <input value={daten.yAchse} onChange={(e) => aendern({ ...daten, yAchse: e.target.value })} />
                  </label>
                </div>
              )}
              <table>
                <thead>
                  <tr>
                    <th>Bezeichnung</th>
                    <th>Wert</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {daten.zeilen.map((z, i) => (
                    <tr key={i}>
                      <td>
                        <input
                          value={z.name}
                          aria-label="Bezeichnung"
                          onChange={(e) => zeileAendern(i, "name", e.target.value)}
                        />
                      </td>
                      <td>
                        <input
                          value={z.wert}
                          aria-label="Wert"
                          inputMode="decimal"
                          className={z.wert.trim() && !/^-?\d+(?:[.,]\d*)?$/.test(z.wert.trim()) ? "ungueltig" : ""}
                          onChange={(e) => zeileAendern(i, "wert", e.target.value)}
                        />
                      </td>
                      <td>
                        <button
                          className="btn diagramm-zeile-weg"
                          aria-label="Zeile entfernen"
                          title="Zeile entfernen"
                          onClick={() => aendern({ ...daten, zeilen: daten.zeilen.filter((_, j) => j !== i) })}
                        >
                          ×
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <button
                className="btn"
                onClick={() => aendern({ ...daten, zeilen: [...daten.zeilen, { name: "", wert: "" }] })}
              >
                Zeile hinzufügen
              </button>
            </div>
          ) : (
            <textarea
              className="diagramm-text"
              value={text}
              readOnly={!editierbar}
              spellCheck={false}
              onChange={(e) => setText(e.target.value)}
            />
          )}
        </div>

        <div className="diagramm-fenster-bild notranslate">
          {bild && <img src={bild} alt="Diagramm" className={fehler ? "veraltet" : ""} />}
          {fehler && <div className="diagramm-fenster-fehler">{fehler}</div>}
        </div>
      </div>
    </Fenster>
  );
}
