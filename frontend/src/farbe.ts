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

/** Relative Leuchtkraft nach WCAG. 0 ist Schwarz, 1 ist Weiß. */
export function leuchtkraft(c: RGB): number {
  const kanal = (v: number) => {
    const s = v / 255;
    // Die Kurve ist nicht linear: das Auge nimmt dunkle Abstufungen feiner
    // wahr als helle. Ein einfacher Mittelwert der Kanäle läge deutlich daneben.
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * kanal(c.r) + 0.7152 * kanal(c.g) + 0.0722 * kanal(c.b);
}

/** Kontrastverhältnis zweier Farben, 1 bis 21. */
export function kontrast(a: RGB, b: RGB): number {
  const la = leuchtkraft(a);
  const lb = leuchtkraft(b);
  const hell = Math.max(la, lb);
  const dunkel = Math.min(la, lb);
  return (hell + 0.05) / (dunkel + 0.05);
}

/**
 * Welche Schrift auf dieser Fläche lesbar ist: Weiß oder ein sehr dunkles Grau.
 * Reines Schwarz wäre härter als nötig und passt zu keinem der Grundtöne.
 *
 * Weiß wird bevorzugt, solange Dunkel nicht deutlich besser abschneidet. Rein
 * nach der Zahl bekäme ein mittleres Blau dunkle Schrift -- rechnerisch
 * richtig, aber ein blauer Knopf mit fast schwarzer Aufschrift sieht nach
 * Versehen aus. Der Faktor kippt erst bei wirklich hellen Farben wie Gelb, und
 * genau dort soll er kippen.
 */
export function schriftAuf(flaeche: string): string {
  const f = ausHex(flaeche);
  const weiss = { r: 255, g: 255, b: 255 };
  const dunkel = { r: 26, g: 26, b: 26 };
  return kontrast(f, dunkel) > kontrast(f, weiss) * 1.5 ? "#1a1a1a" : "#ffffff";
}

/**
 * Rückt eine Farbe so weit von der Fläche ab, bis sie darauf lesbar ist.
 *
 * Aufgehellt wird auf dunklem Grund, abgedunkelt auf hellem -- in kleinen
 * Schritten, damit die gewählte Farbe erkennbar bleibt. Nach 24 Schritten wird
 * abgebrochen: bei einer Fläche mittlerer Helligkeit gibt es Farben, die das
 * Ziel nie erreichen, und eine Endlosschleife wäre die schlechtere Antwort.
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
