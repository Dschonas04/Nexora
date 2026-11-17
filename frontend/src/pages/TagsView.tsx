// All tags of one's account on one page: create, rename, recolour, delete,
// and see at a glance which pages carry each. Cards instead of a table: a tag
// is a place, and the newest pages under it say more than a bare number.
import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router";

import { PageMeta, Tag, api } from "../api/client";
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

const VORSCHAU = 3;

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
  const [seiten, setSeiten] = useState<Record<string, PageMeta[]>>({});
  const neuFeld = useRef<HTMLInputElement>(null);

  // The newest pages per tag, fetched once per change of the tag list. A
  // handful of small requests; with many tags only the visible ones would be
  // worth it, but an account rarely has more than a few dozen.
  const kennungen = allTags.map((t) => `${t.id}:${t.count}`).join(",");
  useEffect(() => {
    let aktiv = true;
    Promise.all(
      allTags
        .filter((t) => (t.count ?? 0) > 0)
        .map((t) =>
          api
            .seitenZuTag(t.id)
            .then((s) => [t.id, s] as const)
            .catch(() => [t.id, [] as PageMeta[]] as const),
        ),
    ).then((paare) => {
      if (aktiv) setSeiten(Object.fromEntries(paare));
    });
    return () => {
      aktiv = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kennungen]);

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
          <span className="schlagwort-leer-symbol" aria-hidden="true">
            <svg viewBox="0 0 16 16">
              <path d="M2 2.5h5.5L14 9l-5 5-6.5-6.5z" />
              <circle cx="5.2" cy="5.2" r="1" />
            </svg>
          </span>
          <p>No tags yet.</p>
          <p className="muted small">
            Create one with &ldquo;New tag&rdquo;, or add one to a page from its menu under &ldquo;Tags&hellip;&rdquo;.
          </p>
        </div>
      ) : liste.length === 0 ? (
        <p className="muted">No tag matches the search.</p>
      ) : (
        <div className="schlagwort-raster">
          {liste.map((t) => {
            const vorschau = (seiten[t.id] ?? []).slice(0, VORSCHAU);
            const rest = (t.count ?? 0) - vorschau.length;
            return (
              <article
                key={t.id}
                className={"schlagwort-karte" + ((t.count ?? 0) === 0 ? " verwaist" : "")}
                style={{ "--schlagwort-farbe": t.color } as React.CSSProperties}
              >
                <div className="schlagwort-karte-kopf">
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
                  <span className="schlagwort-karte-aktionen">
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
                  </span>
                </div>

                <div className="schlagwort-karte-zahl">
                  {(t.count ?? 0) === 0
                    ? "Not on any page"
                    : t.count === 1
                      ? "One page"
                      : `${t.count} pages`}
                </div>

                {vorschau.length > 0 && (
                  <ul className="schlagwort-vorschau">
                    {vorschau.map((p) => (
                      <li key={p.id}>
                        <button onClick={() => nav(`/page/${p.id}`)}>{p.title || "Untitled"}</button>
                      </li>
                    ))}
                    {rest > 0 && (
                      <li>
                        <button className="muted" onClick={() => nav(`/tag/${t.id}`)}>
                          {rest === 1 ? "one more" : `${rest} more`} →
                        </button>
                      </li>
                    )}
                  </ul>
                )}
              </article>
            );
          })}
        </div>
      )}
    </div>
  );
}
