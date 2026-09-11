// Sign-in form. On success AuthProvider sets the user, and App swaps this
// screen for the workspace; there is no explicit redirect here.
import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { api, brauchtZweitenSchritt } from "../api/client";
import { useAuth } from "../auth";

export default function Login() {
  const { login, zweiterSchritt } = useAuth();
  // One line for both. What is in it is decided by the server at the @: a
  // dropdown in front of it would be a question nobody would have to answer.
  const [kennung, setKennung] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  // The ticket from the first step. As long as it stands, the sign-in is half
  // done: the password was right, the code is missing. It holds no session, is
  // valid for five minutes and opens nothing by itself.
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
      // With the directory the session sits in the cookie without the state
      // here knowing about it. A reload lets AuthProvider read it.
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

  // The second step gets a card of its own instead of a third field in the
  // form. Whoever stands here has the password behind them; a screen with
  // exactly one question on it leaves no doubt which one is up now.
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
            Six digits from the app. If the phone is out of reach, one of the recovery
            codes works too; each of them works once.
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
          {/* type="text" even without a directory: with type="email" the
              browser takes every input without an @ for a typo and does not
              even let the form be submitted. */}
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
