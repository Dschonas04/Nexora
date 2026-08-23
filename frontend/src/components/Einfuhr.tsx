// Importing Markdown into the workspace.
//
// The dialog does three things and says which one it is doing: pick files, wait
// while they are read, show what came of it. The report at the end is the point
// -- an import that silently skipped eleven files looks exactly like one that
// worked, and the difference only surfaces weeks later when something is
// missing.
import { useEffect, useRef, useState } from "react";

import { EinfuhrBericht, api } from "../api/client";

export default function Einfuhr({
  ziel,
  zielName,
  onFertig,
  onClose,
}: {
  // Genau eines von beidem: unter eine Seite oder in eine Ablage.
  ziel: { parentId?: string; spaceId?: string };
  zielName: string;
  onFertig: (bericht: EinfuhrBericht) => void;
  onClose: () => void;
}) {
  const [laeuft, setLaeuft] = useState(false);
  const [bericht, setBericht] = useState<EinfuhrBericht | null>(null);
  const [fehler, setFehler] = useState<string | null>(null);
  const [ueber, setUeber] = useState(false);
  const wahl = useRef<HTMLInputElement>(null);
  // Zähler statt Schalter: über einem Kindelement feuert dragleave, obwohl der
  // Zeiger den Kasten nie verlassen hat.
  const tiefe = useRef(0);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && !laeuft && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, laeuft]);

  const schicken = async (dateien: File[]) => {
    if (dateien.length === 0) return;
    setLaeuft(true);
    setFehler(null);
    try {
      const b = await api.importieren(dateien, ziel);
      setBericht(b);
      onFertig(b);
    } catch (e) {
      setFehler((e as Error).message);
    } finally {
      setLaeuft(false);
    }
  };

  return (
    <div className="qv-overlay" onClick={() => !laeuft && onClose()}>
      <div className="qv-box einfuhr-box" onClick={(e) => e.stopPropagation()}>
        <div className="qv-head">
          <span className="qv-title">Einfuhr nach „{zielName}“</span>
          <div className="qv-actions">
            <button className="btn" disabled={laeuft} onClick={onClose}>
              {bericht ? "Fertig" : "Abbrechen"}
            </button>
          </div>
        </div>

        <div className="einfuhr-inhalt">
          {fehler && <div className="fehler">{fehler}</div>}

          {!bericht && (
            <>
              <div
                className={"einfuhr-feld" + (ueber ? " ueber" : "")}
                onDragEnter={(e) => {
                  e.preventDefault();
                  tiefe.current++;
                  setUeber(true);
                }}
                onDragOver={(e) => e.preventDefault()}
                onDragLeave={() => {
                  tiefe.current--;
                  if (tiefe.current <= 0) setUeber(false);
                }}
                onDrop={(e) => {
                  e.preventDefault();
                  tiefe.current = 0;
                  setUeber(false);
                  schicken(Array.from(e.dataTransfer.files));
                }}
                onClick={() => !laeuft && wahl.current?.click()}
              >
                {laeuft ? (
                  <div>Wird gelesen …</div>
                ) : (
                  <>
                    <div className="einfuhr-gross">Dateien hierher ziehen</div>
                    <div className="muted small">oder klicken, um sie auszuwählen</div>
                  </>
                )}
              </div>
              <input
                ref={wahl}
                type="file"
                multiple
                accept=".md,.markdown,.mdown,.mdx,.txt,.zip"
                style={{ display: "none" }}
                onChange={(e) => {
                  schicken(Array.from(e.target.files ?? []));
                  e.target.value = "";
                }}
              />

              <div className="muted small einfuhr-hinweis">
                <p>
                  <strong>Einzelne .md-Dateien</strong> werden je zu einer Seite. Ein{" "}
                  <strong>.zip</strong> behält seinen Aufbau: aus jedem Ordner wird eine Seite,
                  aus den Dateien darin ihre Unterseiten. Liegt im Ordner eine{" "}
                  <code>index.md</code>, <code>README.md</code> oder <code>INHALT.md</code>, ist
                  sie der Inhalt der Ordnerseite.
                </p>
                <p>
                  Verweise zwischen eingeführten Dateien werden zu{" "}
                  <code>[[Seitentitel]]</code> und zählen damit für Rückverweise und das
                  Wissensnetz. Bilder und andere Dateien aus dem Archiv werden Anhänge der Seite,
                  die sie verwendet.
                </p>
                <p>
                  Ein Vorspann aus <code>---</code>-Zeilen liefert Titel, Schlagworte und Symbol.
                  Sonst ist der Titel die erste Überschrift der Datei, ersatzweise ihr Dateiname.
                </p>
              </div>
            </>
          )}

          {bericht && (
            <div className="einfuhr-bericht">
              <p className="einfuhr-gross">
                {bericht.seiten} {bericht.seiten === 1 ? "Seite" : "Seiten"} angelegt
                {bericht.anhaenge > 0 &&
                  `, ${bericht.anhaenge} ${bericht.anhaenge === 1 ? "Anhang" : "Anhänge"} übernommen`}
                .
              </p>
              {bericht.warnungen.length > 0 && (
                <>
                  <h4 className="rechte-ueberschrift">Übergangen</h4>
                  <ul className="einfuhr-warnungen">
                    {bericht.warnungen.map((w, i) => (
                      <li key={i}>{w}</li>
                    ))}
                  </ul>
                </>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
