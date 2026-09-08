// Applies the workspace-wide look.
//
// Grundton und Akzent gehoeren dem Konto: sie stehen in dessen Zeile, nicht in
// der Einstellungstabelle, und jeder waehlt sie fuer sich unter Mein Konto. Die
// Seitenbreite kommt aus derselben Antwort, bleibt aber eine Sache der Instanz.
//
// They are written onto the root element as an attribute and a CSS variable, so
// the change reaches every component at once. No component knows about themes.
import { ReactNode, createContext, useCallback, useContext, useEffect, useState } from "react";

import { api } from "./api/client";
import { useAuth } from "./auth";
import { ausHex, kontrast, lesbarAuf, schriftAuf } from "./farbe";

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
export const GRUND: Record<string, string> = {
  weiss: "#ffffff",
  grau: "#f7f7f6",
  dunkel: "#1f1f1e",
};

// Die Auswahl, die einem Konto angeboten wird. Sie stand bisher in
// EinstellungenView, weil dort gewaehlt wurde; seit das Aussehen dem Konto
// gehoert und nicht der Instanz, gehoert das Vokabular dorthin, wo auch
// anwenden steht.
export const GRUNDTOENE: { wert: string; titel: string }[] = [
  { wert: "grau", titel: "Gegrautes Weiß" },
  { wert: "weiss", titel: "Reines Weiß" },
  { wert: "dunkel", titel: "Dunkel" },
];

// Die vier tragenden Marken je Grundton, in der Reihenfolge --bg, --flaeche,
// --border, --text. Sie stehen so auch in styles.css; die Wiederholung ist der
// Preis dafuer, dass eine Kachel den Ton zeigen kann, ohne das Stylesheet zur
// Laufzeit auszulesen. Aendert sich dort ein Ton, muss er hier mit.
export const TON_MARKEN: Record<string, string[]> = {
  grau: ["#f7f7f6", "#ffffff", "#e2e2df", "#37352f"],
  weiss: ["#ffffff", "#ffffff", "#ededec", "#37352f"],
  dunkel: ["#1f1f1e", "#2a2a28", "#3a3a37", "#e6e5e2"],
};

export const AKZENTE = [
  { wert: "#2383e2", titel: "Blau" },
  { wert: "#2ea043", titel: "Grün" },
  { wert: "#8250df", titel: "Violett" },
  { wert: "#bf5b04", titel: "Bernstein" },
  { wert: "#cf222e", titel: "Rot" },
  { wert: "#57606a", titel: "Graphit" },
];

/**
 * Der Akzent als Text auf dem Grund. Die Oberfläche rechnet ihn hell oder
 * dunkel nach, wenn er als Verknüpfung im Fließtext sonst nicht zu lesen wäre;
 * hier steht nur, ob das passiert. Die Zahl dahinter interessiert niemanden,
 * der eine Hausfarbe einträgt.
 */
export function verschobenAuf(farbe: string, grund: string): string {
  if (!/^#[0-9a-f]{6}$/.test(farbe)) return "";
  const alsText = lesbarAuf(farbe, grund);
  return alsText.toLowerCase() === farbe.toLowerCase() ? "" : alsText;
}

/**
 * Ob Schrift auf dieser Fläche noch zu lesen ist. Drei ist die Schwelle, unter
 * der auch große Schrift durchfällt; darüber trägt die Farbe eine Beschriftung.
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
