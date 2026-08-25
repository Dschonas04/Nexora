// Who may do what in one space.
//
// The three levels are a ladder, not a set of switches: lesen < schreiben <
// verwalten. Whoever may verwalten hands out rights for this space — that is
// the space manager, without needing a global role between user and admin.
// "May manage Marketing" is a sentence an organisation can check; "is half an
// admin" is not.
//
// Aufbau der Maske: erst wer schon Zugriff hat, darunter das Erteilen. Wer
// diesen Kasten öffnet, will meistens zuerst nachsehen und erst dann etwas
// ändern, und das Hinzufügen setzt voraus, dass man die bestehende Liste
// kennt, sonst vergibt man ein Recht zweimal.
import { useCallback, useEffect, useMemo, useState } from "react";

import { Gruppe, SpaceRecht, User, api } from "../api/client";

type Stufe = "lesen" | "schreiben" | "verwalten";

const STUFEN: { wert: Stufe; titel: string; erklaerung: string }[] = [
  { wert: "lesen", titel: "Lesen", erklaerung: "Seiten ansehen, nichts ändern" },
  { wert: "schreiben", titel: "Schreiben", erklaerung: "Seiten ansehen und bearbeiten" },
  { wert: "verwalten", titel: "Verwalten", erklaerung: "zusätzlich Rechte an diesem Space vergeben" },
];

// Ein Kandidat ist eine Gruppe oder ein Konto in derselben Liste. Die
// Unterscheidung steckt nur noch in der Art, damit Suche und Auswahl nicht
// zweimal geschrieben werden müssen.
type Kandidat = { art: "g" | "u"; id: string; name: string; zusatz: string };

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
  const [suche, setSuche] = useState("");
  const [ziel, setZiel] = useState<Kandidat | null>(null);
  const [stufe, setStufe] = useState<Stufe>("lesen");

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
    try {
      await api.spaceRechtSetzen(
        spaceId,
        ziel.art === "g" ? { gruppeId: ziel.id } : { userId: ziel.id },
        stufe,
      );
      setZiel(null);
      setSuche("");
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

  // Schon vergebene Ziele nicht noch einmal anbieten, ändern geht in der
  // Liste darüber, und zwei Wege für dieselbe Sache verwirren nur.
  const vergeben = useMemo(
    () => new Set(rechte.map((r) => r.gruppeId ?? r.userId)),
    [rechte],
  );

  const kandidaten = useMemo<Kandidat[]>(() => {
    const alle: Kandidat[] = [
      ...gruppen.map<Kandidat>((g) => ({
        art: "g",
        id: g.id,
        name: g.name,
        zusatz: `Gruppe, ${g.mitglieder} ${g.mitglieder === 1 ? "Mitglied" : "Mitglieder"}`,
      })),
      ...konten.map<Kandidat>((k) => ({ art: "u", id: k.id, name: k.name, zusatz: k.email })),
    ].filter((k) => !vergeben.has(k.id));
    const s = suche.trim().toLowerCase();
    if (!s) return alle;
    return alle.filter(
      (k) => k.name.toLowerCase().includes(s) || k.zusatz.toLowerCase().includes(s),
    );
  }, [gruppen, konten, vergeben, suche]);

  // Eine getroffene Wahl darf nicht stumm verschwinden, wenn die Suche sie
  // ausblendet oder das Recht inzwischen anderswo vergeben wurde.
  useEffect(() => {
    if (ziel && vergeben.has(ziel.id)) setZiel(null);
  }, [ziel, vergeben]);

  return (
    <div className="qv-overlay" onClick={onClose}>
      <div className="qv-box rechte-box" onClick={(e) => e.stopPropagation()}>
        <div className="qv-head">
          <span className="qv-title">Zugriff auf „{spaceName}“</span>
          <div className="qv-actions">
            <button className="btn" onClick={onClose}>
              Schließen
            </button>
          </div>
        </div>

        <div className="rechte-inhalt">
          {fehler && <div className="fehler">{fehler}</div>}

          <h4 className="rechte-ueberschrift">Wer Zugriff hat</h4>

          <div className="rechte-liste">
            {/* Steht als Zeile und nicht als Fußnote da: sonst liest sich eine
                leere Liste so, als käme niemand an den Space. */}
            <div className="rechte-zeile rechte-zeile-fest">
              <div className="rechte-wer">
                <div className="rechte-name">Eigentümer und Administratoren</div>
                <div className="muted small">immer voller Zugriff</div>
              </div>
              <div className="muted small">Verwalten</div>
            </div>

            {rechte.map((r) => (
              <div className="rechte-zeile" key={(r.gruppeId ?? r.userId)!}>
                <div className="rechte-wer">
                  <div className="rechte-name">{r.gruppeId ? r.gruppeName : r.userName}</div>
                  <div className="muted small">{r.gruppeId ? "Gruppe" : "Konto"}</div>
                </div>
                <select value={r.recht} onChange={(e) => aendern(r, e.target.value)}>
                  {STUFEN.map((s) => (
                    <option key={s.wert} value={s.wert}>
                      {s.titel}
                    </option>
                  ))}
                </select>
                <button className="link-btn" onClick={() => aendern(r, "")}>
                  entziehen
                </button>
              </div>
            ))}
          </div>

          <h4 className="rechte-ueberschrift">Zugriff erteilen</h4>

          <input
            className="rechte-suche"
            value={suche}
            placeholder="Gruppe oder Konto suchen…"
            onChange={(e) => setSuche(e.target.value)}
          />

          <div className="rechte-auswahl">
            {kandidaten.length === 0 ? (
              <div className="muted small rechte-leer">
                {suche.trim()
                  ? "Nichts gefunden."
                  : "Alle bekannten Gruppen und Konten haben bereits ein Recht."}
              </div>
            ) : (
              kandidaten.map((k) => (
                <button
                  type="button"
                  key={`${k.art}:${k.id}`}
                  className={`rechte-kandidat${ziel && ziel.art === k.art && ziel.id === k.id ? " gewaehlt" : ""}`}
                  onClick={() => setZiel(k)}
                >
                  <span className="rechte-name">{k.name}</span>
                  <span className="muted small">{k.zusatz}</span>
                </button>
              ))
            )}
          </div>

          <div className="rechte-stufen">
            {STUFEN.map((s) => (
              <button
                type="button"
                key={s.wert}
                className={`rechte-stufe${stufe === s.wert ? " gewaehlt" : ""}`}
                onClick={() => setStufe(s.wert)}
              >
                <span className="rechte-name">{s.titel}</span>
                <span className="muted small">{s.erklaerung}</span>
              </button>
            ))}
          </div>

          <div className="rechte-abschluss">
            <button className="btn btn-primary" disabled={!ziel} onClick={erteilen}>
              {ziel
                ? `„${ziel.name}“ darf ${STUFEN.find((s) => s.wert === stufe)!.titel.toLowerCase()}`
                : "Erteilen"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
