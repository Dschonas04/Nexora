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
import { lesbarAuf, schriftAuf } from "./farbe";

export interface Design {
  grundton: string;
  akzent: string;
}

const Ctx = createContext<{ design: Design; neuLaden: () => void }>({
  design: { grundton: "grau", akzent: "#2383e2" },
  neuLaden: () => {},
});

// Der Grund, auf dem der Akzent landet, je Grundton. Muss zu den Werten in
// styles.css passen, sie hier zu wiederholen ist unschön, aber die Alternative
// wäre, sie zur Laufzeit aus dem Stylesheet zu lesen, und das kostet mehr als
// es einbringt.
const GRUND: Record<string, string> = {
  weiss: "#ffffff",
  grau: "#f7f7f6",
  dunkel: "#1f1f1e",
};

// anwenden writes the values where CSS can see them. Exported so the settings
// page can preview a choice before it is saved — waiting for a round trip to
// see a colour makes picking one a chore.
//
// Neben dem Akzent selbst werden zwei abgeleitete Werte gesetzt, und die sind
// der eigentliche Punkt: --accent-text ist die Schrift, die AUF der Akzentfläche
// lesbar ist, --accent-lesbar der Akzent so weit abgerückt, dass er als Schrift
// AUF dem Grund lesbar bleibt.
//
// Ohne das stand weiße Schrift auf einem hellen Akzent und ein dunkler Akzent
// als Verweis auf dunklem Grund. Beides war unlesbar.
export function anwenden(d: Design) {
  const wurzel = document.documentElement;
  wurzel.setAttribute("data-grundton", d.grundton);

  const grund = GRUND[d.grundton] ?? GRUND.grau;
  wurzel.style.setProperty("--accent", d.akzent);
  wurzel.style.setProperty("--accent-text", schriftAuf(d.akzent));
  wurzel.style.setProperty("--accent-lesbar", lesbarAuf(d.akzent, grund));
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
