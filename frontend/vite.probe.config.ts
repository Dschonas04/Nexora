// Nur zum Gegentesten von Hand: liefert test/probe/editorprobe.html aus.
// Gehoert nicht ins Erzeugnis und wird von keinem Bauschritt angefasst.
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  root: "test/probe",
  server: { port: 4181, host: "127.0.0.1" },
});
