// Keeps the unlocked feature set in one place so components can ask instead of
// each fetching it. Deliberately separate from auth.tsx: the license does not
// depend on who is signed in, only on the installation.
import { createContext, useContext, useEffect, useState, ReactNode } from "react";

import { api, Lizenz } from "./api/client";

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
  const [lizenz, setLizenz] = useState<Lizenz | null>(null);
  const [geladen, setGeladen] = useState(false);

  useEffect(() => {
    api
      .lizenz()
      .then(setLizenz)
      // A failing call must not break the app. Everything paid stays hidden,
      // which is the same result as an installation without a key.
      .catch(() => setLizenz(null))
      .finally(() => setGeladen(true));
  }, []);

  const frei = (extra: Extra) =>
    !!lizenz?.gueltig && lizenz.freigeschaltet.includes(extra);

  return (
    <LizenzCtx.Provider value={{ lizenz, geladen, frei }}>{children}</LizenzCtx.Provider>
  );
}

export const useLizenz = () => useContext(LizenzCtx);
