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
