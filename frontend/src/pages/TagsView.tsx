// All tags of one's account on one page: create, rename, recolour, delete,
// and see how many pages carry each. The sidebar only leads here; managing
// them in a list of dots there was too cramped.
import { useMemo, useState } from "react";
import { useNavigate } from "react-router";

import { Tag, api } from "../api/client";
import { useRueckfrage } from "../components/Rueckfrage";
import Symbol from "../components/Symbole";

const FARBEN = ["#e0507a", "#e08a2b", "#3aa675", "#2383e2", "#8b5cf6", "#6b7280"];

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
  const [neuName, setNeuName] = useState("");
  const [neuFarbe, setNeuFarbe] = useState(FARBEN[3]);
  const [bearbeite, setBearbeite] = useState<string | null>(null);
  const [entwurf, setEntwurf] = useState("");
  const [fehler, setFehler] = useState<string | null>(null);
  const [sortierung, setSortierung] = useState<"name" | "anzahl">("name");

  const liste = useMemo(() => {
    const q = suche.trim().toLowerCase();
    const l = allTags.filter((t) => !q || t.name.toLowerCase().includes(q));
    return sortierung === "name"
      ? [...l].sort((a, b) => a.name.localeCompare(b.name))
      : [...l].sort((a, b) => (b.count ?? 0) - (a.count ?? 0) || a.name.localeCompare(b.name));
  }, [allTags, suche, sortierung]);

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
      <div className="schlagwort-kopf">
        <h2>Tags</h2>
        <span className="muted small">
          {allTags.length === 1 ? "One tag" : `${allTags.length} tags`}
        </span>
      </div>

      <form className="schlagwort-anlegen" onSubmit={anlegen}>
        <input
          type="color"
          className="farbwahl"
          value={neuFarbe}
          onChange={(e) => setNeuFarbe(e.target.value)}
          title="Colour"
          aria-label="Colour"
        />
        <input
          placeholder="New tag…"
          value={neuName}
          onChange={(e) => setNeuName(e.target.value)}
          aria-label="New tag"
        />
        <button className="btn btn-primary" type="submit" disabled={!neuName.trim()}>
          Create
        </button>
      </form>

      <div className="schlagwort-werkzeuge">
        <input
          placeholder="Search tags…"
          value={suche}
          onChange={(e) => setSuche(e.target.value)}
          aria-label="Search tags"
        />
        <select
          value={sortierung}
          onChange={(e) => setSortierung(e.target.value as "name" | "anzahl")}
          aria-label="Sort"
        >
          <option value="name">By name</option>
          <option value="anzahl">By number of pages</option>
        </select>
      </div>

      {fehler && <div className="fehler">{fehler}</div>}

      {allTags.length === 0 ? (
        <p className="muted">
          No tags yet. Create one above, or add one to a page from its menu under &ldquo;Tags&hellip;&rdquo;.
        </p>
      ) : liste.length === 0 ? (
        <p className="muted">No tag matches the search.</p>
      ) : (
        <table className="tabelle schlagwort-tabelle">
          <thead>
            <tr>
              <th style={{ width: 44 }}>Colour</th>
              <th>Name</th>
              <th style={{ width: 90 }}>Pages</th>
              <th style={{ width: 90 }} />
            </tr>
          </thead>
          <tbody>
            {liste.map((t) => (
              <tr key={t.id}>
                <td>
                  <input
                    type="color"
                    className="farbwahl"
                    value={t.color}
                    onChange={(e) => void aendern(t, { color: e.target.value })}
                    title="Change colour"
                    aria-label="Change colour"
                  />
                </td>
                <td>
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
                </td>
                <td>
                  <span className={"tag-anzahl" + ((t.count ?? 0) === 0 ? " leer" : "")}>{t.count ?? 0}</span>
                </td>
                <td className="schlagwort-aktionen">
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
                  <button
                    className="icon-btn"
                    title="Delete"
                    aria-label="Delete"
                    onClick={() => void loeschen(t)}
                  >
                    <Symbol name="loeschen" />
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
