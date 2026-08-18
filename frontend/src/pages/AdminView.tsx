// Account administration, reachable only for admins. Every action here is
// enforced again in the backend; hiding the view is convenience, not security.
import { useEffect, useState } from "react";
import { User, api } from "../api/client";
import { useAuth } from "../auth";

export default function AdminView() {
  const { user } = useAuth();
  const [users, setUsers] = useState<User[]>([]);
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("user");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = () => api.listUsers().then(setUsers).catch(() => setUsers([]));
  useEffect(() => {
    refresh();
  }, []);

  const setUserRole = async (id: string, r: string) => {
    await api.setUserRole(id, r);
    refresh();
  };

  const addUser = async () => {
    setErr("");
    setBusy(true);
    try {
      await api.createUser(email.trim(), name.trim(), password, role);
      setEmail("");
      setName("");
      setPassword("");
      setRole("user");
      refresh();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  // Deleting an account takes its pages with it through the database cascade,
  // and there is no trash for that, hence the explicit warning.
  const removeUser = async (id: string, label: string) => {
    if (!confirm(`Nutzer ${label} löschen? Seine Seiten werden ebenfalls entfernt. Das kann nicht rückgängig gemacht werden.`)) return;
    await api.deleteUser(id);
    refresh();
  };

  return (
    <div className="editor-scroll">
      <div className="page wide">
        <h1 className="view-title">Nutzer &amp; Rollen</h1>
        <p className="muted">Admins können jede Seite im Workspace lesen und bearbeiten.</p>

        <div className="user-add">
          <div className="modal-label">Nutzer hinzufügen</div>
          <div className="user-add-row">
            <input placeholder="E-Mail" value={email} onChange={(e) => setEmail(e.target.value)} />
            <input placeholder="Name (optional)" value={name} onChange={(e) => setName(e.target.value)} />
            <input
              type="password"
              placeholder="Passwort (mind. 6)"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && addUser()}
            />
            <select value={role} onChange={(e) => setRole(e.target.value)}>
              <option value="user">Nutzer</option>
              <option value="admin">Admin</option>
            </select>
            <button className="btn btn-primary" disabled={busy} onClick={addUser}>
              {busy ? "…" : "Hinzufügen"}
            </button>
          </div>
          {err && <div className="error">{err}</div>}
        </div>

        <div className="list" style={{ marginTop: 18 }}>
          {users.map((u) => (
            <div key={u.id} className="list-row">
              <span className="list-title">
                {u.name} <span className="muted small">{u.email}</span>
              </span>
              <span className="row-actions">
                <span className={"pill" + (u.role === "admin" ? " admin" : "")}>{u.role}</span>
                {/* Your own row is locked: an admin may not demote or delete
                    themselves, which also keeps the last admin in place. */}
                <select
                  value={u.role}
                  disabled={u.id === user?.id}
                  onChange={(e) => setUserRole(u.id, e.target.value)}
                >
                  <option value="user">Nutzer</option>
                  <option value="admin">Admin</option>
                </select>
                <button
                  className="btn danger"
                  disabled={u.id === user?.id}
                  onClick={() => removeUser(u.id, u.email)}
                >
                  Löschen
                </button>
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
