// Group administration.
//
// Groups exist because per-page sharing does not scale: letting fourteen
// colleagues into an area means fourteen clicks per page. This page is where a
// group is defined; where it is granted access is the space, not here.
import { useCallback, useEffect, useState } from "react";

import { Gruppe, Mitglied, api } from "../api/client";
import { useAuth } from "../auth";
import { useLizenz } from "../lizenz";
import { useRueckfrage } from "../components/Rueckfrage";

export default function GruppenView() {
  const frage = useRueckfrage();
  const { user } = useAuth();
  const { frei, geladen } = useLizenz();

  const [gruppen, setGruppen] = useState<Gruppe[]>([]);
  const [offen, setOffen] = useState<string | null>(null);
  const [mitglieder, setMitglieder] = useState<Mitglied[]>([]);
  const [name, setName] = useState("");
  const [beschreibung, setBeschreibung] = useState("");
  const [suche, setSuche] = useState("");
  const [meldung, setMeldung] = useState<{ text: string; art: "ok" | "fehler" } | null>(null);

  const laden = useCallback(() => {
    api.gruppen().then(setGruppen).catch(() => setGruppen([]));
  }, []);
  useEffect(laden, [laden]);

  const oeffnen = async (id: string) => {
    if (offen === id) {
      setOffen(null);
      return;
    }
    setOffen(id);
    setSuche("");
    await api
      .gruppenMitglieder(id)
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
    if (offen === g.id) setOffen(null);
    laden();
  };

  const umschalten = async (m: Mitglied) => {
    if (!offen) return;
    await api.mitgliedSetzen(offen, m.id, !m.drin).catch(() => {});
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

  const begriff = suche.trim().toLowerCase();
  const sichtbar = begriff
    ? mitglieder.filter((m) => (m.name + " " + m.email).toLowerCase().includes(begriff))
    : mitglieder;

  return (
    <div className="gruppenliste">
      <h3>Gruppen</h3>
      <p className="muted small">
        Eine Gruppe bündelt Konten. Zugriff bekommt sie nicht hier, sondern an einer Ablage —
        über das Schlüsselsymbol neben seinem Namen in der Seitenleiste.
      </p>

      {meldung && (
        <div className={meldung.art === "ok" ? "hinweis-ok" : "fehler"}>{meldung.text}</div>
      )}

      <h3>Neue Gruppe</h3>
      <div className="einstellung">
        <div className="s3-felder">
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
        <div className="einstellung-aktionen">
          <button className="btn" disabled={!name.trim()} onClick={anlegen}>
            Anlegen
          </button>
        </div>
      </div>

      <h3>Vorhandene Gruppen</h3>
      {gruppen.length === 0 && <p className="muted">Noch keine Gruppe angelegt.</p>}

      {gruppen.map((g) => (
        <div className="einstellung" key={g.id}>
          <div className="einstellung-kopf">
            <div>
              <div className="einstellung-titel">{g.name}</div>
              <div className="einstellung-erklaerung">
                {g.beschreibung || <span className="muted">ohne Beschreibung</span>}
              </div>
              <div className="muted small" style={{ marginTop: 4 }}>
                {g.mitglieder === 1 ? "1 Mitglied" : `${g.mitglieder} Mitglieder`}
              </div>
            </div>
            <div className="einstellung-feld">
              <button className="btn" onClick={() => oeffnen(g.id)}>
                {offen === g.id ? "Schließen" : "Mitglieder"}
              </button>
              <button className="btn" onClick={() => loeschen(g)}>
                Löschen
              </button>
            </div>
          </div>

          {offen === g.id && (
            <div className="mitgliederliste">
              <input
                placeholder="Konto suchen…"
                value={suche}
                onChange={(e) => setSuche(e.target.value)}
              />
              {sichtbar.map((m) => (
                <label key={m.id} className="mitglied">
                  <input type="checkbox" checked={m.drin} onChange={() => umschalten(m)} />
                  <span className="mitglied-name">{m.name}</span>
                  <span className="muted small">{m.email}</span>
                  {m.rolle === "admin" && <span className="pill klein">Admin</span>}
                </label>
              ))}
              {sichtbar.length === 0 && <div className="muted small">Kein Treffer.</div>}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
