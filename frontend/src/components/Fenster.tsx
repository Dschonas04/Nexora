// A dialog above the page.
//
// The administration used to have its forms standing one below the other on the
// page: first the form for creating, the list below it. That costs the upper
// half of the screen for something needed once a day, and pushes the list one
// is actually after past the edge. What is rarely needed belongs in a dialog
// that opens when one calls it.
//
// Rueckfrage.tsx stays beside it: there it is about exactly one question with
// two answers, here about a form of any length. Both use the same marks from
// the stylesheet, so they do not drift apart.
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
  /** A line under the title, for what the dialog does. */
  unter?: string;
  /** For dialogs with a list in them that would not be readable in 480 pixels. */
  breit?: boolean;
  schliessen: () => void;
  /** The row of buttons at the bottom. Without it the dialog stands there
    unfinished. */
  fuss?: ReactNode;
  children: ReactNode;
}) {
  const kasten = useRef<HTMLDivElement>(null);

  // Esc closes. Without the key the only way out would be the mouse, and a
  // dialog one cannot get out of with the keyboard is a trap for everybody who
  // cannot point.
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

  // The pointer on the first field. Whoever opens a dialog in order to enter
  // something should not have to click into it first.
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
          {/* The cross sits in the head and not among the buttons at the
              bottom: cancel and close are the same thing, and two ways to do it
              in one row read like two different matters. */}
          <button className="fenster-zu" onClick={schliessen} aria-label="Close">
            ×
          </button>
        </div>
        <div className="fenster-inhalt">{children}</div>
        {fuss && <div className="fenster-fuss">{fuss}</div>}
      </div>
    </div>
  );
}
