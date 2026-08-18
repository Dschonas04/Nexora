// Entry point. The provider order matters: AuthProvider calls the API through
// the router's context, so it has to sit inside BrowserRouter.
//
// StrictMode double-invokes effects in development, which is why every effect in
// this app has to tolerate running twice.
import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { AuthProvider } from "./auth";
import App from "./App";
import "./styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter>
      <AuthProvider>
        <App />
      </AuthProvider>
    </BrowserRouter>
  </React.StrictMode>,
);
