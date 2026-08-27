// Entry point. The provider order matters: AuthProvider calls the API through
// the router's context, so it has to sit inside BrowserRouter. LizenzProvider
// sits inside it because it reads an endpoint that requires a session.
//
// StrictMode double-invokes effects in development, which is why every effect in
// this app has to tolerate running twice.
import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { AuthProvider } from "./auth";
import { LizenzProvider } from "./lizenz";
import { DesignProvider } from "./design";
import { RueckfrageProvider } from "./components/Rueckfrage";
import App from "./App";
import Fehlergrenze from "./components/Fehlergrenze";
import "./styles.css";

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
