// Applies the workspace-wide look.
//
// Base tone and accent belong to the account: they sit in its row, not in the
// settings table, and everybody chooses them for themselves under My account.
// The page width comes from the same response but remains a matter for the
// instance.
//
// They are written onto the root element as an attribute and a CSS variable, so
// the change reaches every component at once. No component knows about themes.
import { ReactNode, createContext, useCallback, useContext, useEffect, useState } from "react";
import { setzeSprache, sprache } from "./sprache";

import { api } from "./api/client";
import { useAuth } from "./auth";
import { ausHex, kontrast, lesbarAuf, schriftAuf } from "./farbe";

export interface Design {
  grundton: string;
  akzent: string;
  /** The width a page stands in that says nothing itself. */
  seitenbreite: string;
}

const Ctx = createContext<{ design: Design; neuLaden: () => void }>({
  design: { grundton: "grau", akzent: "#2383e2", seitenbreite: "voll" },
  neuLaden: () => {},
});

// The ground the accent lands on, per base tone. Has to match the values in
// styles.css; repeating them here is not pretty, but the alternative would be to
// read them from the stylesheet at runtime, and that costs more than it brings.
export const GRUND: Record<string, string> = {
  weiss: "#ffffff",
  grau: "#f7f7f6",
  dunkel: "#1f1f1e",
};

// The choice offered to an account. It used to sit in EinstellungenView,
// because that was where the choosing happened; since the appearance belongs to
// the account and not to the instance, the vocabulary belongs where anwenden
// sits too.
export const GRUNDTOENE: { wert: string; titel: string }[] = [
  { wert: "grau", titel: "Off-white" },
  { wert: "weiss", titel: "Pure white" },
  { wert: "dunkel", titel: "Dark" },
];

// The four load-bearing marks per base tone, in the order --bg, --flaeche,
// --border, --text. They stand like this in styles.css as well; the repetition
// is the price for a tile being able to show the tone without reading the
// stylesheet at runtime. If a tone changes there, it has to change here too.
export const TON_MARKEN: Record<string, string[]> = {
  grau: ["#f7f7f6", "#ffffff", "#e2e2df", "#37352f"],
  weiss: ["#ffffff", "#ffffff", "#ededec", "#37352f"],
  dunkel: ["#1f1f1e", "#2a2a28", "#3a3a37", "#e6e5e2"],
};

export const AKZENTE = [
  { wert: "#2383e2", titel: "Blue" },
  { wert: "#2ea043", titel: "Green" },
  { wert: "#8250df", titel: "Violet" },
  { wert: "#bf5b04", titel: "Amber" },
  { wert: "#cf222e", titel: "Red" },
  { wert: "#57606a", titel: "Graphite" },
];

/**
 * The accent as text on the ground. The interface recomputes it lighter or
 * darker when it would otherwise be unreadable as a link in running text; all
 * that stands here is whether that happens. Nobody entering a house colour
 * cares about the number behind it.
 */
export function verschobenAuf(farbe: string, grund: string): string {
  if (!/^#[0-9a-f]{6}$/.test(farbe)) return "";
  const alsText = lesbarAuf(farbe, grund);
  return alsText.toLowerCase() === farbe.toLowerCase() ? "" : alsText;
}

/**
 * Whether type on this surface can still be read. Three is the threshold below
 * which even large type fails; above it the colour carries a label.
 */
export function flaecheLesbar(farbe: string): boolean {
  if (!/^#[0-9a-f]{6}$/.test(farbe)) return true;
  return kontrast(ausHex(schriftAuf(farbe)), ausHex(farbe)) >= 3;
}

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
// The width is not part of the appearance in the narrower sense and is not
// applied here either -- it merely comes along in the same request because it
// has the same origin and everybody needs it. That is why anwenden only takes
// what it really sets.
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
        // The language lives on the account: on every device it follows the
        // person. An account without a choice takes over the one this browser
        // already shows, so it is bound to the account from now on.
        if (d.sprache === "de" || d.sprache === "en") setzeSprache(d.sprache);
        else api.spracheSpeichern(sprache()).catch(() => {});
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
