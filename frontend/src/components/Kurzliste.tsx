// Keeping long lists short.
//
// The administration's tables grow with operation: sign-in attempts, sessions,
// audit entries, accounts. After a few weeks a single one of them fills the
// screen, and whoever is looking for something below it scrolls past it -- on a
// page carrying four more topic groups.
//
// That is why only the first five rows stand there at first, and those are the
// new ones: all these lists come out of the backend sorted descending. The rest
// is one click away and stays open as long as one stays on the page.
//
// It is a component and not a hook, because the administration's sections sit
// in a switch: a hook may not be called there, a component brings its own state
// along.
import { ReactNode, useState } from "react";

export const KURZ = 5;

export default function KurzeZeilen<T>({
  alle,
  spalten,
  zeile,
  grenze = KURZ,
}: {
  alle: T[];
  /** How many columns the table has, so the row with the button runs through. */
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
