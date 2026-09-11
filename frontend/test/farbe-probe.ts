// Checks the colour arithmetic: contrast, the text colour on a surface, and that
// every accent becomes readable on every base tone. Run like the other probes:
//
//   npx esbuild test/farbe-probe.ts --bundle --format=esm --platform=node --outfile=/tmp/farbe.mjs
//   node /tmp/farbe.mjs
import { kontrast, ausHex, lesbarAuf, schriftAuf } from "../src/farbe";

const fehler: string[] = [];

// Known values: white on black is the maximum.
if (Math.round(kontrast(ausHex("#ffffff"), ausHex("#000000"))) !== 21) {
  fehler.push("contrast white/black is not 21");
}

// On a light accent the text has to turn dark.
if (schriftAuf("#ffd400") !== "#1a1a1a") fehler.push("yellow got white text");
if (schriftAuf("#1a3a6b") !== "#ffffff") fehler.push("dark blue got dark text");

// Every accent has to become readable on every base tone.
for (const grund of ["#ffffff", "#f7f7f6", "#1f1f1e"]) {
  for (const akzent of ["#2383e2", "#2ea043", "#8250df", "#bf5b04", "#cf222e", "#57606a"]) {
    const l = lesbarAuf(akzent, grund);
    if (kontrast(ausHex(l), ausHex(grund)) < 4.4) {
      fehler.push(`${akzent} on ${grund} stays unreadable (${l})`);
    }
  }
}

if (fehler.length) {
  console.error("colour probe failed:\n  " + fehler.join("\n  "));
  process.exit(1);
}
console.log("colour probe: all checks passed");
