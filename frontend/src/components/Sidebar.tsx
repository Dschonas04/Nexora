// The sidebar: search, favorites, spaces, the page tree, shared pages, tags and
// the workspace links. It owns no data of its own, everything arrives as props
// from Workspace; what it does own is view state such as which branches are
// open and what is currently being dragged.
import { useEffect, useState } from "react";
import { PageMeta, SearchHit, Space, SuchFilter, Tag, api } from "../api/client";
import { useLizenz } from "../lizenz";
import { useAuth } from "../auth";
import PageTree from "./PageTree";
import SpaceRechte from "./SpaceRechte";
import Einfuhr from "./Einfuhr";

// Keys in the browser's storage. Collapsed is remembered, not open: that way a
// newly created space is expanded by itself without having to be recorded
// anywhere.
const ZU_SCHLUESSEL = "nexora.leiste.eingeklappt";

function gemerkteZu(): Set<string> {
  try {
    const roh = localStorage.getItem(ZU_SCHLUESSEL);
    return new Set<string>(roh ? (JSON.parse(roh) as string[]) : []);
  } catch {
    // Ein privates Fenster ohne Speicher oder ein kaputter Eintrag darf die
    // Leiste nicht lahmlegen, dann eben alles aufgeklappt.
    return new Set<string>();
  }
}

interface Props {
  /** Ungelesene Nachrichten, die Zahl kommt von oben, damit sie nur einmal
      geholt wird und nicht je Ansicht neu. */
  ungelesen: number;
  pages: PageMeta[];
  shared: PageMeta[];
  favorites: PageMeta[];
  tags: Tag[];
  spaces: Space[];
  activeId?: string;
  onSelect: (id: string) => void;
  onCreateRoot: (vorlageId?: string) => void;
  onCreateChild: (parentId: string) => void;
  onCreateInSpace: (spaceId: string) => void;
  onDelete: (id: string) => void;
  onCreateSpace: () => void;
  onRenameSpace: (id: string, current: string) => void;
  onDeleteSpace: (id: string) => void;
  onSpaceOeffentlich: (id: string, wert: "nein" | "lesen" | "schreiben") => void;
  onMovePage: (id: string, parentId: string | null, spaceId: string | null) => void;
  onNavigate: (to: string) => void;
  currentPath: string;
  // Nach einer Einfuhr sind Seiten, Ablagen und Schlagworte veraltet, die
  // Leiste kann sie nicht selbst nachladen, sie besitzt keine davon.
  onEingefuehrt: () => void;
}

export default function Sidebar(props: Props) {
  const {
    pages,
    shared,
    favorites,
    tags,
    spaces,
    activeId,
    onSelect,
    onCreateRoot,
    onCreateChild,
    onCreateInSpace,
    onDelete,
    onCreateSpace,
    onRenameSpace,
    onDeleteSpace,
    onSpaceOeffentlich,
    onMovePage,
    onNavigate,
    currentPath,
    onEingefuehrt,
    ungelesen,
  } = props;
  const { user, logout } = useAuth();
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  // Eingeklappte Abschnitte der Leiste. Getrennt von `expanded`, das die
  // Verzweigungen INNERHALB eines Baums steuert, hier geht es um den
  // Abschnitt als Ganzes.
  const [zu, setZu] = useState<Set<string>>(gemerkteZu);
  const klappen = (key: string) =>
    setZu((prev) => {
      const n = new Set(prev);
      n.has(key) ? n.delete(key) : n.add(key);
      try {
        localStorage.setItem(ZU_SCHLUESSEL, JSON.stringify([...n]));
      } catch {
        // Not being able to save is no reason not to collapse.
      }
      return n;
    });
  // While dragging, a section expands by itself as soon as the pointer rests
  // over its heading. Without that one would have to drop the page, expand, pick
  // it up again, or the target would stay invisible.
  const aufklappen = (key: string) => setZu((prev) => {
    if (!prev.has(key)) return prev;
    const n = new Set(prev);
    n.delete(key);
    try {
      localStorage.setItem(ZU_SCHLUESSEL, JSON.stringify([...n]));
    } catch {
      /* siehe oben */
    }
    return n;
  });
  const [q, setQ] = useState("");
  const { frei } = useLizenz();

  // Templates are fetched once, not every time the menu opens: the list is short
  // and rarely changes, one request per click would be a waste.
  const [vorlagen, setVorlagen] = useState<PageMeta[]>([]);
  const [vorlagenOffen, setVorlagenOffen] = useState(false);
  const [rechteFuer, setRechteFuer] = useState<{ id: string; name: string } | null>(null);
  // Id of the space whose visibility menu is open, at most one at a time, hence
  // a single value and not a set.
  const [sichtbarkeitFuer, setSichtbarkeitFuer] = useState<string | null>(null);
  // The same for the export menu of a space.
  const [exportFuer, setExportFuer] = useState<string | null>(null);
  // Ziel der Einfuhr, solange ihr Kasten offen steht.
  const [einfuhrZiel, setEinfuhrZiel] = useState<
    { ziel: { parentId?: string; spaceId?: string }; name: string } | null
  >(null);
  // Long lists are cut to four entries. A sidebar filled by fifteen spaces and
  // thirty tags is no longer an overview; one scrolls past everything one is
  // looking for. The rest is one click away and the number beside it says how
  // much is waiting there.
  const [alleSpaces, setAlleSpaces] = useState(false);
  const [alleTags, setAlleTags] = useState(false);
  useEffect(() => {
    if (!frei("vorlagen")) return;
    api.vorlagen().then(setVorlagen).catch(() => setVorlagen([]));
  }, [frei, pages]);
  const [results, setResults] = useState<SearchHit[] | null>(null);
  // Filter der Suche. Getrennt vom Suchwort, damit ein gesetzter Filter beim
  // Weitertippen stehen bleibt, man engt einmal ein und sucht dann mehrmals.
  const [filter, setFilter] = useState<SuchFilter>({});
  const [filterOffen, setFilterOffen] = useState(false);
  const filterAktiv = Boolean(filter.space || filter.tag || filter.tage || filter.wer);
  const [dragId, setDragId] = useState<string | null>(null);
  // One drop target for three kinds of destination, encoded as a string: a bare
  // page id, "space:<id>" for a space header, or "root" for the ungrouped
  // section. A single value guarantees only one target can be highlighted.
  const [dropTarget, setDropTarget] = useState<string | null>(null);

  // Descendants of the dragged page. Dropping a page into its own subtree would
  // detach that branch from the tree entirely, so those targets are refused.
  // Recomputed per drag-over rather than cached, which is cheap at this size and
  // cannot go stale.
  const descendantsOf = (rootId: string): Set<string> => {
    const out = new Set<string>();
    const walk = (pid: string) => {
      for (const c of pages) {
        if ((c.parentId ?? null) === pid && !out.has(c.id)) {
          out.add(c.id);
          walk(c.id);
        }
      }
    };
    walk(rootId);
    return out;
  };
  const canDropOnPage = (targetId: string) =>
    !!dragId && dragId !== targetId && !descendantsOf(dragId).has(targetId);

  // The three drop destinations. Dropping onto a page nests beneath it and
  // takes over that page's space, so a subtree cannot end up split across two
  // spaces. Dropping onto a space header moves the page there at top level, and
  // dropping onto the ungrouped section clears its space.
  const dropOnPage = (target: PageMeta) => {
    if (dragId && canDropOnPage(target.id)) onMovePage(dragId, target.id, target.spaceId ?? null);
    setDragId(null);
    setDropTarget(null);
  };
  const dropOnSpace = (spaceId: string | null) => {
    if (dragId) onMovePage(dragId, null, spaceId);
    setDragId(null);
    setDropTarget(null);
  };

  const toggle = (id: string) =>
    setExpanded((prev) => {
      const n = new Set(prev);
      n.has(id) ? n.delete(id) : n.add(id);
      return n;
    });

  // Debounced search: 250 ms after the last keystroke. The cleanup cancels the
  // pending timer, so typing quickly fires exactly one request. A null result
  // means "not searching" and shows the normal tree again, which is different
  // from an empty array meaning "no matches".
  useEffect(() => {
    if (!q.trim()) {
      setResults(null);
      return;
    }
    const t = setTimeout(() => {
      api.search(q, filter).then(setResults).catch(() => setResults([]));
    }, 250);
    return () => clearTimeout(t);
  }, [q, filter]);

  // Flat row without nesting or drag handles, used wherever hierarchy carries no
  // meaning: search results, favorites and pages shared with the user.
  const flatRow = (p: PageMeta) => (
    <div
      key={p.id}
      className={"tree-row" + (activeId === p.id ? " active" : "")}
      onClick={() => onSelect(p.id)}
    >
      <span className="tree-label">{p.title || "Ohne Titel"}</span>
    </div>
  );

  // A search result row: title, and under it the snippet with the matched words
  // marked. The snippet arrives with <b> markers around the hits.
  //
  // It is split on those markers and rendered as React nodes. That matters:
  // ts_headline does NOT escape the surrounding text, so page content
  // containing a < arrives verbatim. React escapes every text node, which is
  // what keeps it inert, dangerouslySetInnerHTML here would be a stored XSS.
  const trefferRow = (h: SearchHit) => (
    <div
      key={h.id}
      className={"tree-row treffer" + (activeId === h.id ? " active" : "")}
      onClick={() => onSelect(h.id)}
    >
      <div className="treffer-titel">
        <span className="tree-label">{h.title || "Ohne Titel"}</span>
        {/* Says plainly that this page is not the user's own, instead of
            leaving them to wonder why it is not in their tree. */}
        {!h.eigen && <span className="pill klein">geteilt</span>}
      </div>
      {h.quelle && (
        // Where the hit comes from. Without this line an excerpt from a PDF
        // would read like page content that one then looks for on the page in
        // vain.
        <div className="treffer-quelle muted small">aus Anhang: {h.quelle}</div>
      )}
      {h.ausschnitt.trim() !== "" && (
        <div className="treffer-ausschnitt">{markiere(h.ausschnitt)}</div>
      )}
    </div>
  );

  // Pages outside every space. They get their own section below the spaces, so
  // no page can become invisible by not belonging anywhere.
  const ungrouped = pages.filter((p) => !p.spaceId);

  // How many entries a shortened list shows.
  const KURZ = 4;
  // The open space is always shown, even when it lies beyond the limit: the
  // sidebar shall not conceal where one currently stands.
  const aktiveSpaceId = pages.find((p) => p.id === activeId)?.spaceId ?? null;
  const sichtbareSpaces = alleSpaces
    ? spaces
    : spaces.filter((sp, idx) => idx < KURZ || sp.id === aktiveSpaceId);
  const sichtbareTags = alleTags ? tags : tags.slice(0, KURZ);

  // Drag-and-drop bundle handed to the page tree.
  const dnd = {
    dragId,
    dropTarget,
    onDragStartPage: (id: string) => setDragId(id),
    onDragEndPage: () => {
      setDragId(null);
      setDropTarget(null);
    },
    onDragOverPage: (id: string) => {
      if (canDropOnPage(id)) setDropTarget(id);
    },
    onDragLeavePage: (id: string) => setDropTarget((t) => (t === id ? null : t)),
    onDropPage: (page: PageMeta) => dropOnPage(page),
    canDropOnPage,
  };

  return (
    <div className="sidebar">
      <div className="sidebar-header">
        <span className="brand">Nexora</span>
        <button className="icon-btn" title="Neue Seite" onClick={() => onCreateRoot()}>
          +
        </button>
        {/* The second button appears only when there are templates at all.
            Offering an empty menu would be a promise without content. */}
        {frei("vorlagen") && vorlagen.length > 0 && (
          <div className="vorlagenmenue">
            <button
              className="icon-btn"
              title="Neue Seite aus Vorlage"
              onClick={() => setVorlagenOffen((v) => !v)}
            >
              ▾
            </button>
            {vorlagenOffen && (
              <div className="vorlagenliste">
                <div className="vorlagenliste-titel">Aus Vorlage</div>
                {vorlagen.map((v) => (
                  <button
                    key={v.id}
                    className="vorlageneintrag"
                    onClick={() => {
                      setVorlagenOffen(false);
                      onCreateRoot(v.id);
                    }}
                  >
                    {v.title || "Ohne Titel"}
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
      </div>

      <div className="search-box">
        <input placeholder="Suchen…" value={q} onChange={(e) => setQ(e.target.value)} />
        {/* The button appears only while searching: without a search term there
            would be nothing to narrow down. The dot beside it says that a filter
            is set, otherwise one wonders about too few hits. */}
        {q.trim() !== "" && (
          <button
            className={"icon-btn" + (filterAktiv ? " aktiv" : "")}
            title={filterAktiv ? "Filter (aktiv)" : "Treffer eingrenzen"}
            onClick={() => setFilterOffen((v) => !v)}
          >
            ⚙
          </button>
        )}
      </div>

      {q.trim() !== "" && filterOffen && (
        <div className="suchfilter">
          <select
            value={filter.space ?? ""}
            onChange={(e) => setFilter((f) => ({ ...f, space: e.target.value || undefined }))}
          >
            <option value="">Alle Ablagen</option>
            <option value="ohne">Ohne Ablage</option>
            {spaces.map((sp) => (
              <option key={sp.id} value={sp.id}>
                {sp.name}
              </option>
            ))}
          </select>
          <select
            value={filter.tag ?? ""}
            onChange={(e) => setFilter((f) => ({ ...f, tag: e.target.value || undefined }))}
          >
            <option value="">Alle Schlagworte</option>
            {tags.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
          <select
            value={filter.tage ? String(filter.tage) : ""}
            onChange={(e) =>
              setFilter((f) => ({ ...f, tage: e.target.value ? Number(e.target.value) : undefined }))
            }
          >
            <option value="">Beliebig alt</option>
            <option value="7">Letzte 7 Tage</option>
            <option value="30">Letzte 30 Tage</option>
            <option value="365">Letztes Jahr</option>
          </select>
          <select
            value={filter.wer ?? ""}
            onChange={(e) => setFilter((f) => ({ ...f, wer: e.target.value || undefined }))}
          >
            <option value="">Von allen</option>
            <option value="ich">Nur meine</option>
          </select>
          {filterAktiv && (
            <button className="link-btn" onClick={() => setFilter({})}>
              zurücksetzen
            </button>
          )}
        </div>
      )}

      {/* Both actions used to hang on the heading "Seiten". That tied them to a
          section which does not always exist and which looked out of place
          beside named spaces anyway. They now stand on their own, above all
          sections. */}
      {results === null && (
        <div className="sidebar-werkzeuge">
          <button className="text-btn" onClick={onCreateSpace}>
            + Space
          </button>
          {/* Labelled instead of an arrow: an icon alone does not say that a
              whole archive can be read in here, and since the import can create
              a space of its own, this is the way back out of an export. */}
          <button
            className="text-btn"
            title="Markdown, HTML oder ein ZIP einlesen, wahlweise als eigene Ablage"
            onClick={() => setEinfuhrZiel({ ziel: {}, name: "Seiten" })}
          >
            ↑ Einlesen
          </button>
        </div>
      )}

      <div className="sidebar-scroll">
        {results !== null ? (
          <div className="sidebar-section">
            <div className="sidebar-section-title">
              Ergebnisse{filterAktiv ? " (gefiltert)" : ""}
            </div>
            {results.length === 0 && <div className="tree-row muted">Keine Treffer</div>}
            {results.map(trefferRow)}
          </div>
        ) : (
          <>
            {favorites.length > 0 && (
              <div className="sidebar-section">
                <Klapptitel marke="favoriten" zu={zu} klappen={klappen} anzahl={favorites.length}>
                  Favoriten
                </Klapptitel>
                {!zu.has("favoriten") && favorites.map(flatRow)}
              </div>
            )}

            {/* One section per space, each with its own tree. Passing only that
                space's pages means the tree component never has to know spaces
                exist. */}
            {sichtbareSpaces.map((sp) => {
              const spacePages = pages.filter((p) => p.spaceId === sp.id);
              const marke = "space:" + sp.id;
              const eingeklappt = zu.has(marke);
              return (
                <div className="sidebar-section" key={sp.id}>
                  <div
                    className={"sidebar-section-title" + (dropTarget === marke ? " drop-target" : "")}
                    onDragOver={(e) => {
                      if (!dragId) return;
                      e.preventDefault();
                      setDropTarget(marke);
                      // A collapsed space opens as soon as one hovers over it
                      // with a page: otherwise one drags onto a target whose
                      // content one cannot see.
                      aufklappen(marke);
                    }}
                    onDragLeave={() => setDropTarget((t) => (t === marke ? null : t))}
                    onDrop={(e) => {
                      e.preventDefault();
                      dropOnSpace(sp.id);
                    }}
                  >
                    {/* The whole left part folds, arrow and name. Renaming now
                        has a button of its own: a label that opens an input
                        field when clicked is not what one expects from a heading
                        in a sidebar. */}
                    <button
                      className="klapp-btn"
                      aria-expanded={!eingeklappt}
                      title={eingeklappt ? "Aufklappen" : "Einklappen"}
                      onClick={() => klappen(marke)}
                    >
                      {eingeklappt ? "▸" : "▾"}
                    </button>
                    <span className="klapp-name" onClick={() => klappen(marke)}>
                      {sp.name}
                    </span>
                    {/* Says without detour that everybody reads along here. The
                        addition stands in the title because "offen" alone does
                        not reveal whether writing is allowed too. */}
                    {sp.oeffentlich !== "nein" && (
                      <span
                        className="pill klein offen"
                        title={
                          sp.oeffentlich === "schreiben"
                            ? "Öffentliche Ablage: alle angemeldeten Konten dürfen lesen und bearbeiten"
                            : "Öffentliche Ablage: alle angemeldeten Konten dürfen lesen"
                        }
                      >
                        {sp.oeffentlich === "schreiben" ? "offen" : "öffentlich"}
                      </span>
                    )}
                    {/* Die Zahl erscheint nur eingeklappt: aufgeklappt sieht
                        man die Seiten ja. */}
                    {eingeklappt && spacePages.length > 0 && (
                      <span className="tag-anzahl muted small">{spacePages.length}</span>
                    )}
                    <span className="tree-actions" style={{ display: "flex" }}>
                      <button className="icon-btn" title="Neue Seite" onClick={() => onCreateInSpace(sp.id)}>
                        +
                      </button>
                      {/*Manage buttons only for those allowed to. The backend
                          checks it again anyway; the point here is not to put a
                          button in front of somebody that is refused on every
                          press. */}
                      {sp.darfVerwalten && (
                        <button
                          className="icon-btn"
                          title="Ablage umbenennen"
                          onClick={() => onRenameSpace(sp.id, sp.name)}
                        >
                          ✎
                        </button>
                      )}
                      {sp.darfVerwalten && (
                        <button
                          className="icon-btn"
                          title="Sichtbarkeit dieser Ablage"
                          onClick={() => setSichtbarkeitFuer((v) => (v === sp.id ? null : sp.id))}
                        >
                          ◎
                        </button>
                      )}
                      {frei("gruppen") && sp.darfVerwalten && (
                        <button
                          className="icon-btn"
                          title="Rechte an diesem Space"
                          onClick={() => setRechteFuer({ id: sp.id, name: sp.name })}
                        >
                          ⚿
                        </button>
                      )}
                      <button
                        className="icon-btn"
                        title="Markdown in diese Ablage einführen"
                        onClick={() => setEinfuhrZiel({ ziel: { spaceId: sp.id }, name: sp.name })}
                      >
                        ↑
                      </button>
                      {/* An ordinary link to the address, not a fetch: that way
                          the browser takes the file name from the
                          Content-Disposition header and writes the stream
                          straight to disk instead of pulling it into memory
                          first. */}
                      {frei("export") && (
                        <div className="exportmenue">
                          <button
                            className="icon-btn"
                            title="Ablage exportieren"
                            onClick={() => setExportFuer((v) => (v === sp.id ? null : sp.id))}
                          >
                            ↓
                          </button>
                          {exportFuer === sp.id && (
                            <div className="vorlagenliste" onMouseLeave={() => setExportFuer(null)}>
                              <div className="vorlagenliste-titel">Ablage exportieren</div>
                              {(
                                [
                                  ["", "Markdown-Dateien (.zip)"],
                                  ["pdf", "Ein PDF mit allen Seiten"],
                                  ["word", "Ein Word-Dokument"],
                                ] as const
                              ).map(([form, titel]) => (
                                <button
                                  key={titel}
                                  className="vorlageneintrag"
                                  onClick={() => {
                                    setExportFuer(null);
                                    window.location.href =
                                      `/api/spaces/${sp.id}/export` + (form ? `?format=${form}` : "");
                                  }}
                                >
                                  {titel}
                                </button>
                              ))}
                            </div>
                          )}
                        </div>
                      )}
                      {sp.darfVerwalten && (
                        <button className="icon-btn" title="Space löschen" onClick={() => onDeleteSpace(sp.id)}>
                          ✕
                        </button>
                      )}
                    </span>
                  </div>

                  {/* A small menu instead of a three way toggle: which of the
                      three levels applies shall be visible and not something one
                      has to find out by clicking on. */}
                  {sichtbarkeitFuer === sp.id && (
                    <div className="sichtbarkeit-menue">
                      <div className="sichtbarkeit-kopf">Wer sieht diese Ablage?</div>
                      {(
                        [
                          ["nein", "Nur Berechtigte", "Eigentümer und wer ausdrücklich ein Recht hat"],
                          ["lesen", "Alle dürfen lesen", "Jedes angemeldete Konto dieser Instanz"],
                          ["schreiben", "Alle dürfen bearbeiten", "Jedes angemeldete Konto darf auch ändern"],
                        ] as const
                      ).map(([wert, titel, erklaerung]) => (
                        <button
                          key={wert}
                          className={"sichtbarkeit-eintrag" + (sp.oeffentlich === wert ? " gewaehlt" : "")}
                          onClick={() => {
                            setSichtbarkeitFuer(null);
                            if (sp.oeffentlich !== wert) onSpaceOeffentlich(sp.id, wert);
                          }}
                        >
                          <span className="sichtbarkeit-text">
                            <span className="sichtbarkeit-titel">{titel}</span>
                            <span className="sichtbarkeit-erklaerung">{erklaerung}</span>
                          </span>
                          {/* The check marks what applies. A tick beside the
                              entry is read as a state; a coloured row is read as
                              a button one is about to press. */}
                          <span className="sichtbarkeit-haken" aria-hidden="true">
                            {sp.oeffentlich === wert ? "✓" : ""}
                          </span>
                        </button>
                      ))}
                      {/* The sentence stands there on purpose: "public" does not
                          mean "on the internet" in Nexora. Whoever wants a page
                          reachable anonymously takes the share link of the
                          page. */}
                      <div className="sichtbarkeit-hinweis muted small">
                        Betrifft nur angemeldete Konten dieser Instanz. Ohne Anmeldung bleibt die
                        Ablage unerreichbar.
                      </div>
                    </div>
                  )}

                  {eingeklappt ? null : spacePages.length === 0 ? (
                    /* An empty space still needs a drop area, otherwise there
                       would be no way to drag the first page into it. */
                    <div
                      className={"tree-row muted" + (dropTarget === marke ? " drop-target" : "")}
                      onDragOver={(e) => {
                        if (!dragId) return;
                        e.preventDefault();
                        setDropTarget(marke);
                      }}
                      onDrop={(e) => {
                        e.preventDefault();
                        dropOnSpace(sp.id);
                      }}
                    >
                      Leer
                    </div>
                  ) : (
                    <PageTree
                      pages={spacePages}
                      parentId={null}
                      activeId={activeId}
                      expanded={expanded}
                      onToggle={toggle}
                      onSelect={onSelect}
                      onCreateChild={onCreateChild}
                      onDelete={onDelete}
                      dnd={dnd}
                    />
                  )}
                </div>
              );
            })}

            {/* The rest of the spaces, behind one click. The number stands with
                it so that one knows whether expanding is worth it. */}
            {spaces.length > sichtbareSpaces.length && (
              <div className="tree-row mehr-zeile" onClick={() => setAlleSpaces(true)}>
                <span className="tree-label">
                  {spaces.length - sichtbareSpaces.length} weitere Ablagen
                </span>
              </div>
            )}
            {alleSpaces && spaces.length > KURZ && (
              <div className="tree-row mehr-zeile" onClick={() => setAlleSpaces(false)}>
                <span className="tree-label">Weniger anzeigen</span>
              </div>
            )}

            {/* The catch all section for pages lying in no space. It appears only
                when it contains something, or while a page is being dragged,
                because then it is the target with which one pulls a page back
                out of its space. An empty heading "Seiten" between named spaces
                said nothing and only stood in the way. */}
            {(ungrouped.length > 0 || dragId) && (
            <div className="sidebar-section">
              <div
                className={"sidebar-section-title" + (dropTarget === "root" ? " drop-target" : "")}
                onDragOver={(e) => {
                  if (!dragId) return;
                  e.preventDefault();
                  setDropTarget("root");
                  aufklappen("root");
                }}
                onDragLeave={() => setDropTarget((t) => (t === "root" ? null : t))}
                onDrop={(e) => {
                  e.preventDefault();
                  dropOnSpace(null);
                }}
              >
                <button
                  className="klapp-btn"
                  aria-expanded={!zu.has("root")}
                  title={zu.has("root") ? "Aufklappen" : "Einklappen"}
                  onClick={() => klappen("root")}
                >
                  {zu.has("root") ? "▸" : "▾"}
                </button>
                <span className="klapp-name" onClick={() => klappen("root")}>
                  Ohne Ablage
                </span>
                {zu.has("root") && ungrouped.length > 0 && (
                  <span className="tag-anzahl muted small">{ungrouped.length}</span>
                )}
                <span className="tree-actions">
                  {/* Without the arrow React would pass the click event as the
                      first argument, and that would then be the template id. */}
                  <button className="icon-btn" title="Neue Seite" onClick={() => onCreateRoot()}>
                    +
                  </button>
                </span>
              </div>
              {zu.has("root") ? null : ungrouped.length === 0 ? (
                <div
                  className={"tree-row muted" + (dropTarget === "root" ? " drop-target" : "")}
                  onDragOver={(e) => {
                    if (!dragId) return;
                    e.preventDefault();
                    setDropTarget("root");
                  }}
                  onDrop={(e) => {
                    e.preventDefault();
                    dropOnSpace(null);
                  }}
                >
                  Hierher ziehen, um aus der Ablage zu nehmen
                </div>
              ) : (
                <PageTree
                  pages={ungrouped}
                  parentId={null}
                  activeId={activeId}
                  expanded={expanded}
                  onToggle={toggle}
                  onSelect={onSelect}
                  onCreateChild={onCreateChild}
                  onDelete={onDelete}
                  dnd={dnd}
                />
              )}
            </div>
            )}

            {/* A fresh workspace: no space, no page, nothing shared. Instead of
                three empty headings, what to do next stands here. */}
            {spaces.length === 0 && pages.length === 0 && shared.length === 0 && (
              <div className="sidebar-section leerer-anfang">
                <p className="muted">
                  Noch nichts angelegt. Eine Ablage ordnet Seiten zu einem Thema &mdash; oder
                  fang einfach mit einer Seite an.
                </p>
                <button className="btn" onClick={onCreateSpace}>
                  Erste Ablage anlegen
                </button>
                <button className="btn" onClick={() => onCreateRoot()}>
                  Erste Seite anlegen
                </button>
                <button
                  className="btn"
                  onClick={() => setEinfuhrZiel({ ziel: {}, name: "Seiten" })}
                >
                  Ablage einlesen
                </button>
              </div>
            )}

            {shared.length > 0 && (
              <div className="sidebar-section">
                <Klapptitel marke="geteilt" zu={zu} klappen={klappen} anzahl={shared.length}>
                  Mit mir geteilt
                </Klapptitel>
                {!zu.has("geteilt") && shared.map(flatRow)}
              </div>
            )}

            {tags.length > 0 && (
              <div className="sidebar-section">
                <Klapptitel marke="schlagwoerter" zu={zu} klappen={klappen} anzahl={tags.length}>
                  Schlagwörter
                </Klapptitel>
                {!zu.has("schlagwoerter") &&
                  sichtbareTags.map((t) => (
                    // Clickable, and the number behind it says whether anything
                    // hangs on it. Without both this was mere decoration: a
                    // label one cannot follow promises an order that does not
                    // exist.
                    <div
                      key={t.id}
                      className={"tree-row" + (currentPath === `/tag/${t.id}` ? " active" : "")}
                      onClick={() => onNavigate(`/tag/${t.id}`)}
                    >
                      <span className="tag-dot" style={{ background: t.color }} />
                      <span className="tree-label">{t.name}</span>
                      <span className={"tag-anzahl muted small" + (t.anzahl === 0 ? " leer" : "")}>
                        {t.anzahl}
                      </span>
                    </div>
                  ))}
                {!zu.has("schlagwoerter") && tags.length > sichtbareTags.length && (
                  <div className="tree-row mehr-zeile" onClick={() => setAlleTags(true)}>
                    <span className="tree-label">
                      {tags.length - sichtbareTags.length} weitere Schlagwörter
                    </span>
                  </div>
                )}
                {!zu.has("schlagwoerter") && alleTags && tags.length > KURZ && (
                  <div className="tree-row mehr-zeile" onClick={() => setAlleTags(false)}>
                    <span className="tree-label">Weniger anzeigen</span>
                  </div>
                )}
              </div>
            )}

            {/* What belongs to one's own work. The heading was once called
                "Workspace", an English leftover that also said nothing about the
                content: below it stood the inbox and the trash next to user
                administration. Administration therefore now stands on its own,
                see below. */}
            <div className="sidebar-section">
              <Klapptitel marke="workspace" zu={zu} klappen={klappen}>
                Arbeitsbereich
              </Klapptitel>
              <div
                className={"tree-row" + (currentPath === "/postfach" ? " active" : "")}
                onClick={() => onNavigate("/postfach")}
              >
                <span className="tree-label">Postfach</span>
                {/* The number stands there only when it says something. A zero
                    beside the entry would be a prompt without occasion. */}
                {ungelesen > 0 && <span className="postfach-zaehler">{ungelesen}</span>}
              </div>
              <div
                className={"tree-row" + (currentPath === "/graph" ? " active" : "")}
                onClick={() => onNavigate("/graph")}
              >
                <span className="tree-label">Wissensgraph</span>
              </div>
              <div
                className={"tree-row" + (currentPath === "/trash" ? " active" : "")}
                onClick={() => onNavigate("/trash")}
              >
                <span className="tree-label">Papierkorb</span>
              </div>
            </div>

            {/* Administration. Visible only to administrators, which merely
                tidies up the interface; the backend checks the role again on
                every call. The log is a paid extra on top: an entry that would
                be refused anyway is not offered in the first place.

                Settings, users and groups no longer have a row of their own:
                they all live behind the gear in the heading. What stands in the
                list is there to read, not to configure. */}
            {user?.role === "admin" && (
              <div className="sidebar-section">
                <Klapptitel
                  marke="verwaltung"
                  zu={zu}
                  klappen={klappen}
                  symbol={<Zahnrad />}
                  symbolTitel="Einstellungen"
                  symbolAktion={() => onNavigate("/einstellungen")}
                  symbolAktiv={currentPath.startsWith("/einstellungen")}
                >
                  Verwaltung
                </Klapptitel>
                {frei("pruefspur") && (
                  <div
                    className={"tree-row" + (currentPath === "/pruefspur" ? " active" : "")}
                    onClick={() => onNavigate("/pruefspur")}
                  >
                    <span className="tree-label">Protokoll</span>
                  </div>
                )}
              </div>
            )}
          </>
        )}
      </div>

      {rechteFuer && (
        <SpaceRechte
          spaceId={rechteFuer.id}
          spaceName={rechteFuer.name}
          onClose={() => setRechteFuer(null)}
        />
      )}

      {einfuhrZiel && (
        <Einfuhr
          ziel={einfuhrZiel.ziel}
          zielName={einfuhrZiel.name}
          onFertig={onEingefuehrt}
          onClose={() => setEinfuhrZiel(null)}
        />
      )}

      <div className="sidebar-footer">
        <span className="tree-label">{user?.name}</span>
        <button className="btn" onClick={logout}>
          Abmelden
        </button>
      </div>
    </div>
  );
}

// Klapptitel is the heading of a sidebar section that only expands and
// collapses and can do nothing else. The spaces and the section "Seiten" have
// their own written out heading: drop targets and management buttons hang there,
// which would only be in the way here.
// A small gear for administration. Drawn by hand instead of taken from a
// library: the sidebar needs exactly this one symbol, and dragging a whole icon
// set along for it would be out of proportion.
//
// Hub and rim as circles, the teeth as eight short strokes pointing outwards. At
// thirteen pixels a side that reads more clearly than a finely worked outline,
// which only smears at this size.
function Zahnrad() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">
      <circle cx="8" cy="8" r="2.2" />
      <circle cx="8" cy="8" r="4.3" />
      <g>
        <line x1="12.30" y1="8.00" x2="14.20" y2="8.00" />
        <line x1="11.04" y1="11.04" x2="12.38" y2="12.38" />
        <line x1="8.00" y1="12.30" x2="8.00" y2="14.20" />
        <line x1="4.96" y1="11.04" x2="3.62" y2="12.38" />
        <line x1="3.70" y1="8.00" x2="1.80" y2="8.00" />
        <line x1="4.96" y1="4.96" x2="3.62" y2="3.62" />
        <line x1="8.00" y1="3.70" x2="8.00" y2="1.80" />
        <line x1="11.04" y1="4.96" x2="12.38" y2="3.62" />
      </g>
    </svg>
  );
}

function Klapptitel({
  marke,
  zu,
  klappen,
  anzahl,
  symbol,
  symbolTitel,
  symbolAktion,
  symbolAktiv,
  children,
}: {
  marke: string;
  zu: Set<string>;
  klappen: (marke: string) => void;
  anzahl?: number;
  // Symbol in front of the name. Only set where it distinguishes something; an
  // icon beside every heading would be ornament and no help.
  symbol?: React.ReactNode;
  symbolTitel?: string;
  // If an action is given, the symbol becomes a button. Without one it stays a
  // pure label.
  symbolAktion?: () => void;
  // Highlights the symbol as long as one stands on the view it opens. Without
  // that one would lose the indication of where one is on clicking; the row that
  // used to do that does not exist any more.
  symbolAktiv?: boolean;
  children: React.ReactNode;
}) {
  const eingeklappt = zu.has(marke);
  return (
    <div className="sidebar-section-title">
      <button
        className="klapp-btn"
        aria-expanded={!eingeklappt}
        title={eingeklappt ? "Aufklappen" : "Einklappen"}
        onClick={() => klappen(marke)}
      >
        {eingeklappt ? "▸" : "▾"}
      </button>
      {symbol &&
        (symbolAktion ? (
          <button
            className={"klapp-symbol klapp-symbol-btn" + (symbolAktiv ? " aktiv" : "")}
            title={symbolTitel}
            aria-label={symbolTitel}
            // Otherwise the heading collapses as well, because the click keeps
            // travelling and triggers the open and close there.
            onClick={(e) => {
              e.stopPropagation();
              symbolAktion();
            }}
          >
            {symbol}
          </button>
        ) : (
          <span className="klapp-symbol">{symbol}</span>
        ))}
      <span className="klapp-name" onClick={() => klappen(marke)}>
        {children}
      </span>
      {/* The number only while collapsed: expanded one counts oneself. */}
      {eingeklappt && anzahl !== undefined && anzahl > 0 && (
        <span className="tag-anzahl muted small">{anzahl}</span>
      )}
    </div>
  );
}

// markiere turns the "<b>…</b>" markers ts_headline puts around matches into
// real elements.
//
// Everything between the markers goes through React as a text node and is
// therefore escaped, which is the whole reason this is safe, the database
// hands over raw page text, not escaped HTML.
//
// The one imperfection: a page that literally contains "<b>" gets that piece
// marked as a hit. Cosmetic, and the alternative (a second escaping pass)
// would break the markers themselves.
function markiere(s: string) {
  return s.split(/(<b>.*?<\/b>)/g).map((teil, i) =>
    teil.startsWith("<b>") ? (
      <mark key={i}>{teil.slice(3, -4)}</mark>
    ) : (
      <span key={i}>{teil}</span>
    ),
  );
}
