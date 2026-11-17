// The first screen of the Android app: with which server, or without one.
// It runs in the app's own origin (https://localhost); the server address is
// kept by the app (MainActivity), which lets only that server into the window.
import { FormEvent, useEffect, useState } from "react";

import { lokalWaehlen } from "../lokal";

declare global {
  interface Window {
    NexoraApp?: { server: () => string; setzen: (adresse: string) => boolean };
  }
}

const MODUS = "nexora-modus";

export function appEinrichtungNoetig(): boolean {
  return typeof window !== "undefined" && !!window.NexoraApp && location.hostname === "localhost";
}

function oeffnen(adresse: string) {
  location.replace(adresse.replace(/\/+$/, "") + "/");
}

export default function AppEinrichtung() {
  const gespeichert = window.NexoraApp?.server() || "";
  const [modus] = useState(() => {
    try {
      return localStorage.getItem(MODUS) || "";
    } catch {
      return "";
    }
  });
  const [waehlen, setWaehlen] = useState(!(modus === "server" && gespeichert));
  const [adresse, setAdresse] = useState(gespeichert);
  const [meldung, setMeldung] = useState("");
  const [fehler, setFehler] = useState(false);
  const [laeuft, setLaeuft] = useState(false);

  // The design comes from the account, and there is none yet: follow the
  // system's light or dark.
  useEffect(() => {
    if (window.matchMedia?.("(prefers-color-scheme: dark)").matches) {
      document.documentElement.dataset.grundton = "dunkel";
    }
  }, []);

  useEffect(() => {
    if (waehlen) return;
    const t = window.setTimeout(() => oeffnen(gespeichert), 1200);
    return () => window.clearTimeout(t);
  }, [waehlen, gespeichert]);

  const verbinden = async (e: FormEvent) => {
    e.preventDefault();
    let a = adresse.trim().replace(/\/+$/, "");
    if (!/^https?:\/\//.test(a)) a = "https://" + a;
    setAdresse(a);
    setLaeuft(true);
    setFehler(false);
    setMeldung("Checking…");
    try {
      // no-cors hides the answer, but a failure means the server cannot be
      // reached or its certificate is not trusted.
      await fetch(a + "/healthz", { mode: "no-cors", signal: AbortSignal.timeout(8000) });
    } catch {
      setFehler(true);
      setMeldung("The server cannot be reached. Is the address right, and are you in the same network?");
      setLaeuft(false);
      return;
    }
    if (!window.NexoraApp?.setzen(a)) {
      setFehler(true);
      setMeldung("The address could not be saved.");
      setLaeuft(false);
      return;
    }
    localStorage.setItem(MODUS, "server");
    setMeldung("Connected.");
    oeffnen(new URL(a).origin);
  };

  const ohneServer = () => {
    lokalWaehlen();
    location.replace("/");
  };

  return (
    <div className="app-einrichtung">
      <main>
        <img src="/favicon.svg" width={48} height={48} alt="" className="app-einrichtung-zeichen" />
        {!waehlen ? (
          <>
            <h1>Nexora</h1>
            <p>{`Connecting to ${gespeichert}…`}</p>
            <div className="app-einrichtung-knoepfe">
              <button className="btn btn-primary" onClick={() => oeffnen(gespeichert)}>
                Open
              </button>
              <button className="btn" onClick={() => setWaehlen(true)}>
                Change server
              </button>
            </div>
          </>
        ) : (
          <>
            <h1>Set up Nexora</h1>
            <p>
              Use Nexora with your server: on the go you work on a copy of your pages, at home everything is synced.
              Or use it on this device only, without a server and without an account.
            </p>
            <form onSubmit={verbinden} noValidate>
              <label htmlFor="adresse">Server address</label>
              <input
                id="adresse"
                className="input"
                inputMode="url"
                autoCapitalize="off"
                spellCheck={false}
                placeholder="https://10.0.2.7:3000"
                value={adresse}
                onChange={(e) => setAdresse(e.target.value)}
              />
              <div className="app-einrichtung-knoepfe">
                <button className="btn btn-primary" type="submit" disabled={laeuft || !adresse.trim()}>
                  Connect
                </button>
              </div>
              <div className={"app-einrichtung-meldung" + (fehler ? " fehler" : "")} role="status">
                {meldung}
              </div>
            </form>
            <div className="app-einrichtung-trenner">
              <span>or</span>
            </div>
            <button className="btn app-einrichtung-lokal" onClick={ohneServer}>
              Use without a server
            </button>
            <p className="muted small">The pages are kept only on this device, for the user signed in to it.</p>
          </>
        )}
      </main>
    </div>
  );
}
