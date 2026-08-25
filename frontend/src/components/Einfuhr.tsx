// Importing Markdown into the workspace.
//
// The dialog does three things and says which one it is doing: pick files, wait
// while they are read, show what came of it. The report at the end is the point
// -- an import that silently skipped eleven files looks exactly like one that
// worked, and the difference only surfaces weeks later when something is
// missing.
import { useEffect, useRef, useState } from "react";

import { EinfuhrAst, EinfuhrBericht, EinfuhrVorschau, api } from "../api/client";

export default function Einfuhr({
  ziel,
  zielName,
  onFertig,
  onClose,
}: {
  // Genau eines von beidem: unter eine Seite oder in eine Ablage. Ist keines
  // gesetzt, landet die Einfuhr an der Wurzel, und nur dann darf sie
  // stattdessen eine eigene Ablage mitbringen.
  ziel: { parentId?: string; spaceId?: string };
  zielName: string;
  onFertig: (bericht: EinfuhrBericht) => void;
  onClose: () => void;
}) {
  const [laeuft, setLaeuft] = useState(false);
  const [vorschau, setVorschau] = useState<EinfuhrVorschau | null>(null);
  // Die gewählten Dateien werden festgehalten, bis die Vorschau bestätigt ist
  // -- sonst müsste man sie zum Einführen ein zweites Mal auswählen.
  const [dateien, setDateien] = useState<File[]>([]);
  const [bericht, setBericht] = useState<EinfuhrBericht | null>(null);
  const [fehler, setFehler] = useState<string | null>(null);
  const [ueber, setUeber] = useState(false);
  const wahl = useRef<HTMLInputElement>(null);
  // Eine ganze Ablage einführen. Möglich nur an der Wurzel: in einer Ablage
  // eine zweite anzulegen ergäbe keine Ordnung, sondern zwei.
  const alsAblageMoeglich = !ziel.parentId && !ziel.spaceId;
  const [alsAblage, setAlsAblage] = useState(false);
  const [ablageName, setAblageName] = useState("");
  // Zähler statt Schalter: über einem Kindelement feuert dragleave, obwohl der
  // Zeiger den Kasten nie verlassen hat.
  const tiefe = useRef(0);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && !laeuft && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, laeuft]);

  // Erst rechnen lassen, dann fragen. Zweihundert Seiten rückgängig zu machen
  // hieße, sie einzeln in den Papierkorb zu schieben, die Vorschau kostet
  // einen Klick und erspart genau das.
  // Aus "Homelab.zip" wird "Homelab": der Name, unter dem die Ablage
  // ausgeführt wurde, ist der beste Vorschlag für die, die daraus entsteht.
  const nameAusDatei = (dateien: File[]) =>
    (dateien[0]?.name ?? "").replace(/\.[^.]+$/, "").replace(/[_-]+/g, " ").trim();

  const pruefen = async (gewaehlt: File[]) => {
    if (gewaehlt.length === 0) return;
    setLaeuft(true);
    setFehler(null);
    const name = alsAblage ? ablageName.trim() || nameAusDatei(gewaehlt) : "";
    if (alsAblage && !ablageName.trim() && name) setAblageName(name);
    try {
      const v = await api.importieren(gewaehlt, { ...ziel, neueAblage: name }, true);
      setDateien(gewaehlt);
      setVorschau(v);
    } catch (e) {
      setFehler((e as Error).message);
    } finally {
      setLaeuft(false);
    }
  };

  const einfuehren = async () => {
    setLaeuft(true);
    setFehler(null);
    try {
      const b = await api.importieren(dateien, {
        ...ziel,
        neueAblage: alsAblage ? ablageName.trim() || nameAusDatei(dateien) : undefined,
      });
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
          <span className="qv-title">
            Einfuhr nach „{alsAblage ? ablageName.trim() || "neuer Ablage" : zielName}“
          </span>
          <div className="qv-actions">
            <button className="btn" disabled={laeuft} onClick={onClose}>
              {bericht ? "Fertig" : "Abbrechen"}
            </button>
          </div>
        </div>

        <div className="einfuhr-inhalt">
          {fehler && <div className="fehler">{fehler}</div>}

          {!bericht && !vorschau && alsAblageMoeglich && (
            <div className="einfuhr-ziel">
              <label className="einfuhr-schalter">
                <input
                  type="checkbox"
                  checked={alsAblage}
                  onChange={(e) => setAlsAblage(e.target.checked)}
                />
                <span>Als eigene Ablage anlegen</span>
              </label>
              {alsAblage ? (
                <input
                  className="rueckfrage-feld"
                  placeholder="Name der Ablage (sonst der Dateiname)"
                  value={ablageName}
                  onChange={(e) => setAblageName(e.target.value)}
                />
              ) : (
                <p className="muted small">
                  Ohne Haken landen die Seiten unter „Ohne Ablage“. Mit Haken entsteht eine
                  neue Ablage, so kommt eine ausgeführte Ablage als Ganzes zurück.
                </p>
              )}
            </div>
          )}

          {!bericht && !vorschau && (
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
                  pruefen(Array.from(e.dataTransfer.files));
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
                accept=".md,.markdown,.mdown,.mdx,.txt,.html,.htm,.zip"
                style={{ display: "none" }}
                onChange={(e) => {
                  pruefen(Array.from(e.target.files ?? []));
                  e.target.value = "";
                }}
              />

              <div className="muted small einfuhr-hinweis">
                <p>
                  <strong>Einzelne .md- oder .html-Dateien</strong> werden je zu einer Seite. Ein{" "}
                  <strong>.zip</strong> behält seinen Aufbau: aus jedem Ordner wird eine Seite,
                  aus den Dateien darin ihre Unterseiten. Liegt im Ordner eine{" "}
                  <code>index.md</code>, <code>README.md</code> oder <code>INHALT.md</code>, ist
                  sie der Inhalt der Ordnerseite.
                </p>
                <p>
                  Damit lassen sich ein <strong>Obsidian</strong>-Tresor, eine{" "}
                  <strong>Notion</strong>-Ausfuhr (die Kennung im Dateinamen fällt weg) und ein{" "}
                  <strong>Confluence</strong>-Export aus HTML-Dateien einlesen.
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

          {vorschau && !bericht && (
            <div className="einfuhr-bericht">
              <p className="einfuhr-gross">
                {vorschau.seiten} {vorschau.seiten === 1 ? "Seite" : "Seiten"} würden entstehen
                {vorschau.beilagen > 0 &&
                  `, ${vorschau.beilagen} ${vorschau.beilagen === 1 ? "Datei" : "Dateien"} werden Anhänge`}
                {vorschau.ablage && ` — in der neuen Ablage „${vorschau.ablage}“`}.
              </p>
              <div className="einfuhr-baum">
                <Aeste knoten={vorschau.baum} />
              </div>
              {vorschau.warnungen.length > 0 && (
                <>
                  <h4 className="rechte-ueberschrift">Übergangen</h4>
                  <ul className="einfuhr-warnungen">
                    {vorschau.warnungen.map((w, i) => (
                      <li key={i}>{w}</li>
                    ))}
                  </ul>
                </>
              )}
              <div className="rechte-abschluss">
                <button
                  className="btn"
                  disabled={laeuft}
                  onClick={() => {
                    setVorschau(null);
                    setDateien([]);
                  }}
                >
                  Andere Dateien
                </button>
                <button className="btn btn-primary" disabled={laeuft} onClick={einfuehren}>
                  {laeuft ? "Wird angelegt …" : "Einführen"}
                </button>
              </div>
            </div>
          )}

          {bericht && (
            <div className="einfuhr-bericht">
              <p className="einfuhr-gross">
                {bericht.seiten} {bericht.seiten === 1 ? "Seite" : "Seiten"} angelegt
                {bericht.anhaenge > 0 &&
                  `, ${bericht.anhaenge} ${bericht.anhaenge === 1 ? "Anhang" : "Anhänge"} übernommen`}
                {bericht.ablage && ` in der neuen Ablage „${bericht.ablage.name}“`}.
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

// Aeste zeigt den geplanten Baum. Die Quelldatei steht klein daneben: ein Titel
// allein verrät nicht, woher er kommt, und genau das will man wissen, wenn
// eine Seite an unerwarteter Stelle steht.
function Aeste({ knoten }: { knoten: EinfuhrAst[] }) {
  return (
    <ul className="einfuhr-aeste">
      {knoten.map((k, i) => (
        <li key={i}>
          <span>{k.titel}</span>
          {k.quelle && <span className="muted small"> · {k.quelle}</span>}
          {k.kinder && k.kinder.length > 0 && <Aeste knoten={k.kinder} />}
        </li>
      ))}
    </ul>
  );
}
