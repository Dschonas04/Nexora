// Kontenverwaltung, nur für Administratoren. Jede Aktion wird im Backend noch
// einmal geprüft; diese Ansicht zu verbergen ist Bequemlichkeit, kein Schutz.
//
// Sie steht innerhalb der Einstellungen und bringt deshalb keinen eigenen
// Rahmen mit; Überschrift und Abstände kommen von dort.
//
// Auf der Seite steht nur noch die Liste. Vorher stand darüber ein Formular zum
// Anlegen mit fünf Feldern, das die obere Hälfte des Bildschirms für etwas
// belegte, das man einmal in der Woche braucht -- die Liste, um die es hier
// geht, fing erst darunter an. Anlegen und Ändern gehen jetzt in einem Fenster
// auf, und die Liste hat die Seite für sich.
import { useEffect, useState } from "react";
import { User, api } from "../api/client";
import { useAuth } from "../auth";
import Fenster from "../components/Fenster";
import KurzeZeilen from "../components/Kurzliste";
import Listenkopf from "../components/Listenkopf";
import { useRueckfrage } from "../components/Rueckfrage";

/** Eine Zeile aus dem Feld für mehrere Konten, schon zerlegt. */
interface Anzulegen {
  email: string;
  name: string;
  benutzername: string;
  rolle: string;
  passwort: string;
}

/** Das Ergebnis eines Anlegeversuchs, für die Liste am Ende. */
interface Ergebnis extends Anzulegen {
  fehler?: string;
}

/**
 * Ein Passwort für ein frisch angelegtes Konto.
 *
 * Aus dem Zufallsgenerator des Browsers und nicht aus Math.random: das hier ist
 * ein Zugangsdatum, auch wenn es nur bis zur ersten eigenen Änderung gilt. Die
 * verwechselbaren Zeichen fehlen im Vorrat, denn es wird vorgelesen oder
 * abgeschrieben.
 */
function neuesPasswort(): string {
  const vorrat = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789";
  const roh = new Uint32Array(14);
  crypto.getRandomValues(roh);
  return Array.from(roh, (z) => vorrat[z % vorrat.length]).join("");
}

/**
 * Eine Zeile aus dem Feld für mehrere Konten lesen.
 *
 * Angenommen wird alles, was mit einer Adresse anfängt, getrennt durch Komma,
 * Semikolon oder Tabulator -- eine Tabellenzeile aus einem Kalkulationsblatt
 * fällt so hinein, ohne dass sie jemand umschreibt. Was fehlt, wird ergänzt:
 * ohne Namen der Teil vor dem @, ohne Rolle "user".
 */
function zeileLesen(zeile: string): Anzulegen | null {
  const teile = zeile.split(/[;,\t]/).map((t) => t.trim());
  const email = (teile[0] ?? "").toLowerCase();
  if (!email || !email.includes("@")) return null;
  const rolle = (teile[3] ?? teile[2] ?? "").toLowerCase();
  return {
    email,
    name: teile[1] || email.split("@")[0],
    benutzername: teile[2] && !/^(admin|user|nutzer)$/.test(teile[2]) ? teile[2] : "",
    rolle: rolle === "admin" ? "admin" : "user",
    passwort: neuesPasswort(),
  };
}

export default function AdminView() {
  const frage = useRueckfrage();
  const { user } = useAuth();
  const [users, setUsers] = useState<User[]>([]);
  const [hinweis, setHinweis] = useState("");
  // Ein Filter statt einer Suche im Browser. Ab etwa zwanzig Konten ist das
  // Blättern durch die Tabelle länger als das Tippen von drei Buchstaben.
  const [filter, setFilter] = useState("");
  // Welches Fenster offen ist: keines, das zum Anlegen, oder das eines Kontos.
  const [anlegen, setAnlegen] = useState(false);
  const [bearbeitet, setBearbeitet] = useState<User | null>(null);

  const refresh = () => api.listUsers().then(setUsers).catch(() => setUsers([]));
  useEffect(() => {
    refresh();
  }, []);

  const suche = filter.trim().toLowerCase();
  const sichtbar = suche
    ? users.filter((u) =>
        [u.name, u.email, u.benutzername].some((f) => (f ?? "").toLowerCase().includes(suche)),
      )
    : users;

  // Das Konto im Fenster kommt aus der frisch geladenen Liste, nicht aus der
  // Kopie von damals: sonst zeigte das Fenster nach einer Änderung noch den
  // alten Stand, während die Zeile dahinter bereits den neuen hat.
  const offenesKonto = bearbeitet ? (users.find((u) => u.id === bearbeitet.id) ?? null) : null;

  return (
    <>
      <Listenkopf
        titel="Users and roles"
        zahl={
          `${users.length} gesamt · ${users.filter((u) => u.role === "admin").length} mit ` +
          `Verwaltungsrecht · ${users.filter((u) => u.zweitfaktor).length} mit zweitem Faktor`
        }
        filter={filter}
        setFilter={setFilter}
        platzhalter="Filter by name, address, login name"
      >
        <button className="btn btn-primary" onClick={() => setAnlegen(true)}>
          Create accounts
        </button>
      </Listenkopf>
      <p className="muted small">
        Administrators may read and edit every page in the workspace. Roles take effect
        at once; a running session is not ended for it.
      </p>

      {hinweis && <div className="hinweis-ok">{hinweis}</div>}

      <div className="tabelle-rollen">
        <table className="tabelle konten-tabelle">
          <thead>
            <tr>
              <th>Name</th>
              <th>Email</th>
              <th>Login name</th>
              <th>Role</th>
              <th>Second factor</th>
              <th>Created</th>
              <th />
            </tr>
          </thead>
          <tbody>
            <KurzeZeilen
              alle={sichtbar}
              spalten={7}
              zeile={(u) => (
              <tr key={u.id}>
                <td>
                  {u.name}
                  {u.id === user?.id && <span className="muted"> (du)</span>}
                </td>
                <td className="muted">{u.email}</td>
                <td className="muted">
                  {u.benutzername || <span className="muted">not set</span>}
                </td>
                <td className="muted">{u.role === "admin" ? "Admin" : "User"}</td>
                <td className={u.zweitfaktor ? undefined : "muted"}>
                  {u.zweitfaktor ? "steht" : "keiner"}
                </td>
                <td className="muted einzeilig">
                  {u.createdAt ? new Date(u.createdAt).toLocaleDateString() : ""}
                </td>
                <td className="zeilen-aktionen">
                  {/* Ein Knopf statt vier. Was mit einem Konto geht, steht im
                      Fenster beieinander; in der Zeile war es eine Reihe, die
                      mit jeder neuen Möglichkeit länger wurde. */}
                  <button className="btn-schlicht" onClick={() => setBearbeitet(u)}>
                    Manage
                  </button>
                </td>
              </tr>
              )}
            />
            {sichtbar.length === 0 && (
              <tr>
                <td colSpan={7} className="muted">
                  {users.length === 0
                    ? "No account created yet."
                    : "No account matches the filter."}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {anlegen && (
        <AnlegenFenster
          schliessen={() => {
            setAnlegen(false);
            refresh();
          }}
        />
      )}
      {offenesKonto && (
        <KontoFenster
          konto={offenesKonto}
          selbst={offenesKonto.id === user?.id}
          frage={frage}
          melden={setHinweis}
          nachladen={refresh}
          schliessen={() => setBearbeitet(null)}
        />
      )}
    </>
  );
}

/**
 * Das Fenster zum Anlegen -- für ein Konto und für dreißig.
 *
 * Beides in einem Fenster mit einer Umschaltung, weil es dieselbe Sache ist:
 * eine neue Mannschaft trägt man nicht einzeln ein, ein Nachzügler nicht als
 * Liste. Die Passwörter entstehen hier und stehen danach genau einmal da; in
 * der Datenbank liegt nur ihr Hash.
 */
function AnlegenFenster({ schliessen }: { schliessen: () => void }) {
  const [mehrere, setMehrere] = useState(false);
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [benutzername, setBenutzername] = useState("");
  const [passwort, setPasswort] = useState("");
  const [rolle, setRolle] = useState("user");
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [fertig, setFertig] = useState<Ergebnis[] | null>(null);

  const zeilen = text
    .split("\n")
    .map(zeileLesen)
    .filter((z): z is Anzulegen => z !== null);

  const anlegen = async () => {
    const liste: Anzulegen[] = mehrere
      ? zeilen
      : [
          {
            email: email.trim().toLowerCase(),
            name: name.trim() || email.trim().split("@")[0],
            benutzername: benutzername.trim(),
            rolle,
            passwort: passwort || neuesPasswort(),
          },
        ];
    if (liste.length === 0) return;

    setBusy(true);
    const ergebnisse: Ergebnis[] = [];
    // Nacheinander und nicht alle auf einmal: bcrypt mit Kostenfaktor 12
    // braucht pro Konto ein paar Zehntelsekunden, und dreißig gleichzeitige
    // Anfragen legen dafür dreißig Verbindungen an.
    for (const z of liste) {
      try {
        await api.createUser(z.email, z.name, z.passwort, z.rolle, z.benutzername);
        ergebnisse.push(z);
      } catch (e) {
        ergebnisse.push({ ...z, fehler: (e as Error).message });
      }
    }
    setBusy(false);
    setFertig(ergebnisse);
  };

  // Die Liste zum Weitergeben. Als Text mit Tabulatoren, damit sie in einer
  // Tabelle wieder in Spalten fällt.
  const kopieren = () => {
    if (!fertig) return;
    const text = fertig
      .filter((e) => !e.fehler)
      .map((e) => [e.email, e.name, e.passwort].join("\t"))
      .join("\n");
    navigator.clipboard?.writeText(text).catch(() => {});
  };

  if (fertig) {
    const gut = fertig.filter((e) => !e.fehler);
    return (
      <Fenster
        titel="Created"
        unter={`${gut.length} von ${fertig.length} Konten`}
        breit
        schliessen={schliessen}
        fuss={
          <>
            <button className="btn" onClick={kopieren} disabled={gut.length === 0}>
              Copy the list
            </button>
            <button className="btn btn-primary" onClick={schliessen}>
              Done
            </button>
          </>
        }
      >
        {/* Die Passwörter stehen hier zum einzigen Mal. Wer das Fenster
            schließt, ohne sie mitzunehmen, muss sie zurücksetzen -- deshalb der
            Satz und nicht bloß die Tabelle. */}
        <p className="muted small">
          The passwords are shown in this window only. Hand them over by some route
          other than email, and let everyone pick their own afterwards.
        </p>
        <table className="tabelle">
          <thead>
            <tr>
              <th>Email</th>
              <th>Password</th>
              <th>State</th>
            </tr>
          </thead>
          <tbody>
            {fertig.map((e) => (
              <tr key={e.email}>
                <td>{e.email}</td>
                <td>{e.fehler ? <span className="muted">—</span> : <code>{e.passwort}</code>}</td>
                <td className={e.fehler ? "fehlertext" : "muted"}>{e.fehler ?? "angelegt"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Fenster>
    );
  }

  return (
    <Fenster
      titel="Create accounts"
      unter={mehrere ? "One line per account" : "One account"}
      breit={mehrere}
      schliessen={schliessen}
      fuss={
        <>
          <button className="btn" onClick={() => setMehrere((v) => !v)}>
            {mehrere ? "Create just one" : "Several at once"}
          </button>
          <span className="fuss-luecke" />
          <button className="btn" onClick={schliessen}>
            Cancel
          </button>
          <button
            className="btn btn-primary"
            disabled={busy || (mehrere ? zeilen.length === 0 : !email.includes("@"))}
            onClick={anlegen}
          >
            {busy
              ? "Creating…"
              : mehrere
                ? `Create ${zeilen.length} ${zeilen.length === 1 ? "account" : "accounts"}`
                : "Create"}
          </button>
        </>
      }
    >
      {mehrere ? (
        <>
          <p className="muted small">
            One email address per line, then name, login name and role, separated by
            comma, semicolon or tab. Everything except the address may be left out. A
            column selection from a spreadsheet can be pasted unchanged.
          </p>
          <textarea
            className="mengenfeld"
            rows={9}
            spellCheck={false}
            placeholder={
              "anna.beck@firma.de, Anna Beck, abeck, admin\n" +
              "tom.roth@firma.de, Tom Roth\n" +
              "kiosk@firma.de"
            }
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
          <div className="muted small">
            {zeilen.length === 0
              ? "No usable line yet."
              : `${zeilen.length} ${zeilen.length === 1 ? "line" : "lines"} recognised · ` +
                `${zeilen.filter((z) => z.rolle === "admin").length} davon als Admin · ` +
                "the passwords are generated and shown afterwards"}
          </div>
        </>
      ) : (
        <div className="fenster-felder">
          <label>
            <span>Email</span>
            <input value={email} onChange={(e) => setEmail(e.target.value)} />
          </label>
          <label>
            <span>Name</span>
            <input
              placeholder={email.includes("@") ? email.split("@")[0] : "optional"}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </label>
          <label>
            <span>Login name</span>
            <input
              placeholder="optional"
              value={benutzername}
              onChange={(e) => setBenutzername(e.target.value)}
            />
          </label>
          <label>
            <span>Role</span>
            <select value={rolle} onChange={(e) => setRolle(e.target.value)}>
              <option value="user">User</option>
              <option value="admin">Admin</option>
            </select>
          </label>
          <label className="feld-breit">
            <span>Password</span>
            <input
              value={passwort}
              placeholder="leer: wird erzeugt und danach angezeigt"
              onChange={(e) => setPasswort(e.target.value)}
            />
          </label>
        </div>
      )}
    </Fenster>
  );
}

/**
 * Alles, was mit einem vorhandenen Konto geht, in einem Fenster.
 *
 * Die eigene Zeile ist an mehreren Stellen verriegelt: eine Verwaltung darf
 * sich weder herabstufen noch löschen noch das eigene Passwort auf diesem Weg
 * setzen. Das hält nebenbei den letzten Administrator an seinem Platz.
 */
function KontoFenster({
  konto,
  selbst,
  frage,
  melden,
  nachladen,
  schliessen,
}: {
  konto: User;
  selbst: boolean;
  frage: ReturnType<typeof useRueckfrage>;
  melden: (t: string) => void;
  nachladen: () => void;
  schliessen: () => void;
}) {
  const [rolle, setRolle] = useState(konto.role);
  const [benutzername, setBenutzername] = useState(konto.benutzername ?? "");
  const [neu, setNeu] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const speichern = async () => {
    setErr("");
    setBusy(true);
    try {
      if (rolle !== konto.role) await api.setUserRole(konto.id, rolle);
      if (benutzername.trim() !== (konto.benutzername ?? ""))
        await api.benutzernameSetzen(konto.id, benutzername.trim());
      nachladen();
      schliessen();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  // Ein neues Passwort beendet jede Sitzung des Kontos: wer eines zurücksetzen
  // lässt, hat in aller Regel den Verdacht, dass jemand anders daran sitzt.
  const passwortSetzen = async () => {
    setErr("");
    setBusy(true);
    try {
      const { beendet } = await api.passwortSetzen(konto.id, neu);
      setNeu("");
      melden(
        `Password set for ${konto.name}` +
          (beendet > 0 ? `, ${beendet} ${beendet === 1 ? "session" : "sessions"} ended` : ""),
      );
      schliessen();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const zweitfaktorWeg = async () => {
    if (
      !(await frage({
        titel: "Remove the second factor",
        text:
          `${konto.name} then signs in with the password alone again until a new one is ` +
          `set up. Take this route for a lost phone. Make sure beforehand, over some other ` +
          `channel, that the request really comes from this person.`,
        bestaetigen: "Remove",
        gefaehrlich: true,
      }))
    )
      return;
    try {
      await api.zweitfaktorZuruecksetzen(konto.id);
      melden(`Zweiter Faktor von ${konto.name} entfernt.`);
      nachladen();
      schliessen();
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  // Ein Konto zu löschen nimmt seine Seiten mit, über die Kaskade in der
  // Datenbank, und dafür gibt es keinen Papierkorb.
  const loeschen = async () => {
    if (
      !(await frage({
        titel: "Delete user",
        text: `The account ${konto.email} is deleted and its pages are removed with it. This cannot be undone.`,
        bestaetigen: "Delete user",
        gefaehrlich: true,
      }))
    )
      return;
    try {
      await api.deleteUser(konto.id);
      nachladen();
      schliessen();
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  const geaendert =
    rolle !== konto.role || benutzername.trim() !== (konto.benutzername ?? "");

  return (
    <Fenster
      titel={konto.name}
      unter={konto.email}
      schliessen={schliessen}
      fuss={
        <>
          <button className="btn gefaehrlich" disabled={selbst || busy} onClick={loeschen}>
            Delete
          </button>
          <span className="fuss-luecke" />
          <button className="btn" onClick={schliessen}>
            Cancel
          </button>
          <button className="btn btn-primary" disabled={!geaendert || busy} onClick={speichern}>
            Save
          </button>
        </>
      }
    >
      {err && <div className="fehler">{err}</div>}
      <div className="fenster-felder">
        <label>
          <span>Role</span>
          <select value={rolle} disabled={selbst} onChange={(e) => setRolle(e.target.value)}>
            <option value="user">User</option>
            <option value="admin">Admin</option>
          </select>
        </label>
        <label>
          <span>Login name</span>
          <input
            placeholder="nicht vergeben"
            value={benutzername}
            onChange={(e) => setBenutzername(e.target.value)}
          />
        </label>
      </div>
      {selbst && (
        <p className="muted small">
          Das eigene Konto: Rolle und Löschen sind gesperrt, das eigene Passwort wechselt man
          unten in der Leiste.
        </p>
      )}

      <div className="fenster-abschnitt">
        <div className="modal-label">Reset password</div>
        <p className="muted small">
          Every session of this account is ended in the process. Tell the new password by
          some route other than email, and let them pick their own afterwards.
        </p>
        <div className="fenster-zeile">
          <input
            value={neu}
            disabled={selbst}
            placeholder="New password, at least 6 characters"
            onChange={(e) => setNeu(e.target.value)}
          />
          <button className="btn" disabled={selbst || busy || neu.length < 6} onClick={passwortSetzen}>
            Set
          </button>
          <button className="btn" disabled={selbst} onClick={() => setNeu(neuesPasswort())}>
            Generate
          </button>
        </div>
      </div>

      <div className="fenster-abschnitt">
        <div className="modal-label">Second factor</div>
        {konto.zweitfaktor ? (
          <div className="fenster-zeile">
            <span className="muted small">
              In place. The account is asked for a code at every sign-in.
            </span>
            <button className="btn" onClick={zweitfaktorWeg}>
              Remove
            </button>
          </div>
        ) : (
          <p className="muted small">
            None set up. Only the account itself can set one up, under Access; the
            administration can only remove it.
          </p>
        )}
      </div>
    </Fenster>
  );
}
