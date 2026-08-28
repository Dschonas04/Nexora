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
  | "kommentare"
  | "konflikte";

interface Ctx {
  lizenz: Lizenz | null;
  /** True once the state is known, so the interface does not flicker. */
  geladen: boolean;
  /** Whether one paid extra is available. Unknown state counts as locked. */
  frei: (extra: Extra) => boolean;
  /** After reading in a key: fetch the state again right away. */
  neuLaden: () => void;
}

const LizenzCtx = createContext<Ctx>({
  lizenz: null,
  geladen: false,
  frei: () => false,
  neuLaden: () => {},
});

export function LizenzProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const [lizenz, setLizenz] = useState<Lizenz | null>(null);
  const [geladen, setGeladen] = useState(false);

  // Fetched anew as soon as the signed in account changes.
  //
  // Before this the request ran exactly once, while the application was being
  // built up, that is before signing in. The information needs a session though
  // and answered with 401; after that "nothing unlocked" applied for the rest of
  // the session. Whoever signed in freshly saw bought features as not included
  // until they reloaded the page.
  //
  // A failure is moreover no longer taken as an answer: the backend is gone for
  // a few seconds while rolling out, and an instance that permanently forgets
  // its licence afterwards is worse than one that waits briefly.
  // A counter instead of a flag: every call of neuLaden triggers the effect
  // again, even when the account has not changed.
  const [runde, setRunde] = useState(0);

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
          // Three attempts with growing distance, then it stays at the locked
          // scope; at some point the interface has to show something.
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
  }, [user, runde]);

  const frei = (extra: Extra) =>
    !!lizenz?.gueltig && lizenz.freigeschaltet.includes(extra);

  return (
    <LizenzCtx.Provider value={{ lizenz, geladen, frei, neuLaden: () => setRunde((r) => r + 1) }}>
      {children}
    </LizenzCtx.Provider>
  );
}

export const useLizenz = () => useContext(LizenzCtx);
