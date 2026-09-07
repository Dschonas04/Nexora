// Der Kopf über einer Liste in der Verwaltung.
//
// Jede Liste dort beantwortet dieselben drei Fragen, und sie standen bisher an
// jeder Stelle anders: wie viel ist es (die Zahl gehört an die Überschrift, weil
// eine Verwaltung sie ohnehin zählt, sobald sie die Tabelle sieht), wie finde
// ich eine Zeile (ein Feld schlägt ab etwa zwanzig Zeilen das Blättern), und
// was kann ich hier anlegen (ein Knopf, kein Formular über der Liste).
//
// Als eigene Komponente und nicht als sechsmal abgeschriebenes Markup: sonst
// ist es kein Muster, sondern sechs Stellen, die auseinanderlaufen.
import { ReactNode } from "react";

export default function Listenkopf({
  titel,
  zahl,
  filter,
  setFilter,
  platzhalter,
  children,
}: {
  titel: string;
  /** Die Zahl neben der Überschrift. Weggelassen, wo es nichts zu zählen gibt. */
  zahl?: ReactNode;
  /** Zusammen mit setFilter: das Filterfeld rechts. Beides oder keines. */
  filter?: string;
  setFilter?: (v: string) => void;
  platzhalter?: string;
  /** Was rechts neben dem Filter steht -- in aller Regel ein Knopf. */
  children?: ReactNode;
}) {
  return (
    <div className="listenkopf">
      <h3>
        {titel}
        {zahl !== undefined && <span className="muted small"> {zahl}</span>}
      </h3>
      <div className="listenkopf-werkzeug">
        {setFilter && (
          <input
            className="listenfilter"
            placeholder={platzhalter ?? "Filtern"}
            value={filter ?? ""}
            onChange={(e) => setFilter(e.target.value)}
          />
        )}
        {children}
      </div>
    </div>
  );
}
