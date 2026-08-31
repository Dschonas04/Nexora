// Dev server configuration. In production the SPA is served by nginx, which
// also proxies /api, so this file only shapes `npm run dev`.
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Der Kopf, den jedes erzeugte Buendel traegt.
//
// Er steht hier und nicht in einer Datei daneben, weil MPL-2.0 verlangt, dass
// der Hinweis die VERTEILTE Form begleitet, und verteilt wird das Buendel. Der
// Minimierer wirft Kommentare weg, auch die Lizenzkoepfe von BlockNote; ohne
// diese Zeilen ginge also MPL-Quelltext ohne jeden Hinweis hinaus.
//
// Kurz gehalten mit Absicht: die vollstaendige Liste steht in THIRD-PARTY.md,
// und ein Kopf, der laenger waere als noetig, wuerde bei der naechsten
// Aenderung vergessen.
const kopf = [
  "/*!",
  " * Nexora. Business Source License 1.1, ab 19.08.2030 Apache 2.0.",
  " *",
  " * Dieses Buendel enthaelt fremde Bestandteile, darunter BlockNote",
  " * (@blocknote/core, /react, /mantine) unter der Mozilla Public License 2.0.",
  " * Quelltext: https://github.com/TypeCellOS/BlockNote",
  " *",
  " * Vollstaendige Aufstellung aller Bestandteile und ihrer Lizenzen:",
  " * THIRD-PARTY.md im Quelltextbestand.",
  " */",
].join("\n");

export default defineConfig({
  plugins: [react()],
  build: {
    rollupOptions: {
      output: { banner: kopf },
    },
  },
  server: {
    port: 5173,
    // Proxy the API through the dev server so the browser sees one origin.
    // Without it the session cookie would be cross-site and never be sent.
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
