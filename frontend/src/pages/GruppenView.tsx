// Group administration.
//
// Groups exist because per-page sharing does not scale: letting fourteen
// colleagues into an area means fourteen clicks per page. This page is where a
// group is defined; where it is granted access is the space, not here.
import { useCallback, useEffect, useState } from "react";

import { Gruppe, Mitglied, api } from "../api/client";
import { useAuth } from "../auth";
import { useLizenz } from "../lizenz";
import Fenster from "../components/Fenster";
import KurzeZeilen from "../components/Kurzliste";
import Listenkopf from "../components/Listenkopf";
import { useRueckfrage } from "../components/Rueckfrage";

export default function GruppenView() {
  const frage = useRueckfrage();
  const { user } = useAuth();
  const { frei, geladen } = useLizenz();

  const [gruppen, setGruppen] = useState<Gruppe[]>([]);
  // Die Gruppe, deren Mitglieder gerade im Fenster stehen. Vorher klappte die
  // Liste unter der Zeile auf und schob die Tabelle auseinander; bei acht
  // Gruppen und vierzig Konten war von der Tabelle danach nichts mehr zu sehen.
  const [offen, setOffen] = useState<Gruppe | null>(null);
  const [mitglieder, setMitglieder] = useState<Mitglied[]>([]);
  const [anlegenOffen, setAnlegenOffen] = useState(false);
  const [name, setName] = useState("");
  const [beschreibung, setBeschreibung] = useState("");
  const [suche, setSuche] = useState("");
  // Der Filter über der Gruppenliste selbst, nicht über den Mitgliedern.
  const [filter, setFilter] = useState("");
  const [meldung, setMeldung] = useState<{ text: string; art: "ok" | "fehler" } | null>(null);

  const laden = useCallback(() => {
    api.gruppen().then(setGruppen).catch(() => setGruppen([]));
  }, []);
  useEffect(laden, [laden]);

  const oeffnen = async (g: Gruppe) => {
    setOffen(g);
    setSuche("");
    setMitglieder([]);
    await api
      .gruppenMitglieder(g.id)
      .then(setMitglieder)
      .catch(() => setMitglieder([]));
  };

  const anlegen = async () => {
    const n = name.trim();
    if (!n) return;
    try {
      await api.gruppeAnlegen(n, beschreibung.trim());
      setName("");
      setBeschreibung("");
      setAnlegenOffen(false);
      setMeldung({ text: `Gruppe „${n}“ angelegt.`, art: "ok" });
      laden();
    } catch (e) {
      setMeldung({ text: (e as Error).message, art: "fehler" });
    }
  };

  const loeschen = async (g: Gruppe) => {
    // A confirmation, because with the group all rights granted through it fall
    // too, which may hit people who are working right now.
    if (
      !(await frage({
        titel: "Delete group",
        text:
          `The group \u201c${g.name}\u201d is deleted. Every permission on spaces granted ` +
          `through it goes with it; the accounts themselves remain.`,
        bestaetigen: "Delete group",
        gefaehrlich: true,
      }))
    )
      return;
    await api.gruppeLoeschen(g.id).catch(() => {});
    if (offen?.id === g.id) setOffen(null);
    laden();
  };

  const umschalten = async (m: Mitglied) => {
    if (!offen) return;
    await api.mitgliedSetzen(offen.id, m.id, !m.drin).catch(() => {});
    setMitglieder((v) => v.map((x) => (x.id === m.id ? { ...x, drin: !x.drin } : x)));
    laden();
  };

  if (!geladen) return null;
  if (user?.role !== "admin") {
    return (
      <>
        <h3>Groups</h3>
        <p className="muted small">This area is for administrators only.</p>
      </>
    );
  }
  if (!frei("gruppen")) {
    return (
      <>
        <h3>Groups</h3>
        <p className="muted small">
          This feature belongs to the paid scope and is not included in the licence
          currently installed.
        </p>
      </>
    );
  }

  const gruppenBegriff = filter.trim().toLowerCase();
  const gruppenSichtbar = gruppenBegriff
    ? gruppen.filter((g) =>
        (g.name + " " + (g.beschreibung ?? "")).toLowerCase().includes(gruppenBegriff),
      )
    : gruppen;

  const begriff = suche.trim().toLowerCase();
  const sichtbar = begriff
    ? mitglieder.filter((m) => (m.name + " " + m.email).toLowerCase().includes(begriff))
    : mitglieder;

  return (
    <div className="gruppenliste">
      <Listenkopf
        titel="Groups"
        zahl={`${gruppen.length} ${gruppen.length === 1 ? "group" : "groups"}`}
        filter={filter}
        setFilter={setFilter}
        platzhalter="Filter by name"
      >
        <button className="btn btn-primary" onClick={() => setAnlegenOffen(true)}>
          Create group
        </button>
      </Listenkopf>
      <p className="muted small">
        A group bundles accounts. It gets access not here but on the space itself, through
        the key icon next to its name in the sidebar.
      </p>

      {meldung && (
        <div className={meldung.art === "ok" ? "hinweis-ok" : "fehler"}>{meldung.text}</div>
      )}

      {/* Als Tabelle, wie die Konten daneben. Jede Gruppe traegt dieselben drei
          Angaben, und untereinander in Spalten sind sie zu vergleichen; als
          Bloecke fing jede woanders an. */}
      <div className="tabelle-rollen">
        <table className="tabelle gruppen-tabelle">
          <thead>
            <tr>
              <th>Name</th>
              <th>Description</th>
              <th>Members</th>
              <th />
            </tr>
          </thead>
          <tbody>
            <KurzeZeilen
              alle={gruppenSichtbar}
              spalten={4}
              zeile={(g) => (
              <tr key={g.id}>
                <td>{g.name}</td>
                <td className="muted">
                  {g.beschreibung || <span className="muted">no description</span>}
                </td>
                <td className="zahl">{g.mitglieder}</td>
                <td className="zeilen-aktionen">
                  <button className="btn-schlicht" onClick={() => oeffnen(g)}>
                    Members
                  </button>
                  <button className="btn-schlicht gefaehrlich" onClick={() => loeschen(g)}>
                    Delete
                  </button>
                </td>
              </tr>
              )}
            />
            {gruppenSichtbar.length === 0 && (
              <tr>
                <td colSpan={4} className="muted">
                  {gruppen.length === 0
                    ? "No group created yet."
                    : "No group matches the filter."}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {anlegenOffen && (
        <Fenster
          titel="New group"
          unter="It gets permissions later, on the space"
          schliessen={() => setAnlegenOffen(false)}
          fuss={
            <>
              <button className="btn" onClick={() => setAnlegenOffen(false)}>
                Cancel
              </button>
              <button className="btn btn-primary" disabled={!name.trim()} onClick={anlegen}>
                Create
              </button>
            </>
          }
        >
          <div className="fenster-felder">
            <label>
              <span>Name</span>
              <input
                value={name}
                placeholder="Vertrieb"
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && anlegen()}
              />
            </label>
            <label>
              <span>Description</span>
              <input
                value={beschreibung}
                placeholder="what it stands for"
                onChange={(e) => setBeschreibung(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && anlegen()}
              />
            </label>
          </div>
        </Fenster>
      )}

      {offen && (
        // Die Mitglieder im Fenster und nicht aufgeklappt unter der Zeile: die
        // Liste ist so lang wie die Belegschaft, und aufgeklappt schob sie
        // alles darunter aus dem Bild. Ein Haken wirkt sofort -- es gibt hier
        // nichts zu speichern und deshalb auch keinen Knopf dafür.
        <Fenster
          titel={offen.name}
          unter={`${mitglieder.filter((m) => m.drin).length} von ${mitglieder.length} Konten in der Gruppe`}
          schliessen={() => setOffen(null)}
          fuss={
            <>
              <span className="fuss-luecke" />
              <button className="btn btn-primary" onClick={() => setOffen(null)}>
                Done
              </button>
            </>
          }
        >
          <input
            className="listenfilter"
            placeholder="Find an account…"
            value={suche}
            onChange={(e) => setSuche(e.target.value)}
          />
          <div className="mitgliederliste">
            {sichtbar.map((m) => (
              <label key={m.id} className="mitglied">
                <input type="checkbox" checked={m.drin} onChange={() => umschalten(m)} />
                <span className="mitglied-name">{m.name}</span>
                <span className="muted small">{m.email}</span>
                {m.rolle === "admin" && <span className="muted small">Administrator</span>}
              </label>
            ))}
            {sichtbar.length === 0 && <div className="muted small">No match.</div>}
          </div>
        </Fenster>
      )}
    </div>
  );
}
