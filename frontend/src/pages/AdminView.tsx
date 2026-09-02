// Account administration, for administrators only. Every action is checked
// again in the backend; hiding the view is convenience, not protection.
//
// It is shown inside the settings and therefore no longer brings a frame of its
// own; heading and spacing come from there.
//
// Die Konten stehen als Tabelle. Sie waren einmal eine Reihe von Zeilen, in
// denen Name, Adresse und Anmeldename hintereinander weg liefen und jede Zeile
// woanders anfing. Bei drei Konten geht das, bei dreissig sucht man. In Spalten
// steht jede Angabe untereinander und laesst sich vergleichen.
import { useEffect, useState } from "react";
import { User, api } from "../api/client";
import { useAuth } from "../auth";
import { useEingabe, useRueckfrage } from "../components/Rueckfrage";

export default function AdminView() {
  const frage = useRueckfrage();
  const eingabeFragen = useEingabe();
  const { user } = useAuth();
  const [users, setUsers] = useState<User[]>([]);
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [benutzername, setBenutzername] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("user");
  const [err, setErr] = useState("");
  const [hinweis, setHinweis] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = () => api.listUsers().then(setUsers).catch(() => setUsers([]));
  useEffect(() => {
    refresh();
  }, []);

  const setUserRole = async (id: string, r: string) => {
    await api.setUserRole(id, r);
    refresh();
  };

  // Der Anmeldename eines Kontos. Wird hier vergeben, weil er an Konten fehlen
  // kann, die über das Verzeichnis oder in einer älteren Fassung entstanden
  // sind -- die melden sich sonst weiter nur über ihre Adresse an.
  const setBenutzernameVon = async (u: User) => {
    const eingabe = await eingabeFragen({
      titel: "Anmeldename",
      text: `Womit sich ${u.name} statt mit der E-Mail-Adresse anmelden kann. Leer lassen entfernt ihn.`,
      feld: "Benutzername",
      vorgabe: u.benutzername,
      bestaetigen: "Speichern",
    });
    if (eingabe === null) return;
    setErr("");
    try {
      await api.benutzernameSetzen(u.id, eingabe.trim());
      refresh();
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  // Ein vergessenes Passwort. Die Verwaltung setzt ein neues und beendet damit
  // jede Sitzung des Kontos: wer ein Passwort zurücksetzen lässt, hat in aller
  // Regel den Verdacht, dass jemand anders daran sitzt.
  const passwortSetzenVon = async (u: User) => {
    const neu = await eingabeFragen({
      titel: "Passwort zurücksetzen",
      text:
        `Ein neues Passwort für ${u.name}. Alle Sitzungen dieses Kontos werden dabei ` +
        `beendet, es muss sich überall neu anmelden. Sag es ihm auf einem anderen Weg als ` +
        `per E-Mail, und lass es danach selbst eines wählen.`,
      feld: "Neues Passwort",
      art: "passwort",
      bestaetigen: "Passwort setzen",
    });
    if (neu === null) return;
    setErr("");
    try {
      const { beendet } = await api.passwortSetzen(u.id, neu);
      setHinweis(
        `Passwort für ${u.name} gesetzt` +
          (beendet > 0 ? `, ${beendet} ${beendet === 1 ? "Sitzung" : "Sitzungen"} beendet` : ""),
      );
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  const addUser = async () => {
    setErr("");
    setBusy(true);
    try {
      await api.createUser(email.trim(), name.trim(), password, role, benutzername.trim());
      setEmail("");
      setName("");
      setBenutzername("");
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
    if (
      !(await frage({
        titel: "Nutzer löschen",
        text: `Das Konto ${label} wird gelöscht, seine Seiten werden mit entfernt. Das lässt sich nicht rückgängig machen.`,
        bestaetigen: "Nutzer löschen",
        gefaehrlich: true,
      }))
    )
      return;
    await api.deleteUser(id);
    refresh();
  };

  return (
    <>
      <h3>Nutzer &amp; Rollen</h3>
      <p className="muted small">
        Administratoren können jede Seite im Arbeitsbereich lesen und bearbeiten.
      </p>

      <h3>Konto anlegen</h3>
      <div className="einstellung">
        <div className="s3-felder">
          <label>
            <span>E-Mail</span>
            <input value={email} onChange={(e) => setEmail(e.target.value)} />
          </label>
          <label>
            <span>Name</span>
            <input placeholder="optional" value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <label>
            <span>Anmeldename</span>
            <input
              placeholder="optional"
              value={benutzername}
              onChange={(e) => setBenutzername(e.target.value)}
            />
          </label>
          <label>
            <span>Passwort</span>
            <input
              type="password"
              placeholder="mindestens 6 Zeichen"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && addUser()}
            />
          </label>
          <label>
            <span>Rolle</span>
            <select value={role} onChange={(e) => setRole(e.target.value)}>
              <option value="user">Nutzer</option>
              <option value="admin">Admin</option>
            </select>
          </label>
        </div>
        <div className="einstellung-aktionen">
          <button className="btn" disabled={busy || !email.trim()} onClick={addUser}>
            {busy ? "Legt an…" : "Anlegen"}
          </button>
        </div>
        {err && <div className="fehler">{err}</div>}
      </div>

      {hinweis && <div className="hinweis-ok">{hinweis}</div>}

      <h3>Vorhandene Konten</h3>
      <div className="tabelle-rollen">
        <table className="tabelle konten-tabelle">
          <thead>
            <tr>
              <th>Name</th>
              <th>E-Mail</th>
              <th>Anmeldename</th>
              <th>Rolle</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id}>
                <td>
                  {u.name}
                  {u.id === user?.id && <span className="muted"> (du)</span>}
                </td>
                <td className="muted">{u.email}</td>
                <td className="muted">
                  {u.benutzername || <span className="muted">nicht vergeben</span>}
                </td>
                <td>
                  {/* Die eigene Zeile ist verriegelt: ein Administrator darf
                      sich weder herabstufen noch loeschen. Das haelt nebenbei
                      den letzten Administrator an seinem Platz. Ein Schild mit
                      der Rolle steht nicht mehr daneben, das Auswahlfeld zeigt
                      sie ja bereits an. */}
                  <select
                    value={u.role}
                    disabled={u.id === user?.id}
                    onChange={(e) => setUserRole(u.id, e.target.value)}
                  >
                    <option value="user">Nutzer</option>
                    <option value="admin">Admin</option>
                  </select>
                </td>
                <td className="zeilen-aktionen">
                  <button className="btn-schlicht" onClick={() => setBenutzernameVon(u)}>
                    {u.benutzername ? "Anmeldename ändern" : "Anmeldename setzen"}
                  </button>
                  {/* Die eigene Zeile bleibt aus: dieser Weg beendet alle
                      Sitzungen, und eine Verwaltung, die sich damit selbst
                      aussperrt, hat nichts gewonnen. Das eigene Passwort
                      wechselt man unten in der Leiste. */}
                  <button
                    className="btn-schlicht"
                    disabled={u.id === user?.id}
                    title={
                      u.id === user?.id
                        ? "Das eigene Passwort wechselt man unten in der Leiste"
                        : undefined
                    }
                    onClick={() => passwortSetzenVon(u)}
                  >
                    Passwort zurücksetzen
                  </button>
                  <button
                    className="btn-schlicht gefaehrlich"
                    disabled={u.id === user?.id}
                    onClick={() => removeUser(u.id, u.email)}
                  >
                    Löschen
                  </button>
                </td>
              </tr>
            ))}
            {users.length === 0 && (
              <tr>
                <td colSpan={5} className="muted">
                  Noch kein Konto angelegt.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </>
  );
}
