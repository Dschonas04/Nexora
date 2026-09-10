// Sign-in form. On success AuthProvider sets the user, and App swaps this
// screen for the workspace; there is no explicit redirect here.
import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api, brauchtZweitenSchritt } from "../api/client";
import { useAuth } from "../auth";

export default function Login() {
  const { login, zweiterSchritt } = useAuth();
  // Eine Zeile für beides. Was drin steht, entscheidet der Server am @: eine
  // Auswahl davor wäre eine Frage, die niemand beantworten müsste.
  const [kennung, setKennung] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  // Das Ticket aus dem ersten Schritt. Solange es steht, ist die Anmeldung
  // halb fertig: das Passwort stimmte, der Code fehlt. Es hält keine Sitzung,
  // gilt fünf Minuten und öffnet für sich genommen nichts.
  const [ticket, setTicket] = useState("");
  const [code, setCode] = useState("");

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
      const antwort = ueberVerzeichnis
        ? await api.ldapAnmelden(kennung, password)
        : await login(kennung, password);
      if (brauchtZweitenSchritt(antwort)) {
        setTicket(antwort.ticket);
        setCode("");
        return;
      }
      // Beim Verzeichnis sitzt die Sitzung im Keks, ohne dass der Zustand hier
      // davon weiß. Ein Neuladen lässt AuthProvider sie lesen.
      if (ueberVerzeichnis) window.location.href = "/";
    } catch (err) {
      setError((err as Error).message || "Anmeldung fehlgeschlagen");
    } finally {
      setBusy(false);
    }
  };

  const codeSenden = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await zweiterSchritt(ticket, code);
    } catch (err) {
      setError((err as Error).message || "Der Code stimmt nicht");
      setCode("");
    } finally {
      setBusy(false);
    }
  };

  // Der zweite Schritt bekommt eine eigene Karte statt eines dritten Feldes im
  // Formular. Wer hier steht, hat das Passwort hinter sich; ein Bildschirm mit
  // genau einer Frage darauf lässt keinen Zweifel, welche gerade dran ist.
  if (ticket) {
    return (
      <div className="auth">
        <form className="auth-card" onSubmit={codeSenden}>
          <h1>Nexora</h1>
          <p className="sub">Second step</p>
          {error && <div className="error">{error}</div>}
          <div className="field">
            <label>Code from your authenticator app</label>
            <input
              className="codefeld"
              inputMode="text"
              autoComplete="one-time-code"
              placeholder="123456"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              autoFocus
            />
          </div>
          <p className="muted small">
            Sechs Ziffern aus der App. Ist das Telefon nicht zur Hand, tut es auch einer
            der Ersatzcodes; jeder davon gilt einmal.
          </p>
          <button className="btn-primary" type="submit" disabled={busy || !code.trim()}>
            {busy ? "Checking…" : "Sign in"}
          </button>
          <div className="switch">
            <button
              type="button"
              className="link-btn"
              onClick={() => {
                setTicket("");
                setPassword("");
                setError("");
              }}
            >
              Back to sign-in
            </button>
          </div>
        </form>
      </div>
    );
  }

  return (
    <div className="auth">
      <form className="auth-card" onSubmit={submit}>
        <h1>Nexora</h1>
        <p className="sub">Sign in to your workspace</p>
        {error && <div className="error">{error}</div>}
        <div className="field">
          <label>{ueberVerzeichnis ? "User" : "Email or username"}</label>
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
          <label>Password</label>
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
        <button className="btn-primary" type="submit" disabled={busy}>
          {busy ? "Signing in…" : "Sign in"}
        </button>
        {wege?.ldap && (
          <div className="switch">
            <button
              type="button"
              className="link-btn"
              onClick={() => setUeberVerzeichnis((v) => !v)}
            >
              {ueberVerzeichnis
                ? "Sign in with account and password instead"
                : "Sign in through the directory instead"}
            </button>
          </div>
        )}

        {wege?.oidc && (
          <>
            <div className="anmelde-trenner">
              <span>or</span>
            </div>
            {/* An ordinary link, not a fetch: the provider answers with a
                redirect to its own page, and the browser has to go there
                itself. */}
            <a className="btn-primary anmelde-sso" href="/api/auth/oidc/start">
              {wege.oidcText || `Sign in with ${wege.anbieter || "SSO"}`}
            </a>
          </>
        )}

        <div className="switch">
          No account? <Link to="/register">Register</Link>
        </div>
      </form>
    </div>
  );
}
