// Keeps the unlocked feature set in one place so components can ask instead of
// each fetching it. Deliberately separate from auth.tsx: the license does not
// depend on who is signed in, only on the installation.
import { createContext, useContext, useEffect, useState, ReactNode } from "react";

import { api, Lizenz } from "./api/client";
import { useAuth } from "./auth";

// The names must match the backend constants in internal/lizenz. Keeping them
// as a union rather than plain string is what turns a typo into a build error
// instead of a feature that silently never unlocks.
export type Extra =
  | "versionen"
  | "anhaenge"
  | "freigeben"
  | "pruefspur"
  | "gruppen"
  | "sso"
  | "ldap"
  | "anhangsuche"
  | "export"
  | "vorlagen"
  | "kommentare"
  | "konflikte";

interface Ctx {
  lizenz: Lizenz | null;
  /** True once the state is known, so the interface does not flicker. */
  geladen: boolean;
  /** Whether one paid extra is available. Unknown state counts as locked. */
  frei: (extra: Extra) => boolean;
}

const LizenzCtx = createContext<Ctx>({
  lizenz: null,
  geladen: false,
  frei: () => false,
});

export function LizenzProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const [lizenz, setLizenz] = useState<Lizenz | null>(null);
  const [geladen, setGeladen] = useState(false);

  // Neu geholt, sobald sich das angemeldete Konto ändert.
  //
  // Vorher lief der Abruf genau einmal, beim Aufbau der Anwendung -- also vor
  // der Anmeldung. Die Auskunft braucht aber eine Sitzung und antwortete mit
  // 401; danach galt für den Rest der Sitzung "nichts freigeschaltet". Wer
  // sich frisch anmeldete, sah gekaufte Funktionen als nicht enthalten, bis er
  // die Seite neu lud.
  //
  // Ein Fehlschlag wird zudem nicht mehr als Antwort genommen: das Backend ist
  // beim Ausrollen für ein paar Sekunden weg, und eine Instanz, die danach
  // dauerhaft ihre Lizenz vergisst, ist schlechter als eine, die kurz wartet.
  useEffect(() => {
    if (!user) {
      setLizenz(null);
      setGeladen(true);
      return;
    }
    let abgebrochen = false;
    let versuch = 0;
    const holen = () => {
      api
        .lizenz()
        .then((l) => {
          if (abgebrochen) return;
          setLizenz(l);
          setGeladen(true);
        })
        .catch(() => {
          if (abgebrochen) return;
          versuch += 1;
          // Drei Anläufe mit wachsendem Abstand, dann bleibt es beim
          // gesperrten Umfang -- irgendwann muss die Oberfläche etwas zeigen.
          if (versuch <= 3) {
            window.setTimeout(holen, 600 * versuch);
          } else {
            setGeladen(true);
          }
        });
    };
    holen();
    return () => {
      abgebrochen = true;
    };
  }, [user]);

  const frei = (extra: Extra) =>
    !!lizenz?.gueltig && lizenz.freigeschaltet.includes(extra);

  return (
    <LizenzCtx.Provider value={{ lizenz, geladen, frei }}>{children}</LizenzCtx.Provider>
  );
}

export const useLizenz = () => useContext(LizenzCtx);
