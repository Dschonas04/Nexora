// Who may do what in one space.
//
// The three levels are a ladder, not a set of switches: lesen < schreiben <
// verwalten. Whoever may verwalten hands out rights for this space, which is the
// space manager, without needing a global role between user and admin. "May
// manage Marketing" is a sentence an organisation can check; "is half an admin"
// is not.
//
// Layout of the mask: first who already has access, below it the granting.
// Whoever opens this box mostly wants to look first and only then change
// something, and adding presupposes knowing the existing list, otherwise one
// grants a right twice.
import { useCallback, useEffect, useMemo, useState } from "react";

import { Gruppe, SpaceRecht, User, api } from "../api/client";

type Stufe = "lesen" | "schreiben" | "verwalten";

const STUFEN: { wert: Stufe; titel: string; erklaerung: string }[] = [
  {
    wert: "lesen",
    titel: "Lesen",
    erklaerung: "Seiten ansehen, nichts ändern",
  },
  {
    wert: "schreiben",
    titel: "Schreiben",
    erklaerung: "Seiten ansehen und bearbeiten",
  },
  {
    wert: "verwalten",
    titel: "Verwalten",
    erklaerung: "zusätzlich Rechte an diesem Space vergeben",
  },
];

// A candidate is a group or an account in the same list. The distinction sits
// only in its kind, so that search and selection do not have to be written
// twice.
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
    api
      .gruppen()
      .then(setGruppen)
      .catch(() => setGruppen([]));
    // Accounts for granting individually. If the right for that is missing the
    // list stays empty and only groups can be entitled.
    api
      .listUsers()
      .then(setKonten)
      .catch(() => setKonten([]));
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

  // Do not offer targets that are already granted a second time; changing works
  // in the list above, and two ways for the same thing only confuse.
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
      ...konten.map<Kandidat>((k) => ({
        art: "u",
        id: k.id,
        name: k.name,
        zusatz: k.email,
      })),
    ].filter((k) => !vergeben.has(k.id));
    const s = suche.trim().toLowerCase();
    if (!s) return alle;
    return alle.filter(
      (k) =>
        k.name.toLowerCase().includes(s) || k.zusatz.toLowerCase().includes(s),
    );
  }, [gruppen, konten, vergeben, suche]);

  // A choice once made must not vanish silently when the search hides it or the
  // right has meanwhile been granted elsewhere.
  useEffect(() => {
    if (ziel && vergeben.has(ziel.id)) setZiel(null);
  }, [ziel, vergeben]);

  const gewaehlteStufe = STUFEN.find((x) => x.wert === stufe)!;

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
            {/* Stands there as a row and not as a footnote: otherwise an empty
                list reads as if nobody could reach the space. */}
            <div className="rechte-zeile rechte-zeile-fest">
              <Zeichen name="Eigentümer" gruppe />
              <div className="rechte-wer">
                <div className="rechte-name">
                  Eigentümer und Administratoren
                </div>
                <div className="rechte-meta">immer voller Zugriff</div>
              </div>
              <span className="rechte-fest-stufe">Verwalten</span>
            </div>

            {rechte.map((r) => (
              <div className="rechte-zeile" key={(r.gruppeId ?? r.userId)!}>
                <Zeichen
                  name={(r.gruppeId ? r.gruppeName : r.userName) ?? ""}
                  gruppe={!!r.gruppeId}
                />
                <div className="rechte-wer">
                  <div className="rechte-name">
                    {r.gruppeId ? r.gruppeName : r.userName}
                  </div>
                  <div className="rechte-meta">
                    {r.gruppeId ? "Gruppe" : "Konto"}
                  </div>
                </div>
                <select
                  value={r.recht}
                  onChange={(e) => aendern(r, e.target.value)}
                >
                  {STUFEN.map((x) => (
                    <option key={x.wert} value={x.wert}>
                      {x.titel}
                    </option>
                  ))}
                </select>
                {/* A symbol instead of the word "entziehen": the row already
                    carries a select, and two labels side by side read like two
                    offers of equal rank. */}
                <button
                  className="icon-btn"
                  title="Recht entziehen"
                  onClick={() => aendern(r, "")}
                >
                  ✕
                </button>
              </div>
            ))}
          </div>

          <h4 className="rechte-ueberschrift">Zugriff erteilen</h4>

          {/* Two steps, not three lists stacked on top of each other: first
              who, then how much. As long as nobody is chosen only the search
              stands here; afterwards the choice takes its place. */}
          {ziel ? (
            <div className="rechte-ziel">
              <Zeichen name={ziel.name} gruppe={ziel.art === "g"} />
              <div className="rechte-wer">
                <div className="rechte-name">{ziel.name}</div>
                <div className="rechte-meta">{ziel.zusatz}</div>
              </div>
              <button
                className="icon-btn"
                title="Auswahl aufheben"
                onClick={() => setZiel(null)}
              >
                ✕
              </button>
            </div>
          ) : (
            <>
              <input
                className="rechte-suche"
                value={suche}
                placeholder="Gruppe oder Konto suchen…"
                onChange={(e) => setSuche(e.target.value)}
              />

              <div className="rechte-auswahl">
                {kandidaten.length === 0 ? (
                  <div className="rechte-leer muted small">
                    {suche.trim()
                      ? "Nichts gefunden."
                      : "Alle bekannten Gruppen und Konten haben bereits ein Recht."}
                  </div>
                ) : (
                  kandidaten.map((k) => (
                    <button
                      type="button"
                      key={`${k.art}:${k.id}`}
                      className="rechte-kandidat"
                      onClick={() => setZiel(k)}
                    >
                      <Zeichen name={k.name} gruppe={k.art === "g"} />
                      <span className="rechte-wer">
                        <span className="rechte-name">{k.name}</span>
                        <span className="rechte-meta">{k.zusatz}</span>
                      </span>
                    </button>
                  ))
                )}
              </div>
            </>
          )}

          {/* The level as one piece with three fields: it is a ladder, and
              three cards side by side made it look like three separate
              switches. The explanation stands below for the chosen level only,
              instead of three times at once. */}
          <div className="rechte-fuss">
            <div className="rechte-leiter" role="group" aria-label="Stufe">
              {STUFEN.map((x) => (
                <button
                  type="button"
                  key={x.wert}
                  className={`rechte-sprosse${stufe === x.wert ? " gewaehlt" : ""}`}
                  onClick={() => setStufe(x.wert)}
                >
                  {x.titel}
                </button>
              ))}
            </div>
            <button
              className="btn btn-primary"
              disabled={!ziel}
              onClick={erteilen}
            >
              Erteilen
            </button>
          </div>
          <div className="rechte-erklaerung muted small">
            {gewaehlteStufe.erklaerung}
          </div>
        </div>
      </div>
    </div>
  );
}

// Zeichen is the mark in front of a name: a circle for an account, a rounded
// square for a group. The shape alone already says which of the two it is; the
// word beside it only has to confirm it.
function Zeichen({ name, gruppe }: { name: string; gruppe?: boolean }) {
  const buchstabe = (name.trim()[0] ?? "?").toUpperCase();
  return (
    <span className={"rechte-zeichen" + (gruppe ? " gruppe" : "")}>
      {buchstabe}
    </span>
  );
}
