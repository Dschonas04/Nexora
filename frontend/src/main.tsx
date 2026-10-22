// Entry point. The provider order matters: AuthProvider calls the API through
// the router's context, so it has to sit inside BrowserRouter. LizenzProvider
// sits inside it because it reads an endpoint that requires a session.
//
// StrictMode double-invokes effects in development, which is why every effect in
// this app has to tolerate running twice.
import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router";
import { AuthProvider } from "./auth";
import { LizenzProvider } from "./lizenz";
import { DesignProvider } from "./design";
import { RueckfrageProvider } from "./components/Rueckfrage";
import App from "./App";
import Fehlergrenze from "./components/Fehlergrenze";
import "./styles.css";
// Before the first render, so the very first frame already stands in the chosen language.
import "./sprache";

// The service worker makes Nexora installable as an app and keeps the pages
// one has opened readable without a connection; see public/sw.js. Only in
// the built application: in development it would serve yesterday's bundle.
if (import.meta.env.PROD && "serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js").catch(() => {
      /* without it everything works as before, only not offline */
    });
  });
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter>
      <AuthProvider>
        <LizenzProvider>
          <DesignProvider>
            {/* Right on the outside, so that every view can ask a question, and
                so that the dialog lies above everything it draws. */}
            <RueckfrageProvider>
              {/* The last net. Whatever a view throws while drawing, a message
                  stands here instead of an empty window. */}
              <Fehlergrenze text="Die Ansicht liess sich nicht aufbauen.">
                <App />
              </Fehlergrenze>
            </RueckfrageProvider>
          </DesignProvider>
        </LizenzProvider>
      </AuthProvider>
    </BrowserRouter>
  </React.StrictMode>,
);
