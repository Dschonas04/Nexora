// No test run is set up in the build; this file serves as an executable
// description of what the computation has to achieve, and as a template once a
// test run for the frontend is added.
import { kontrast, ausHex, lesbarAuf, schriftAuf } from "./farbe";

export function pruefe(): string[] {
  const fehler: string[] = [];

  // Known values: white on black is the maximum.
  if (Math.round(kontrast(ausHex("#ffffff"), ausHex("#000000"))) !== 21) {
    fehler.push("Kontrast Weiß/Schwarz ist nicht 21");
  }

  // Auf einem hellen Akzent muss die Schrift dunkel werden.
  if (schriftAuf("#ffd400") !== "#1a1a1a") fehler.push("Gelb bekam weiße Schrift");
  if (schriftAuf("#1a3a6b") !== "#ffffff") fehler.push("Dunkelblau bekam dunkle Schrift");

  // Jeder Akzent muss auf jedem Grundton lesbar werden.
  for (const grund of ["#ffffff", "#f7f7f6", "#1f1f1e"]) {
    for (const akzent of ["#2383e2", "#2ea043", "#8250df", "#bf5b04", "#cf222e", "#57606a"]) {
      const l = lesbarAuf(akzent, grund);
      if (kontrast(ausHex(l), ausHex(grund)) < 4.4) {
        fehler.push(`${akzent} auf ${grund} bleibt unlesbar (${l})`);
      }
    }
  }
  return fehler;
}
