// Sign-in form. On success AuthProvider sets the user, and App swaps this
// screen for the workspace; there is no explicit redirect here.
import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import { useAuth } from "../auth";

export default function Login() {
  const { login } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  // Was diese Instanz anbietet. Erst vom Server erfragt, statt es zu erraten:
  // ein SSO-Knopf, hinter dem nichts eingerichtet ist, führt in eine
  // Fehlermeldung statt zu einer Anmeldung.
  const [wege, setWege] = useState<{ oidc: boolean; oidcText: string; ldap: boolean; anbieter: string } | null>(null);
  const [ueberVerzeichnis, setUeberVerzeichnis] = useState(false);
  const [suche] = useSearchParams();

  useEffect(() => {
    api
      .ssoZustand()
      .then((z) => setWege(z))
      .catch(() => setWege(null));
    // Kommt der Browser von einer abgebrochenen SSO-Anmeldung zurück, steht
    // der Grund in der Adresse.
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
        await api.ldapAnmelden(email, password);
        // Die Sitzung steht im Plätzchen; neu laden lässt AuthProvider sie
        // lesen und schaltet auf den Arbeitsbereich um.
        window.location.href = "/";
        return;
      }
      await login(email, password);
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
          <label>{ueberVerzeichnis ? "Benutzer" : "E-Mail"}</label>
          <input
            type={ueberVerzeichnis ? "text" : "email"}
            value={email}
            onChange={(e) => setEmail(e.target.value)}
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
                ? "Stattdessen mit E-Mail und Passwort anmelden"
                : "Stattdessen über das Verzeichnis anmelden"}
            </button>
          </div>
        )}

        {wege?.oidc && (
          <>
            <div className="anmelde-trenner">
              <span>oder</span>
            </div>
            {/* Ein gewöhnlicher Verweis, kein fetch: der Anbieter antwortet mit
                einer Weiterleitung auf seine eigene Seite, und die muss der
                Browser selbst ansteuern. */}
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
