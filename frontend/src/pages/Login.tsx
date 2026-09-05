// Sign-in form. On success AuthProvider sets the user, and App swaps this
// screen for the workspace; there is no explicit redirect here.
import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { api } from "../api/client";
import { useAuth } from "../auth";

export default function Login() {
  const { login } = useAuth();
  // Eine Zeile für beides. Was drin steht, entscheidet der Server am @: eine
  // Auswahl davor wäre eine Frage, die niemand beantworten müsste.
  const [kennung, setKennung] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  // What this instance offers. Asked of the server rather than guessed: an SSO
  // button with nothing set up behind it leads into an error message instead of
  // a sign in.
  const [wege, setWege] = useState<{ oidc: boolean; oidcText: string; ldap: boolean; anbieter: string } | null>(null);
  const [ueberVerzeichnis, setUeberVerzeichnis] = useState(false);
  const [suche] = useSearchParams();

  useEffect(() => {
    api
      .ssoZustand()
      .then((z) => setWege(z))
      .catch(() => setWege(null));
    // If the browser comes back from an aborted SSO sign in, the reason stands
    // in the address.
    const meldung = suche.get("sso");
    if (meldung) setError(meldung);
  }, [suche]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    // busy disables the button so a double click cannot fire two logins.
    setBusy(true);
    setError("");
    try {
      if (ueberVerzeichnis) {
        await api.ldapAnmelden(kennung, password);
        // The session sits in the cookie; reloading lets AuthProvider read it
        // and switches over to the workspace.
        window.location.href = "/";
        return;
      }
      await login(kennung, password);
    } catch (err) {
      setError((err as Error).message || "Anmeldung fehlgeschlagen");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="auth">
      <form className="auth-card" onSubmit={submit}>
        <h1>Nexora</h1>
        <p className="sub">Melde dich in deinem Workspace an</p>
        {error && <div className="error">{error}</div>}
        <div className="field">
          <label>{ueberVerzeichnis ? "Benutzer" : "E-Mail oder Benutzername"}</label>
          {/* type="text" auch ohne Verzeichnis: bei type="email" hält der
              Browser jede Eingabe ohne @ für einen Tippfehler und lässt das
              Formular gar nicht erst abschicken. */}
          <input
            type="text"
            autoComplete="username"
            value={kennung}
            onChange={(e) => setKennung(e.target.value)}
            autoFocus
          />
        </div>
        <div className="field">
          <label>Passwort</label>
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
        <button className="btn-primary" type="submit" disabled={busy}>
          {busy ? "Anmelden…" : "Anmelden"}
        </button>
        {wege?.ldap && (
          <div className="switch">
            <button
              type="button"
              className="link-btn"
              onClick={() => setUeberVerzeichnis((v) => !v)}
            >
              {ueberVerzeichnis
                ? "Stattdessen mit Konto und Passwort anmelden"
                : "Stattdessen über das Verzeichnis anmelden"}
            </button>
          </div>
        )}

        {wege?.oidc && (
          <>
            <div className="anmelde-trenner">
              <span>oder</span>
            </div>
            {/* An ordinary link, not a fetch: the provider answers with a
                redirect to its own page, and the browser has to go there
                itself. */}
            <a className="btn-primary anmelde-sso" href="/api/auth/oidc/start">
              {wege.oidcText || `Mit ${wege.anbieter || "SSO"} anmelden`}
            </a>
          </>
        )}

        <div className="switch">
          Kein Konto? <Link to="/register">Registrieren</Link>
        </div>
      </form>
    </div>
  );
}
