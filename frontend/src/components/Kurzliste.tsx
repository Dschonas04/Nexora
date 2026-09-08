// Lange Listen kurz halten.
//
// Die Tabellen der Verwaltung wachsen mit dem Betrieb: Anmeldeversuche,
// Sitzungen, Protokolleinträge, Konten. Nach ein paar Wochen füllt eine
// einzige davon den Bildschirm, und wer darunter etwas sucht, rollt an ihr
// vorbei -- auf einer Seite, die vier weitere Sachgruppen trägt.
//
// Deshalb stehen zunächst nur die ersten fünf Zeilen da, und das sind die
// neuen: alle diese Listen kommen absteigend sortiert aus dem Backend. Der
// Rest ist einen Klick entfernt und bleibt offen, solange man auf der Seite
// bleibt.
//
// Es ist eine Komponente und kein Hook, weil die Abschnitte der Verwaltung in
// einem switch stehen: ein Hook dürfte dort nicht aufgerufen werden, eine
// Komponente bringt ihren eigenen Zustand mit.
import { ReactNode, useState } from "react";

export const KURZ = 5;

export default function KurzeZeilen<T>({
  alle,
  spalten,
  zeile,
  grenze = KURZ,
}: {
  alle: T[];
  /** Wie viele Spalten die Tabelle hat, damit die Zeile mit dem Knopf durchläuft. */
  spalten: number;
  zeile: (eintrag: T, index: number) => ReactNode;
  grenze?: number;
}) {
  const [offen, setOffen] = useState(false);
  const sichtbar = offen ? alle : alle.slice(0, grenze);
  const rest = alle.length - grenze;

  return (
    <>
      {sichtbar.map((e, i) => zeile(e, i))}
      {rest > 0 && (
        <tr className="kurz-mehr">
          <td colSpan={spalten}>
            <button className="btn-schlicht" onClick={() => setOffen(!offen)}>
              {offen ? "Weniger anzeigen" : `${rest} weitere anzeigen`}
            </button>
          </td>
        </tr>
      )}
    </>
  );
}
