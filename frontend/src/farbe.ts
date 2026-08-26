// Colour arithmetic for the accent.
//
// The accent is chosen freely, the surfaces it lands on are not. Without a
// calculation somewhere, a light accent gets white text on it and a dark one
// becomes an unreadable link on a dark background. Both happened.
//
// The numbers follow WCAG: relative luminance, then a contrast ratio, then
// adjust until the ratio is good enough. 4.5 is the threshold for body text.

export interface RGB {
  r: number;
  g: number;
  b: number;
}

export function ausHex(hex: string): RGB {
  const h = hex.replace("#", "");
  return {
    r: parseInt(h.slice(0, 2), 16),
    g: parseInt(h.slice(2, 4), 16),
    b: parseInt(h.slice(4, 6), 16),
  };
}

export function zuHex({ r, g, b }: RGB): string {
  const t = (n: number) => Math.max(0, Math.min(255, Math.round(n))).toString(16).padStart(2, "0");
  return `#${t(r)}${t(g)}${t(b)}`;
}

/** Relative luminance per WCAG. 0 is black, 1 is white. */
export function leuchtkraft(c: RGB): number {
  const kanal = (v: number) => {
    const s = v / 255;
    // The curve is not linear: the eye perceives dark gradations more finely
    // than light ones. A simple mean of the channels would be clearly off.
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * kanal(c.r) + 0.7152 * kanal(c.g) + 0.0722 * kanal(c.b);
}

/** Contrast ratio of two colours, 1 to 21. */
export function kontrast(a: RGB, b: RGB): number {
  const la = leuchtkraft(a);
  const lb = leuchtkraft(b);
  const hell = Math.max(la, lb);
  const dunkel = Math.min(la, lb);
  return (hell + 0.05) / (dunkel + 0.05);
}

/**
 * Which text is readable on this surface: white or a very dark grey. Pure black
 * would be harsher than necessary and fits none of the base tones.
 *
 * White is preferred as long as dark does not score clearly better. Purely by
 * the number a medium blue would get dark text, which is arithmetically right,
 * but a blue button with almost black lettering looks like a mistake. The factor
 * only tips with really light colours such as yellow, and that is exactly where
 * it should tip.
 */
export function schriftAuf(flaeche: string): string {
  const f = ausHex(flaeche);
  const weiss = { r: 255, g: 255, b: 255 };
  const dunkel = { r: 26, g: 26, b: 26 };
  return kontrast(f, dunkel) > kontrast(f, weiss) * 1.5 ? "#1a1a1a" : "#ffffff";
}

/**
 * Moves a colour away from the surface until it is readable on it.
 *
 * Lightened on a dark ground, darkened on a light one, in small steps so that
 * the chosen colour stays recognisable. After 24 steps it gives up: on a surface
 * of medium brightness there are colours that never reach the goal, and an
 * endless loop would be the worse answer.
 */
export function lesbarAuf(farbe: string, flaeche: string, ziel = 4.5): string {
  const grund = ausHex(flaeche);
  let c = ausHex(farbe);
  if (kontrast(c, grund) >= ziel) return farbe;

  const aufhellen = leuchtkraft(grund) < 0.5;
  for (let i = 0; i < 24; i++) {
    c = aufhellen
      ? { r: c.r + (255 - c.r) * 0.12, g: c.g + (255 - c.g) * 0.12, b: c.b + (255 - c.b) * 0.12 }
      : { r: c.r * 0.88, g: c.g * 0.88, b: c.b * 0.88 };
    if (kontrast(c, grund) >= ziel) break;
  }
  return zuHex(c);
}
