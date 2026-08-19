// Applies the workspace-wide look.
//
// The values live in the settings table, not in the browser: an administrator
// sets them once and every account sees the same thing. That is the whole point
// of putting them on the settings page rather than into a personal preference.
//
// They are written onto the root element as an attribute and a CSS variable, so
// the change reaches every component at once. No component knows about themes.
import { ReactNode, createContext, useCallback, useContext, useEffect, useState } from "react";

import { api } from "./api/client";

export interface Design {
  grundton: string;
  akzent: string;
}

const Ctx = createContext<{ design: Design; neuLaden: () => void }>({
  design: { grundton: "grau", akzent: "#2383e2" },
  neuLaden: () => {},
});

// anwenden writes the values where CSS can see them. Exported so the settings
// page can preview a choice before it is saved — waiting for a round trip to
// see a colour makes picking one a chore.
export function anwenden(d: Design) {
  const wurzel = document.documentElement;
  wurzel.setAttribute("data-grundton", d.grundton);
  wurzel.style.setProperty("--accent", d.akzent);
}

export function DesignProvider({ children }: { children: ReactNode }) {
  const [design, setDesign] = useState<Design>({ grundton: "grau", akzent: "#2383e2" });

  const neuLaden = useCallback(() => {
    api
      .design()
      .then((d) => {
        setDesign(d);
        anwenden(d);
      })
      // Schlägt der Abruf fehl, bleibt die Vorgabe aus dem Stylesheet stehen.
      // Eine Oberfläche ohne Farben wäre schlimmer als eine mit den falschen.
      .catch(() => {});
  }, []);

  useEffect(neuLaden, [neuLaden]);

  return <Ctx.Provider value={{ design, neuLaden }}>{children}</Ctx.Provider>;
}

export const useDesign = () => useContext(Ctx);
