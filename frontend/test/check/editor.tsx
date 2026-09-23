// Der Editor allein, ohne Anmeldung und ohne Dienst.
//
// Die Typpruefung sagt nichts darueber, ob eine Eingaberegel noch greift: ob
// aus "**fett**" beim Tippen fetter Text wird, entscheidet ProseMirror zur
// Laufzeit im Browser. Diese Seite haengt den Editor allein in eine Seite,
// damit genau das von aussen nachgesehen werden kann.
//
// Sie gehoert nicht ins Erzeugnis; sie wird nur von Hand aufgerufen.
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import Editor from "../../src/components/Editor";

function Probe() {
  return (
    <div style={{ padding: 24, maxWidth: 760, margin: "0 auto" }}>
      <h1 style={{ font: "600 18px system-ui" }}>Editor-Probe</h1>
      <p style={{ font: "14px system-ui", color: "#666" }}>
        Tippe <code>**fett**</code>, <code>*schräg*</code> und <code>***beides***</code>.
        Der Text darunter zeigt, welche Auszeichnungen der Editor daraus gemacht hat.
      </p>
      <div id="editor" style={{ border: "1px solid #ddd", borderRadius: 6, minHeight: 200 }}>
        <Editor
          initialContent={[{ type: "paragraph", content: [] }]}
          onChange={(bloecke) => {
            const feld = document.getElementById("ergebnis");
            if (feld) feld.textContent = JSON.stringify(bloecke);
          }}
        />
      </div>
      <pre
        id="ergebnis"
        style={{ font: "12px ui-monospace", whiteSpace: "pre-wrap", wordBreak: "break-all" }}
      />
    </div>
  );
}

createRoot(document.getElementById("wurzel")!).render(
  <StrictMode>
    <Probe />
  </StrictMode>,
);
