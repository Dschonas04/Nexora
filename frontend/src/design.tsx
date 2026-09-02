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
import { useAuth } from "./auth";
import { lesbarAuf, schriftAuf } from "./farbe";

export interface Design {
  grundton: string;
  akzent: string;
  /** Die Breite, in der eine Seite steht, die selbst nichts sagt. */
  seitenbreite: string;
}

const Ctx = createContext<{ design: Design; neuLaden: () => void }>({
  design: { grundton: "grau", akzent: "#2383e2", seitenbreite: "voll" },
  neuLaden: () => {},
});

// The ground the accent lands on, per base tone. Has to match the values in
// styles.css; repeating them here is not pretty, but the alternative would be to
// read them from the stylesheet at runtime, and that costs more than it brings.
const GRUND: Record<string, string> = {
  weiss: "#ffffff",
  grau: "#f7f7f6",
  dunkel: "#1f1f1e",
};

// anwenden writes the values where CSS can see them. Exported so the settings
// page can preview a choice before it is saved, since waiting for a round trip
// to see a colour makes picking one a chore.
//
// Besides the accent itself two derived values are set, and those are the actual
// point: --accent-text is the text readable ON the accent surface,
// --accent-lesbar the accent moved far enough away to stay readable as text ON
// the ground.
//
// Without that, white text stood on a light accent and a dark accent stood as a
// link on a dark ground. Both were unreadable.
// Die Breite ist nicht Teil des Aussehens im engeren Sinn und wird hier auch
// nicht angewandt -- sie steht nur mit im selben Abruf, weil sie dieselbe
// Herkunft hat und jeder sie braucht. Deshalb nimmt anwenden nur, was es
// wirklich setzt.
export function anwenden(d: Pick<Design, "grundton" | "akzent">) {
  const wurzel = document.documentElement;
  wurzel.setAttribute("data-grundton", d.grundton);

  const grund = GRUND[d.grundton] ?? GRUND.grau;
  wurzel.style.setProperty("--accent", d.akzent);
  wurzel.style.setProperty("--accent-text", schriftAuf(d.akzent));
  wurzel.style.setProperty("--accent-lesbar", lesbarAuf(d.akzent, grund));
}

export function DesignProvider({ children }: { children: ReactNode }) {
  const [design, setDesign] = useState<Design>({ grundton: "grau", akzent: "#2383e2", seitenbreite: "voll" });
  // Whose look is being asked for. /api/design needs a session, so before the
  // sign-in the call answers 401 -- and it has to be repeated afterwards.
  //
  // Without that the interface stayed in the default look until somebody
  // reloaded the page by hand: one signed in and the workspace was light, even
  // though it had been set to dark. Half the picture then belonged to one look
  // and half to the other.
  const { user } = useAuth();

  const neuLaden = useCallback(() => {
    api
      .design()
      .then((d) => {
        setDesign(d);
        anwenden(d);
      })
      // If the request fails, the default from the stylesheet stays. An
      // interface without colours would be worse than one with the wrong ones.
      .catch(() => {});
  }, []);

  // Runs on the first draw and again on every change of account -- signing in,
  // signing out, switching users.
  useEffect(neuLaden, [neuLaden, user?.id]);

  return <Ctx.Provider value={{ design, neuLaden }}>{children}</Ctx.Provider>;
}

export const useDesign = () => useContext(Ctx);
