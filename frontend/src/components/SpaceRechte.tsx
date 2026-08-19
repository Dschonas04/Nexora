// Who may do what in one space.
//
// The three levels are a ladder, not a set of switches: lesen < schreiben <
// verwalten. Whoever may verwalten hands out rights for this space — that is
// the space manager, without needing a global role between user and admin.
// "May manage Marketing" is a sentence an organisation can check; "is half an
// admin" is not.
import { useCallback, useEffect, useState } from "react";

import { Gruppe, SpaceRecht, User, api } from "../api/client";

const STUFEN: { wert: "lesen" | "schreiben" | "verwalten"; titel: string; erklaerung: string }[] = [
  { wert: "lesen", titel: "Lesen", erklaerung: "Seiten ansehen, nichts ändern" },
  { wert: "schreiben", titel: "Schreiben", erklaerung: "Seiten ansehen und bearbeiten" },
  { wert: "verwalten", titel: "Verwalten", erklaerung: "zusätzlich Rechte an diesem Space vergeben" },
];

export default function SpaceRechte({
  spaceId,
  spaceName,
  onClose,
}: {
  spaceId: string;
  spaceName: string;
  onClose: () => void;
}) {
  const [rechte, setRechte] = useState<SpaceRecht[]>([]);
  const [gruppen, setGruppen] = useState<Gruppe[]>([]);
  const [konten, setKonten] = useState<User[]>([]);
  const [fehler, setFehler] = useState<string | null>(null);
  const [ziel, setZiel] = useState("");
  const [stufe, setStufe] = useState<"lesen" | "schreiben" | "verwalten">("lesen");

  const laden = useCallback(() => {
    api
      .spaceRechte(spaceId)
      .then((r) => {
        setRechte(r);
        setFehler(null);
      })
      .catch((e: Error) => setFehler(e.message));
  }, [spaceId]);

  useEffect(() => {
    laden();
    api.gruppen().then(setGruppen).catch(() => setGruppen([]));
    // Konten für die Einzelvergabe. Fehlt das Recht dazu, bleibt die Liste
    // leer und es lassen sich eben nur Gruppen berechtigen.
    api.listUsers().then(setKonten).catch(() => setKonten([]));
  }, [laden]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const erteilen = async () => {
    if (!ziel) return;
    const [art, id] = ziel.split(":");
    try {
      await api.spaceRechtSetzen(
        spaceId,
        art === "g" ? { gruppeId: id } : { userId: id },
        stufe,
      );
      setZiel("");
      laden();
    } catch (e) {
      setFehler((e as Error).message);
    }
  };

  const aendern = async (r: SpaceRecht, neu: string) => {
    await api
      .spaceRechtSetzen(
        spaceId,
        r.gruppeId ? { gruppeId: r.gruppeId } : { userId: r.userId! },
        neu,
      )
      .catch((e: Error) => setFehler(e.message));
    laden();
  };

  // Schon vergebene Ziele nicht noch einmal anbieten -- ändern geht in der
  // Liste darunter, und zwei Wege für dieselbe Sache verwirren nur.
  const vergeben = new Set(rechte.map((r) => r.gruppeId ?? r.userId));

  return (
    <div className="qv-overlay" onClick={onClose}>
      <div className="qv-box rechte-box" onClick={(e) => e.stopPropagation()}>
        <div className="qv-head">
          <span className="qv-title">Rechte an „{spaceName}“</span>
          <div className="qv-actions">
            <button className="btn" onClick={onClose}>
              Schließen
            </button>
          </div>
        </div>

        <div className="rechte-inhalt">
          {fehler && <div className="fehler">{fehler}</div>}

          <p className="muted small">
            Der Eigentümer des Space und Administratoren haben ohnehin vollen Zugriff; sie
            stehen deshalb nicht in dieser Liste.
          </p>

          <div className="rechte-neu">
            <select value={ziel} onChange={(e) => setZiel(e.target.value)}>
              <option value="">Gruppe oder Konto wählen…</option>
              {gruppen.filter((g) => !vergeben.has(g.id)).length > 0 && (
                <optgroup label="Gruppen">
                  {gruppen
                    .filter((g) => !vergeben.has(g.id))
                    .map((g) => (
                      <option key={g.id} value={`g:${g.id}`}>
                        {g.name} ({g.mitglieder})
                      </option>
                    ))}
                </optgroup>
              )}
              {konten.filter((k) => !vergeben.has(k.id)).length > 0 && (
                <optgroup label="Einzelne Konten">
                  {konten
                    .filter((k) => !vergeben.has(k.id))
                    .map((k) => (
                      <option key={k.id} value={`u:${k.id}`}>
                        {k.name} — {k.email}
                      </option>
                    ))}
                </optgroup>
              )}
            </select>
            <select value={stufe} onChange={(e) => setStufe(e.target.value as typeof stufe)}>
              {STUFEN.map((s) => (
                <option key={s.wert} value={s.wert}>
                  {s.titel}
                </option>
              ))}
            </select>
            <button className="btn" disabled={!ziel} onClick={erteilen}>
              Erteilen
            </button>
          </div>
          <div className="muted small">
            {STUFEN.find((s) => s.wert === stufe)?.erklaerung}
          </div>

          {rechte.length === 0 ? (
            <p className="muted" style={{ marginTop: 16 }}>
              Noch keine Rechte vergeben. Der Space ist damit nur für seinen Eigentümer und
              für Administratoren sichtbar.
            </p>
          ) : (
            <table className="tabelle" style={{ marginTop: 16 }}>
              <thead>
                <tr>
                  <th>Wer</th>
                  <th>Recht</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {rechte.map((r) => (
                  <tr key={(r.gruppeId ?? r.userId)!}>
                    <td>
                      {r.gruppeId ? r.gruppeName : r.userName}
                      <div className="muted small">{r.gruppeId ? "Gruppe" : "Konto"}</div>
                    </td>
                    <td>
                      <select value={r.recht} onChange={(e) => aendern(r, e.target.value)}>
                        {STUFEN.map((s) => (
                          <option key={s.wert} value={s.wert}>
                            {s.titel}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td>
                      <button className="link-btn" onClick={() => aendern(r, "")}>
                        entziehen
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
}
