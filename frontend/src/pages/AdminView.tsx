import { useEffect, useState } from "react";
import { User, api } from "../api/client";
import { useAuth } from "../auth";

export default function AdminView() {
  const { user } = useAuth();
  const [users, setUsers] = useState<User[]>([]);

  const refresh = () => api.listUsers().then(setUsers).catch(() => setUsers([]));
  useEffect(() => {
    refresh();
  }, []);

  const setRole = async (id: string, role: string) => {
    await api.setUserRole(id, role);
    refresh();
  };

  return (
    <div className="editor-scroll">
      <div className="page wide">
        <h1 className="view-title">Users &amp; roles</h1>
        <p className="muted">Admins can read and edit every page in the workspace.</p>
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
                  onChange={(e) => setRole(u.id, e.target.value)}
                >
                  <option value="user">User</option>
                  <option value="admin">Admin</option>
                </select>
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
