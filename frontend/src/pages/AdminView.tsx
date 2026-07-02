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

  const removeUser = async (id: string, label: string) => {
    if (!confirm(`Delete user ${label}? Their pages are removed too. This cannot be undone.`)) return;
    await api.deleteUser(id);
    refresh();
  };

  return (
    <div className="editor-scroll">
      <div className="page wide">
        <h1 className="view-title">Users &amp; roles</h1>
        <p className="muted">Admins can read and edit every page in the workspace.</p>

        <div className="user-add">
          <div className="modal-label">Add a user</div>
          <div className="user-add-row">
            <input placeholder="email" value={email} onChange={(e) => setEmail(e.target.value)} />
            <input placeholder="name (optional)" value={name} onChange={(e) => setName(e.target.value)} />
            <input
              type="password"
              placeholder="password (min. 6)"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && addUser()}
            />
            <select value={role} onChange={(e) => setRole(e.target.value)}>
              <option value="user">User</option>
              <option value="admin">Admin</option>
            </select>
            <button className="btn btn-primary" disabled={busy} onClick={addUser}>
              {busy ? "…" : "Add user"}
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
                <select
                  value={u.role}
                  disabled={u.id === user?.id}
                  onChange={(e) => setUserRole(u.id, e.target.value)}
                >
                  <option value="user">User</option>
                  <option value="admin">Admin</option>
                </select>
                <button
                  className="btn danger"
                  disabled={u.id === user?.id}
                  onClick={() => removeUser(u.id, u.email)}
                >
                  Delete
                </button>
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
