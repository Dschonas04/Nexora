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
      <div className="listenkopf">
        <h3>
          Nutzer und Rollen
          <span className="muted small">
            {" "}
            {users.length} gesamt · {users.filter((u) => u.role === "admin").length} mit
            Verwaltungsrecht · {users.filter((u) => u.zweitfaktor).length} mit zweitem Faktor
          </span>
        </h3>
        <div className="listenkopf-werkzeug">
          <input
            className="listenfilter"
            placeholder="Filtern nach Name, Adresse, Anmeldename"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
          <button className="btn btn-primary" onClick={() => setAnlegen(true)}>
            Konten anlegen
          </button>
        </div>
      </div>
      <p className="muted small">
        Administratoren können jede Seite im Arbeitsbereich lesen und bearbeiten. Rollen
        gelten sofort; eine laufende Sitzung wird dafür nicht beendet.
      </p>

      {hinweis && <div className="hinweis-ok">{hinweis}</div>}

      <div className="tabelle-rollen">
        <table className="tabelle konten-tabelle">
          <thead>
            <tr>
              <th>Name</th>
              <th>E-Mail</th>
              <th>Anmeldename</th>
              <th>Rolle</th>
              <th>Zweiter Faktor</th>
              <th>Angelegt</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {sichtbar.map((u) => (
              <tr key={u.id}>
                <td>
                  {u.name}
                  {u.id === user?.id && <span className="muted"> (du)</span>}
                </td>
                <td className="muted">{u.email}</td>
                <td className="muted">
                  {u.benutzername || <span className="muted">nicht vergeben</span>}
                </td>
                <td className="muted">{u.role === "admin" ? "Admin" : "Nutzer"}</td>
                <td className={u.zweitfaktor ? undefined : "muted"}>
                  {u.zweitfaktor ? "steht" : "keiner"}
                </td>
                <td className="muted einzeilig">
                  {u.createdAt ? new Date(u.createdAt).toLocaleDateString("de-DE") : ""}
                </td>
                <td className="zeilen-aktionen">
                  {/* Ein Knopf statt vier. Was mit einem Konto geht, steht im
                      Fenster beieinander; in der Zeile war es eine Reihe, die
                      mit jeder neuen Möglichkeit länger wurde. */}
                  <button className="btn-schlicht" onClick={() => setBearbeitet(u)}>
                    Verwalten
                  </button>
                </td>
              </tr>
            ))}
            {sichtbar.length === 0 && (
              <tr>
                <td colSpan={7} className="muted">
                  {users.length === 0
                    ? "Noch kein Konto angelegt."
                    : "Kein Konto passt auf den Filter."}
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
        titel="Angelegt"
        unter={`${gut.length} von ${fertig.length} Konten`}
        breit
        schliessen={schliessen}
        fuss={
          <>
            <button className="btn" onClick={kopieren} disabled={gut.length === 0}>
              Liste kopieren
            </button>
            <button className="btn btn-primary" onClick={schliessen}>
              Fertig
            </button>
          </>
        }
      >
        {/* Die Passwörter stehen hier zum einzigen Mal. Wer das Fenster
            schließt, ohne sie mitzunehmen, muss sie zurücksetzen -- deshalb der
            Satz und nicht bloß die Tabelle. */}
        <p className="muted small">
          Die Passwörter stehen nur in diesem Fenster. Gib sie auf einem anderen Weg als
          per E-Mail weiter und lass danach jeden selbst eines wählen.
        </p>
        <table className="tabelle">
          <thead>
            <tr>
              <th>E-Mail</th>
              <th>Passwort</th>
              <th>Zustand</th>
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
      titel="Konten anlegen"
      unter={mehrere ? "Eine Zeile je Konto" : "Ein Konto"}
      breit={mehrere}
      schliessen={schliessen}
      fuss={
        <>
          <button className="btn" onClick={() => setMehrere((v) => !v)}>
            {mehrere ? "Nur eines anlegen" : "Mehrere auf einmal"}
          </button>
          <span className="fuss-luecke" />
          <button className="btn" onClick={schliessen}>
            Abbrechen
          </button>
          <button
            className="btn btn-primary"
            disabled={busy || (mehrere ? zeilen.length === 0 : !email.includes("@"))}
            onClick={anlegen}
          >
            {busy
              ? "Legt an…"
              : mehrere
                ? `${zeilen.length} ${zeilen.length === 1 ? "Konto" : "Konten"} anlegen`
                : "Anlegen"}
          </button>
        </>
      }
    >
      {mehrere ? (
        <>
          <p className="muted small">
            Je Zeile eine E-Mail-Adresse, danach durch Komma, Semikolon oder Tabulator
            getrennt Name, Anmeldename und Rolle. Alles außer der Adresse darf fehlen. Eine
            Spaltenauswahl aus einem Kalkulationsblatt lässt sich unverändert einfügen.
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
              ? "Noch keine brauchbare Zeile."
              : `${zeilen.length} ${zeilen.length === 1 ? "Zeile" : "Zeilen"} erkannt · ` +
                `${zeilen.filter((z) => z.rolle === "admin").length} davon als Admin · ` +
                "die Passwörter werden erzeugt und danach angezeigt"}
          </div>
        </>
      ) : (
        <div className="fenster-felder">
          <label>
            <span>E-Mail</span>
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
            <span>Anmeldename</span>
            <input
              placeholder="optional"
              value={benutzername}
              onChange={(e) => setBenutzername(e.target.value)}
            />
          </label>
          <label>
            <span>Rolle</span>
            <select value={rolle} onChange={(e) => setRolle(e.target.value)}>
              <option value="user">Nutzer</option>
              <option value="admin">Admin</option>
            </select>
          </label>
          <label className="feld-breit">
            <span>Passwort</span>
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
        `Passwort für ${konto.name} gesetzt` +
          (beendet > 0 ? `, ${beendet} ${beendet === 1 ? "Sitzung" : "Sitzungen"} beendet` : ""),
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
        titel: "Zweiten Faktor entfernen",
        text:
          `${konto.name} meldet sich danach wieder mit dem Passwort allein an, bis ein neuer ` +
          `eingerichtet ist. Nimm diesen Weg für ein verlorenes Telefon -- und vergewissere ` +
          `dich vorher auf einem anderen Kanal, dass die Bitte wirklich von dieser Person kommt.`,
        bestaetigen: "Entfernen",
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
        titel: "Nutzer löschen",
        text: `Das Konto ${konto.email} wird gelöscht, seine Seiten werden mit entfernt. Das lässt sich nicht rückgängig machen.`,
        bestaetigen: "Nutzer löschen",
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
            Löschen
          </button>
          <span className="fuss-luecke" />
          <button className="btn" onClick={schliessen}>
            Abbrechen
          </button>
          <button className="btn btn-primary" disabled={!geaendert || busy} onClick={speichern}>
            Speichern
          </button>
        </>
      }
    >
      {err && <div className="fehler">{err}</div>}
      <div className="fenster-felder">
        <label>
          <span>Rolle</span>
          <select value={rolle} disabled={selbst} onChange={(e) => setRolle(e.target.value)}>
            <option value="user">Nutzer</option>
            <option value="admin">Admin</option>
          </select>
        </label>
        <label>
          <span>Anmeldename</span>
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
        <div className="modal-label">Passwort zurücksetzen</div>
        <p className="muted small">
          Alle Sitzungen dieses Kontos werden dabei beendet. Sag das neue Passwort auf einem
          anderen Weg als per E-Mail und lass danach selbst eines wählen.
        </p>
        <div className="fenster-zeile">
          <input
            value={neu}
            disabled={selbst}
            placeholder="Neues Passwort, mindestens 6 Zeichen"
            onChange={(e) => setNeu(e.target.value)}
          />
          <button className="btn" disabled={selbst || busy || neu.length < 6} onClick={passwortSetzen}>
            Setzen
          </button>
          <button className="btn" disabled={selbst} onClick={() => setNeu(neuesPasswort())}>
            Erzeugen
          </button>
        </div>
      </div>

      <div className="fenster-abschnitt">
        <div className="modal-label">Zweiter Faktor</div>
        {konto.zweitfaktor ? (
          <div className="fenster-zeile">
            <span className="muted small">
              Steht. Das Konto wird bei jeder Anmeldung nach einem Code gefragt.
            </span>
            <button className="btn" onClick={zweitfaktorWeg}>
              Entfernen
            </button>
          </div>
        ) : (
          <p className="muted small">
            Keiner eingerichtet. Einrichten kann ihn nur das Konto selbst, unter Zugang; die
            Verwaltung kann ihn nur entfernen.
          </p>
        )}
      </div>
    </Fenster>
  );
}
