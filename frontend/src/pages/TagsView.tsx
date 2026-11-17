// All tags of one's account on one page: create, rename, recolour, delete,
// and how many pages carry each. A plain list, one row per tag.
import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router";

import { Tag, api } from "../api/client";
import { useRueckfrage } from "../components/Rueckfrage";
import Symbol from "../components/Symbole";
import { useAussenklick } from "../klappen";

// The same tones the page menu picks from, plus a few more so that ten tags do
// not have to share six colours.
const FARBEN = [
  "#e0507a",
  "#e5484d",
  "#e08a2b",
  "#d4a72c",
  "#3aa675",
  "#14a3a3",
  "#2383e2",
  "#5b6ee1",
  "#8b5cf6",
  "#b4579b",
  "#8a6a4f",
  "#6b7280",
];

function Farbwahl({
  wert,
  waehlen,
  schliessen,
}: {
  wert: string;
  waehlen: (farbe: string) => void;
  schliessen: () => void;
}) {
  const bereich = useAussenklick<HTMLDivElement>(true, schliessen);
  return (
    <div className="farbtupfer-wahl" ref={bereich} role="listbox" aria-label="Colour">
      {FARBEN.map((f) => (
        <button
          key={f}
          type="button"
          className={"farbtupfer" + (f.toLowerCase() === wert.toLowerCase() ? " gewaehlt" : "")}
          style={{ background: f }}
          title={f}
          aria-label={f}
          aria-selected={f.toLowerCase() === wert.toLowerCase()}
          onClick={() => {
            waehlen(f);
            schliessen();
          }}
        />
      ))}
    </div>
  );
}

export default function TagsView({
  allTags,
  onTagsChange,
}: {
  allTags: Tag[];
  onTagsChange: () => void;
}) {
  const nav = useNavigate();
  const frageStellen = useRueckfrage();
  const [suche, setSuche] = useState("");
  const [sortierung, setSortierung] = useState<"name" | "anzahl">("name");
  const [neuOffen, setNeuOffen] = useState(false);
  const [neuName, setNeuName] = useState("");
  const [neuFarbe, setNeuFarbe] = useState(FARBEN[6]);
  const [farbeFuer, setFarbeFuer] = useState<string | null>(null);
  const [bearbeite, setBearbeite] = useState<string | null>(null);
  const [entwurf, setEntwurf] = useState("");
  const [fehler, setFehler] = useState<string | null>(null);
  const neuFeld = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (neuOffen) neuFeld.current?.focus();
  }, [neuOffen]);

  const liste = useMemo(() => {
    const q = suche.trim().toLowerCase();
    const l = allTags.filter((t) => !q || t.name.toLowerCase().includes(q));
    return sortierung === "name"
      ? [...l].sort((a, b) => a.name.localeCompare(b.name))
      : [...l].sort((a, b) => (b.count ?? 0) - (a.count ?? 0) || a.name.localeCompare(b.name));
  }, [allTags, suche, sortierung]);

  const gesamt = allTags.reduce((s, t) => s + (t.count ?? 0), 0);

  const anlegen = async (e: React.FormEvent) => {
    e.preventDefault();
    const name = neuName.trim();
    if (!name) return;
    if (allTags.some((t) => t.name.toLowerCase() === name.toLowerCase())) {
      setFehler("A tag with this name already exists.");
      return;
    }
    try {
      await api.createTag(name, neuFarbe);
      setNeuName("");
      setNeuOffen(false);
      setFehler(null);
      onTagsChange();
    } catch (err) {
      setFehler((err as Error).message);
    }
  };

  const aendern = async (t: Tag, felder: { name?: string; color?: string }) => {
    try {
      await api.updateTag(t.id, felder);
      setFehler(null);
      onTagsChange();
    } catch (err) {
      setFehler((err as Error).message);
    }
  };

  const umbenennen = async (t: Tag) => {
    const name = entwurf.trim();
    setBearbeite(null);
    if (!name || name === t.name) return;
    await aendern(t, { name });
  };

  const loeschen = async (t: Tag) => {
    const n = t.count ?? 0;
    const text =
      n === 0
        ? `Delete the tag “${t.name}”?`
        : `Delete the tag “${t.name}”? It is removed from ${n} pages. The pages themselves remain.`;
    if (!(await frageStellen({ titel: "Delete tag", text, bestaetigen: "Delete tag", gefaehrlich: true })))
      return;
    await api.deleteTag(t.id).catch(() => {});
    onTagsChange();
  };

  return (
    <div className="page-pad schlagwort-seite">
      <header className="schlagwort-kopf">
        <div>
          <h2>Tags</h2>
          <p className="muted small">
            {allTags.length === 0 ? "" : allTags.length === 1 ? "One tag" : `${allTags.length} tags`}
            {gesamt > 0 && <> · {gesamt === 1 ? "one assignment" : `${gesamt} assignments`}</>}
          </p>
        </div>
        <button className="btn btn-primary schlagwort-neu-knopf" onClick={() => setNeuOffen((v) => !v)}>
          <Symbol name="plus" />
          New tag
        </button>
      </header>

      {neuOffen && (
        <form className="schlagwort-anlegen" onSubmit={anlegen}>
          <div className="farbe-anker">
            <button
              type="button"
              className="farbtupfer gross"
              style={{ background: neuFarbe }}
              title="Colour"
              aria-label="Colour"
              onClick={() => setFarbeFuer(farbeFuer === "neu" ? null : "neu")}
            />
            {farbeFuer === "neu" && (
              <Farbwahl wert={neuFarbe} waehlen={setNeuFarbe} schliessen={() => setFarbeFuer(null)} />
            )}
          </div>
          <input
            ref={neuFeld}
            placeholder="Name of the new tag…"
            value={neuName}
            onChange={(e) => setNeuName(e.target.value)}
            onKeyDown={(e) => e.key === "Escape" && setNeuOffen(false)}
            aria-label="New tag"
          />
          <button className="btn btn-primary" type="submit" disabled={!neuName.trim()}>
            Create
          </button>
          <button className="btn" type="button" onClick={() => setNeuOffen(false)}>
            Cancel
          </button>
        </form>
      )}

      {allTags.length > 0 && (
        <div className="schlagwort-werkzeuge">
          <div className="such-feld">
            <input
              placeholder="Search tags…"
              value={suche}
              onChange={(e) => setSuche(e.target.value)}
              aria-label="Search tags"
            />
          </div>
          <div className="umschalter" role="group" aria-label="Sort">
            <button
              className={sortierung === "name" ? "aktiv" : ""}
              onClick={() => setSortierung("name")}
            >
              Name
            </button>
            <button
              className={sortierung === "anzahl" ? "aktiv" : ""}
              onClick={() => setSortierung("anzahl")}
            >
              Most used
            </button>
          </div>
        </div>
      )}

      {fehler && <div className="fehler">{fehler}</div>}

      {allTags.length === 0 ? (
        <div className="schlagwort-leer">
          <p>No tags yet.</p>
          <p className="muted small">
            Create one with &ldquo;New tag&rdquo;, or add one to a page from its menu under &ldquo;Tags&hellip;&rdquo;.
          </p>
        </div>
      ) : liste.length === 0 ? (
        <p className="muted">No tag matches the search.</p>
      ) : (
        <ul className="schlagwort-zeilen">
          {liste.map((t) => (
            <li key={t.id} className={"schlagwort-zeile" + ((t.count ?? 0) === 0 ? " verwaist" : "")}>
              <div className="farbe-anker">
                <button
                  className="farbtupfer"
                  style={{ background: t.color }}
                  title="Change colour"
                  aria-label="Change colour"
                  onClick={() => setFarbeFuer(farbeFuer === t.id ? null : t.id)}
                />
                {farbeFuer === t.id && (
                  <Farbwahl
                    wert={t.color}
                    waehlen={(f) => void aendern(t, { color: f })}
                    schliessen={() => setFarbeFuer(null)}
                  />
                )}
              </div>
              {bearbeite === t.id ? (
                <input
                  autoFocus
                  className="schlagwort-umbenennen"
                  value={entwurf}
                  onChange={(e) => setEntwurf(e.target.value)}
                  onBlur={() => void umbenennen(t)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") void umbenennen(t);
                    if (e.key === "Escape") setBearbeite(null);
                  }}
                  aria-label="Rename tag"
                />
              ) : (
                <button
                  className="schlagwort-name"
                  onClick={() => nav(`/tag/${t.id}`)}
                  title="Show the pages with this tag"
                >
                  {t.name}
                </button>
              )}
              <span className="schlagwort-zahl">
                {(t.count ?? 0) === 0 ? "Not on any page" : t.count === 1 ? "One page" : `${t.count} pages`}
              </span>
              <span className="schlagwort-aktionen">
                <button
                  className="icon-btn"
                  title="Rename"
                  aria-label="Rename"
                  onClick={() => {
                    setEntwurf(t.name);
                    setBearbeite(t.id);
                  }}
                >
                  <Symbol name="stift" />
                </button>
                <button className="icon-btn" title="Delete" aria-label="Delete" onClick={() => void loeschen(t)}>
                  <Symbol name="loeschen" />
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
