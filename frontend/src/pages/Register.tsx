// Sign-up form. The first account created in a fresh instance becomes the
// workspace admin, which the backend decides.
import { useState } from "react";
import { Link } from "react-router";
import { useAuth } from "../auth";

export default function Register() {
  const { register } = useAuth();
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [benutzername, setBenutzername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await register(email, name, password, benutzername);
    } catch (err) {
      setError((err as Error).message || "Registrierung fehlgeschlagen");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="auth">
      <form className="auth-card" onSubmit={submit}>
        <h1>Konto erstellen</h1>
        <p className="sub">Starte deinen Nexora-Workspace</p>
        {error && <div className="error">{error}</div>}
        <div className="field">
          <label>Name</label>
          <input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </div>
        <div className="field">
          <label>E-Mail</label>
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
        </div>
        <div className="field">
          <label>Benutzername</label>
          <input
            value={benutzername}
            onChange={(e) => setBenutzername(e.target.value)}
            placeholder="freiwillig, sonst aus der E-Mail"
            autoComplete="username"
          />
          <div className="muted small">
            Damit kannst du dich statt mit der E-Mail-Adresse anmelden. Kleinbuchstaben,
            Ziffern, Punkt, Strich und Unterstrich.
          </div>
        </div>
        <div className="field">
          <label>Passwort</label>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="mind. 6 Zeichen"
          />
        </div>
        <button className="btn-primary" type="submit" disabled={busy}>
          {busy ? "Erstellen…" : "Konto erstellen"}
        </button>
        <div className="switch">
          Schon ein Konto? <Link to="/login">Anmelden</Link>
        </div>
      </form>
    </div>
  );
}
