// Ein Fenster über der Seite.
//
// Die Verwaltung hatte ihre Formulare bisher untereinander auf der Seite
// stehen: erst das Formular zum Anlegen, darunter die Liste. Das kostet die
// obere Hälfte des Bildschirms für etwas, das man einmal am Tag braucht, und
// drängt die Liste, um die es eigentlich geht, unter den Rand. Was selten
// gebraucht wird, gehört in ein Fenster, das aufgeht, wenn man es ruft.
//
// Rueckfrage.tsx bleibt daneben stehen: dort geht es um genau eine Frage mit
// zwei Antworten, hier um ein Formular beliebiger Länge. Beide benutzen
// dieselben Marken aus dem Stilblatt, damit sie nicht auseinanderlaufen.
import { ReactNode, useEffect, useRef } from "react";

export default function Fenster({
  titel,
  unter,
  breit,
  schliessen,
  fuss,
  children,
}: {
  titel: string;
  /** Eine Zeile unter dem Titel, für das, was das Fenster tut. */
  unter?: string;
  /** Für Fenster mit einer Liste darin, die in 480 Pixeln nicht lesbar wäre. */
  breit?: boolean;
  schliessen: () => void;
  /** Die Knopfreihe unten. Ohne sie steht das Fenster ohne Abschluss da. */
  fuss?: ReactNode;
  children: ReactNode;
}) {
  const kasten = useRef<HTMLDivElement>(null);

  // Esc schließt. Ohne die Taste bliebe als Ausweg nur die Maus, und ein
  // Fenster, aus dem man nicht mit der Tastatur herauskommt, ist eine Falle für
  // jeden, der nicht zeigen kann.
  useEffect(() => {
    const auf = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        schliessen();
      }
    };
    window.addEventListener("keydown", auf);
    return () => window.removeEventListener("keydown", auf);
  }, [schliessen]);

  // Der Zeiger auf das erste Feld. Wer ein Fenster öffnet, um etwas
  // einzutragen, soll nicht erst hineinklicken müssen.
  useEffect(() => {
    const erstes = kasten.current?.querySelector<HTMLElement>(
      "input:not([type=hidden]):not([disabled]), textarea, select",
    );
    erstes?.focus();
  }, []);

  return (
    <div className="modal-backdrop" onClick={schliessen}>
      <div
        ref={kasten}
        className={"modal fenster" + (breit ? " fenster-breit" : "")}
        role="dialog"
        aria-modal="true"
        aria-label={titel}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-header">
          <div>
            <h3>{titel}</h3>
            {unter && <div className="muted small">{unter}</div>}
          </div>
          {/* Das Kreuz sitzt im Kopf und nicht bei den Knöpfen unten:
              Abbrechen und Schließen sind dasselbe, und zwei Wege dafür in
              einer Reihe lesen sich wie zwei verschiedene Sachen. */}
          <button className="fenster-zu" onClick={schliessen} aria-label="Schließen">
            ×
          </button>
        </div>
        <div className="fenster-inhalt">{children}</div>
        {fuss && <div className="fenster-fuss">{fuss}</div>}
      </div>
    </div>
  );
}
