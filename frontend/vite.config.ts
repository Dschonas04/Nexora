// Dev server configuration. In production the SPA is served by nginx, which
// also proxies /api, so this file only shapes `npm run dev`.
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    // Proxy the API through the dev server so the browser sees one origin.
    // Without it the session cookie would be cross-site and never be sent.
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
