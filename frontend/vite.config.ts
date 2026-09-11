// Dev server configuration. In production the SPA is served by nginx, which
// also proxies /api, so this file only shapes `npm run dev`.
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The header every generated bundle carries.
//
// It stands here and not in a file beside it, because MPL-2.0 demands that the
// notice accompany the DISTRIBUTED form, and what is distributed is the bundle.
// The minifier throws comments away, BlockNote's licence headers included; so
// without these lines MPL source would go out without any notice at all.
//
// Kept short on purpose: the complete list stands in THIRD-PARTY.md, and a
// header longer than necessary would be forgotten at the next change.
const kopf = [
  "/*!",
  " * Nexora. Business Source License 1.1, from 19.08.2030 Apache 2.0.",
  " *",
  " * This bundle contains third-party components, among them BlockNote",
  " * (@blocknote/core, /react, /mantine) under the Mozilla Public License 2.0.",
  " * Source: https://github.com/TypeCellOS/BlockNote",
  " *",
  " * Complete list of all components and their licences:",
  " * THIRD-PARTY.md in the source tree.",
  " */",
].join("\n");

// Haengt den Kopf an jedes erzeugte JavaScript-Buendel. Als eigener Schritt,
// weil die Auflage aus MPL-2.0 lautlos bricht: ohne Hinweis geht fremder
// Quelltext hinaus, und niemand sieht es dem Erzeugnis an.
const lizenzkopf = {
  name: "nexora-lizenzkopf",
  generateBundle(_einstellungen: unknown, buendel: Record<string, { type: string; code?: string }>) {
    for (const stueck of Object.values(buendel)) {
      if (stueck.type === "chunk" && typeof stueck.code === "string") {
        stueck.code = kopf + "\n" + stueck.code;
      }
    }
  },
};

export default defineConfig({
  plugins: [react(), lizenzkopf],
  build: {
    // Der Kopf wird von Hand vorangestellt und nicht ueber eine Einstellung
    // des Buendlers. Grund: der Name dieser Einstellung hat sich mit Vite 8
    // geaendert (rollupOptions wurde rolldownOptions), der alte wurde
    // stillschweigend uebergangen, und der Lizenzkopf fiel damit aus jedem
    // Buendel -- ohne Fehler, ohne Warnung. Ein eigener Schritt haengt an
    // keinem Namen, den die naechste Fassung umbenennen kann.
    //
    // React, Yjs and the two PDF libraries each get a chunk of their own, so
    // they stay in the browser cache across Nexora updates and the start chunk
    // carries only Nexora's own code (227 kB instead of 536 kB).
    // BlockNote, Mantine and ProseMirror deliberately do not: grouped, Rolldown
    // tied the whole group to the start chunk, and the editor's megabyte was
    // loaded on every first page view instead of with the editor.
    // The build output has to be checked after every change of this block --
    // Vite 8 silently ignores options it does not know.
    //
    // The only chunk above 500 kB is the editor, and that one is loaded lazily
    // when a page opens; the limit is raised so the warning keeps meaning
    // something for the start chunk.
    chunkSizeWarningLimit: 1000,
    rolldownOptions: {
      output: {
        advancedChunks: {
          groups: [
            { name: "react", test: /node_modules[\\/](react|react-dom|react-router|scheduler)[\\/]/ },
            { name: "yjs", test: /node_modules[\\/](yjs|y-protocols|lib0)[\\/]/ },
            { name: "pdfjs", test: /node_modules[\\/]pdfjs-dist[\\/]/ },
            { name: "pdf-lib", test: /node_modules[\\/](pdf-lib|@pdf-lib|pako)[\\/]/ },
          ],
        },
      },
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
