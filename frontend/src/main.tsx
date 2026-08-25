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
import "./styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter>
      <AuthProvider>
        <LizenzProvider>
          <DesignProvider>
            {/* Ganz außen, damit jede Ansicht eine Rückfrage stellen kann,
                und damit der Dialog über allem liegt, was sie zeichnet. */}
            <RueckfrageProvider>
              <App />
            </RueckfrageProvider>
          </DesignProvider>
        </LizenzProvider>
      </AuthProvider>
    </BrowserRouter>
  </React.StrictMode>,
);
