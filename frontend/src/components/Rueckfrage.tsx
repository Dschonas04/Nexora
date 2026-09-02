// Confirmations before consequential steps.
//
// Before this the browser asked them: window.confirm. That is reliable, but it
// is not this application; the box looks different in every browser, accepts no
// styling, names the service by its domain name and blocks everything running in
// the background on the side.
//
// Here the same question stands in the application's window: with a title, an
// explanation and a button that names what it does ("Löschen") instead of "OK".
//
// It is used like confirm, only with await:
//
//     const frage = useRueckfrage();
//     if (!(await frage({ text: "Wirklich?", bestaetigen: "Löschen" }))) return;
//
// The promise is only resolved once somebody has answered.
import { ReactNode, createContext, useCallback, useContext, useEffect, useRef, useState } from "react";

export interface Rueckfrage {
  text: string;
  titel?: string;
  bestaetigen?: string;
  abbrechen?: string;
  // Colours the confirming button as a warning. For everything that removes
  // data.
  gefaehrlich?: boolean;
}

// The same shell, only with a field in it: for the cases where window.prompt
// used to be called. The answer is the text, or null on cancel.
export interface Eingabe {
  titel?: string;
  text?: string;
  feld?: string;
  vorgabe?: string;
  bestaetigen?: string;
  // "passwort" verdeckt die Eingabe und lässt sie ungetrimmt stehen: ein
  // Leerzeichen am Ende eines Passworts gehört dazu, auch wenn es meist ein
  // Versehen ist. Bei allem anderen wird getrimmt wie bisher.
  art?: "text" | "passwort";
}

type Antwort = (a: boolean | string | null) => void;

interface Dialog extends Rueckfrage {
  mitFeld?: boolean;
  feld?: string;
  vorgabe?: string;
  art?: "text" | "passwort";
}

const Ctx = createContext<{
  frage: (f: Rueckfrage) => Promise<boolean>;
  eingabe: (e: Eingabe) => Promise<string | null>;
}>({ frage: async () => false, eingabe: async () => null });

export function useRueckfrage() {
  return useContext(Ctx).frage;
}

export function useEingabe() {
  return useContext(Ctx).eingabe;
}

export function RueckfrageProvider({ children }: { children: ReactNode }) {
  const [offen, setOffen] = useState<Dialog | null>(null);
  const [wert, setWert] = useState("");
  // As a ref, not as state: resolving the promise shall not trigger another
  // render, and it must not get lost in between.
  const antwortRef = useRef<Antwort | null>(null);
  const jaRef = useRef<HTMLButtonElement>(null);
  const feldRef = useRef<HTMLInputElement>(null);

  const frage = useCallback((f: Rueckfrage) => {
    setOffen(f);
    return new Promise<boolean>((auf) => {
      antwortRef.current = auf as Antwort;
    });
  }, []);

  const eingabe = useCallback((e: Eingabe) => {
    setWert(e.vorgabe ?? "");
    setOffen({
      titel: e.titel,
      text: e.text ?? "",
      feld: e.feld,
      vorgabe: e.vorgabe,
      mitFeld: true,
      bestaetigen: e.bestaetigen,
      art: e.art,
    });
    return new Promise<string | null>((auf) => {
      antwortRef.current = auf as Antwort;
    });
  }, []);

  // One answer, two shapes: the yes/no dialog resolves with true or false, the
  // one with a field with the text or null. Both run through the same place so
  // that an open promise cannot be resolved along two paths.
  const schliessen = useCallback(
    (ja: boolean, text?: string) => {
      const mitFeld = offen?.mitFeld;
      setOffen(null);
      const auf = antwortRef.current;
      antwortRef.current = null;
      if (!auf) return;
      auf(mitFeld ? (ja ? (text ?? "") : null) : ja);
    },
    [offen],
  );

  // Was aus dem Feld herauskommt: bei einem Passwort unverändert, sonst ohne
  // Leerraum an den Rändern. An einer Stelle, damit Tastatur und Knopf nicht
  // verschieden antworten.
  const ausFeld = (roh: string) => (offen?.art === "passwort" ? roh : roh.trim());

  // The keyboard branch reads the field content through a reference: otherwise
  // the listener would have to be registered anew on every character.
  const wertRef = useRef("");
  wertRef.current = wert;

  // Keyboard: Esc cancels, Enter confirms. Both are what one expects here, and
  // without the keyboard the dialog would be unusable for everybody who cannot
  // point.
  useEffect(() => {
    if (!offen) return;
    if (offen.mitFeld) feldRef.current?.select();
    else jaRef.current?.focus();
    const auf = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        schliessen(false);
      } else if (e.key === "Enter") {
        e.preventDefault();
        // With the field its content counts, and empty means: nothing to do.
        if (offen.mitFeld) {
          const t = ausFeld(wertRef.current);
          if (t) schliessen(true, t);
        } else {
          schliessen(true);
        }
      }
    };
    window.addEventListener("keydown", auf);
    return () => window.removeEventListener("keydown", auf);
  }, [offen, schliessen]);

  return (
    <Ctx.Provider value={{ frage, eingabe }}>
      {children}
      {offen && (
        // A click beside it cancels, like the cross, only faster. With a
        // confirmation that is harmless: cancelling is the safe answer.
        <div className="modal-backdrop" onClick={() => schliessen(false)}>
          <div
            className="modal rueckfrage"
            role="alertdialog"
            aria-modal="true"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="modal-header">
              <h3>{offen.titel ?? "Bitte bestätigen"}</h3>
            </div>
            <div className="modal-section">
              {offen.text && <p className="rueckfrage-text">{offen.text}</p>}
              {offen.mitFeld && (
                <>
                  {offen.feld && <div className="modal-label">{offen.feld}</div>}
                  <input
                    ref={feldRef}
                    className="rueckfrage-feld"
                    type={offen.art === "passwort" ? "password" : "text"}
                    autoComplete={offen.art === "passwort" ? "new-password" : undefined}
                    value={wert}
                    onChange={(e) => setWert(e.target.value)}
                  />
                </>
              )}
            </div>
            <div className="rueckfrage-knoepfe">
              <button className="btn" onClick={() => schliessen(false)}>
                {offen.abbrechen ?? "Abbrechen"}
              </button>
              <button
                ref={jaRef}
                className={"btn" + (offen.gefaehrlich ? " warnend" : " betont")}
                disabled={offen.mitFeld && ausFeld(wert) === ""}
                onClick={() => schliessen(true, ausFeld(wert))}
              >
                {offen.bestaetigen ?? "Fortfahren"}
              </button>
            </div>
          </div>
        </div>
      )}
    </Ctx.Provider>
  );
}
