import { useState } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "../auth";

export default function Login() {
  const { login } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
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
          <label>E-Mail</label>
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoFocus />
        </div>
        <div className="field">
          <label>Passwort</label>
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
        <button className="btn-primary" type="submit" disabled={busy}>
          {busy ? "Anmelden…" : "Anmelden"}
        </button>
        <div className="switch">
          Kein Konto? <Link to="/register">Registrieren</Link>
        </div>
      </form>
    </div>
  );
}
