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
        titel: "Gruppe löschen",
        text:
          `Die Gruppe „${g.name}“ wird gelöscht. Alle über sie vergebenen Rechte an Ablagen ` +
          `entfallen damit; die Konten selbst bleiben.`,
        bestaetigen: "Gruppe löschen",
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
        <h3>Gruppen</h3>
        <p className="muted small">Dieser Bereich ist Administratoren vorbehalten.</p>
      </>
    );
  }
  if (!frei("gruppen")) {
    return (
      <>
        <h3>Gruppen</h3>
        <p className="muted small">
          Diese Funktion gehört zum Zusatzumfang und ist in der vorliegenden Lizenz nicht
          enthalten.
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
        titel="Gruppen"
        zahl={`${gruppen.length} ${gruppen.length === 1 ? "Gruppe" : "Gruppen"}`}
        filter={filter}
        setFilter={setFilter}
        platzhalter="Filtern nach Name"
      >
        <button className="btn btn-primary" onClick={() => setAnlegenOffen(true)}>
          Gruppe anlegen
        </button>
      </Listenkopf>
      <p className="muted small">
        Eine Gruppe bündelt Konten. Zugriff bekommt sie nicht hier, sondern an der Ablage
        selbst, über das Schlüsselsymbol neben ihrem Namen in der Seitenleiste.
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
              <th>Beschreibung</th>
              <th>Mitglieder</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {gruppenSichtbar.map((g) => (
              <tr key={g.id}>
                <td>{g.name}</td>
                <td className="muted">
                  {g.beschreibung || <span className="muted">ohne Beschreibung</span>}
                </td>
                <td className="zahl">{g.mitglieder}</td>
                <td className="zeilen-aktionen">
                  <button className="btn-schlicht" onClick={() => oeffnen(g)}>
                    Mitglieder
                  </button>
                  <button className="btn-schlicht gefaehrlich" onClick={() => loeschen(g)}>
                    Löschen
                  </button>
                </td>
              </tr>
            ))}
            {gruppenSichtbar.length === 0 && (
              <tr>
                <td colSpan={4} className="muted">
                  {gruppen.length === 0
                    ? "Noch keine Gruppe angelegt."
                    : "Keine Gruppe passt auf den Filter."}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {anlegenOffen && (
        <Fenster
          titel="Neue Gruppe"
          unter="Rechte bekommt sie später, an der Ablage"
          schliessen={() => setAnlegenOffen(false)}
          fuss={
            <>
              <button className="btn" onClick={() => setAnlegenOffen(false)}>
                Abbrechen
              </button>
              <button className="btn btn-primary" disabled={!name.trim()} onClick={anlegen}>
                Anlegen
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
              <span>Beschreibung</span>
              <input
                value={beschreibung}
                placeholder="wofür sie steht"
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
                Fertig
              </button>
            </>
          }
        >
          <input
            className="listenfilter"
            placeholder="Konto suchen…"
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
            {sichtbar.length === 0 && <div className="muted small">Kein Treffer.</div>}
          </div>
        </Fenster>
      )}
    </div>
  );
}
