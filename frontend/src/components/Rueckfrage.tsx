// Rückfragen vor folgenreichen Schritten.
//
// Vorher stellte der Browser sie: window.confirm. Das ist verlässlich, aber es
// ist nicht diese Anwendung, der Kasten sieht in jedem Browser anders aus,
// nimmt keine Gestaltung an, nennt den Dienst beim Domainnamen und blockiert
// nebenbei alles, was im Hintergrund läuft.
//
// Hier steht dieselbe Frage im Fenster der Anwendung: mit Titel, Erklärung und
// einem Knopf, der benennt, was er tut ("Löschen") statt "OK".
//
// Verwendet wird sie wie confirm, nur mit await:
//
//     const frage = useRueckfrage();
//     if (!(await frage({ text: "Wirklich?", bestaetigen: "Löschen" }))) return;
//
// Das Versprechen wird erst eingelöst, wenn jemand geantwortet hat.
import { ReactNode, createContext, useCallback, useContext, useEffect, useRef, useState } from "react";

export interface Rueckfrage {
  text: string;
  titel?: string;
  bestaetigen?: string;
  abbrechen?: string;
  // Färbt den bestätigenden Knopf als Warnung. Für alles, was Daten entfernt.
  gefaehrlich?: boolean;
}

// Dieselbe Hülle, nur mit einem Feld darin: für die Fälle, in denen bisher
// window.prompt aufgerufen wurde. Antwort ist der Text oder null beim Abbruch.
export interface Eingabe {
  titel?: string;
  text?: string;
  feld?: string;
  vorgabe?: string;
  bestaetigen?: string;
}

type Antwort = (a: boolean | string | null) => void;

interface Dialog extends Rueckfrage {
  mitFeld?: boolean;
  feld?: string;
  vorgabe?: string;
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
  // Als ref, nicht als Zustand: das Auflösen des Versprechens soll kein
  // erneutes Zeichnen auslösen, und es darf nicht zwischendurch verloren gehen.
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
    });
    return new Promise<string | null>((auf) => {
      antwortRef.current = auf as Antwort;
    });
  }, []);

  // Eine Antwort, zwei Formen: der Ja/Nein-Dialog löst mit true oder false auf,
  // der mit Feld mit dem Text oder null. Beides läuft durch dieselbe Stelle,
  // damit ein offenes Versprechen nicht auf zwei Wegen eingelöst werden kann.
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

  // Der Tastaturzweig liest den Feldinhalt über eine Referenz: sonst müsste
  // der Zuhörer bei jedem Zeichen neu angemeldet werden.
  const wertRef = useRef("");
  wertRef.current = wert;

  // Tastatur: Esc bricht ab, Enter bestätigt. Beides ist an dieser Stelle
  // erwartbar, und ohne die Tastatur wäre der Dialog für alle unbrauchbar, die
  // nicht zeigen können.
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
        // Beim Feld zählt sein Inhalt, und leer heißt: nichts zu tun.
        if (offen.mitFeld) {
          const t = wertRef.current.trim();
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
        // Ein Klick daneben bricht ab, wie das Kreuz, nur schneller. Bei einer
        // Rückfrage ist das gefahrlos: Abbrechen ist die harmlose Antwort.
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
                disabled={offen.mitFeld && wert.trim() === ""}
                onClick={() => schliessen(true, wert.trim())}
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
