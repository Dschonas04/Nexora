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
  // The group whose members are currently in the dialog. Before, the list
  // opened under the row and pushed the table apart; with eight groups and
  // forty accounts nothing of the table was left to be seen afterwards.
  const [offen, setOffen] = useState<Gruppe | null>(null);
  const [mitglieder, setMitglieder] = useState<Mitglied[]>([]);
  const [anlegenOffen, setAnlegenOffen] = useState(false);
  const [name, setName] = useState("");
  const [beschreibung, setBeschreibung] = useState("");
  const [suche, setSuche] = useState("");
  // The filter above the group list itself, not above the members.
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

      {/* As a table, like the accounts beside it. Every group carries the same
          three figures, and one below the other in columns they can be
          compared; as blocks each began somewhere else. */}
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
        // The members in the dialog and not expanded under the row: the
        // list is as long as the workforce, and expanded it pushed everything
        // below it out of the picture. A tick takes effect at once -- there is
        // nothing to save here and therefore no button for it either.
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
