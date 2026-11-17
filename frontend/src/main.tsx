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
import { offlineStarten } from "./offline";
import { lokalModus } from "./lokal";
import AppEinrichtung, { appEinrichtungNoetig } from "./components/AppEinrichtung";

// The service worker makes Nexora installable as an app and keeps the pages
// one has opened readable without a connection; see public/sw.js. Only in
// the built application: in development it would serve yesterday's bundle.
// Not inside the apps' own origin (app:// on the desktop, https://localhost
// on Android): there the interface comes with the app, and without a server
// there is nothing to cache.
const imAppUrsprung = location.protocol === "app:" || location.hostname === "localhost";
if (import.meta.env.PROD && "serviceWorker" in navigator && !imAppUrsprung) {
  window.addEventListener("load", () => {
    navigator.serviceWorker
      .register("/sw.js")
      .then(() => offlineStarten())
      .catch(() => {
        /* without it everything works as before, only not offline */
      });
  });
}

const wurzel = ReactDOM.createRoot(document.getElementById("root")!);
if (appEinrichtungNoetig() && !lokalModus()) {
  // The Android app before a choice: server or this device only.
  wurzel.render(<AppEinrichtung />);
} else wurzel.render(
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
