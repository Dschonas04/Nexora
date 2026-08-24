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

// Schlüssel im Speicher des Browsers. Eingeklappt wird gemerkt, nicht offen:
// so ist eine neu angelegte Ablage von selbst aufgeklappt, ohne dass sie
// irgendwo eingetragen werden müsste.
const ZU_SCHLUESSEL = "nexora.leiste.eingeklappt";

function gemerkteZu(): Set<string> {
  try {
    const roh = localStorage.getItem(ZU_SCHLUESSEL);
    return new Set<string>(roh ? (JSON.parse(roh) as string[]) : []);
  } catch {
    // Ein privates Fenster ohne Speicher oder ein kaputter Eintrag darf die
    // Leiste nicht lahmlegen -- dann eben alles aufgeklappt.
    return new Set<string>();
  }
}

interface Props {
  /** Ungelesene Nachrichten -- die Zahl kommt von oben, damit sie nur einmal
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
  // Nach einer Einfuhr sind Seiten, Ablagen und Schlagworte veraltet -- die
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
  // Verzweigungen INNERHALB eines Baums steuert -- hier geht es um den
  // Abschnitt als Ganzes.
  const [zu, setZu] = useState<Set<string>>(gemerkteZu);
  const klappen = (key: string) =>
    setZu((prev) => {
      const n = new Set(prev);
      n.has(key) ? n.delete(key) : n.add(key);
      try {
        localStorage.setItem(ZU_SCHLUESSEL, JSON.stringify([...n]));
      } catch {
        // Nicht speichern zu können ist kein Grund, nicht einzuklappen.
      }
      return n;
    });
  // Beim Ziehen klappt ein Abschnitt von selbst auf, sobald der Zeiger über
  // seiner Überschrift verweilt. Ohne das müsste man die Seite ablegen,
  // aufklappen, wieder aufnehmen -- oder das Ziel bliebe unsichtbar.
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

  // Vorlagen werden einmal geholt, nicht bei jedem Öffnen des Menüs: die Liste
  // ist kurz und ändert sich selten, ein Abruf je Klick wäre Verschwendung.
  const [vorlagen, setVorlagen] = useState<PageMeta[]>([]);
  const [vorlagenOffen, setVorlagenOffen] = useState(false);
  const [rechteFuer, setRechteFuer] = useState<{ id: string; name: string } | null>(null);
  // Kennung der Ablage, deren Sichtbarkeitsmenü offen steht -- höchstens eine
  // zur Zeit, deshalb ein einzelner Wert und keine Menge.
  const [sichtbarkeitFuer, setSichtbarkeitFuer] = useState<string | null>(null);
  // Dasselbe für das Export-Menü einer Ablage.
  const [exportFuer, setExportFuer] = useState<string | null>(null);
  // Ziel der Einfuhr, solange ihr Kasten offen steht.
  const [einfuhrZiel, setEinfuhrZiel] = useState<
    { ziel: { parentId?: string; spaceId?: string }; name: string } | null
  >(null);
  // Lange Listen werden auf vier Einträge gekürzt. Eine Leiste, die von
  // fünfzehn Ablagen und dreißig Schlagwörtern gefüllt wird, ist keine
  // Übersicht mehr -- man scrollt an allem vorbei, was man sucht. Der Rest ist
  // einen Klick entfernt und die Zahl daneben sagt, wie viel dort wartet.
  const [alleSpaces, setAlleSpaces] = useState(false);
  const [alleTags, setAlleTags] = useState(false);
  useEffect(() => {
    if (!frei("vorlagen")) return;
    api.vorlagen().then(setVorlagen).catch(() => setVorlagen([]));
  }, [frei, pages]);
  const [results, setResults] = useState<SearchHit[] | null>(null);
  // Filter der Suche. Getrennt vom Suchwort, damit ein gesetzter Filter beim
  // Weitertippen stehen bleibt -- man engt einmal ein und sucht dann mehrmals.
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
  // what keeps it inert -- dangerouslySetInnerHTML here would be a stored XSS.
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
        // Woher der Treffer stammt. Ohne diese Zeile läse sich ein Ausschnitt
        // aus einem PDF wie Seiteninhalt, den man auf der Seite dann vergeblich
        // sucht.
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

  // Wie viele Einträge eine gekürzte Liste zeigt.
  const KURZ = 4;
  // Die geöffnete Ablage wird immer gezeigt, auch wenn sie hinter der Grenze
  // liegt: die Leiste soll nicht verschweigen, wo man gerade steht.
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
        {/* Der zweite Knopf erscheint nur, wenn es überhaupt Vorlagen gibt.
            Ein leeres Menü anzubieten wäre ein Versprechen ohne Inhalt. */}
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
        {/* Der Knopf erscheint erst beim Suchen: ohne Suchwort gäbe es nichts
            einzugrenzen. Der Punkt daneben sagt, dass ein Filter steht --
            sonst wundert man sich über zu wenige Treffer. */}
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

      {/* Die beiden Aktionen hingen bisher an der Überschrift "Seiten". Damit
          waren sie an einen Abschnitt gebunden, den es nicht immer gibt --
          und der neben benannten Ablagen ohnehin fehl am Platz wirkte. Sie
          stehen jetzt für sich, oberhalb aller Abschnitte. */}
      {results === null && (
        <div className="sidebar-werkzeuge">
          <button className="text-btn" onClick={onCreateSpace}>
            + Space
          </button>
          {/* Beschriftet statt als Pfeil: ein Symbol allein sagt nicht, dass
              sich hier ein ganzes Archiv einlesen lässt -- und seit die
              Einfuhr eine eigene Ablage anlegen kann, ist das der Weg zurück
              aus einer Ausfuhr. */}
          <button
            className="text-btn"
            title="Markdown, HTML oder ein ZIP einlesen -- wahlweise als eigene Ablage"
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
                      // Eine eingeklappte Ablage öffnet sich, sobald man mit
                      // einer Seite darüber steht: sonst zieht man auf ein
                      // Ziel, dessen Inhalt man nicht sieht.
                      aufklappen(marke);
                    }}
                    onDragLeave={() => setDropTarget((t) => (t === marke ? null : t))}
                    onDrop={(e) => {
                      e.preventDefault();
                      dropOnSpace(sp.id);
                    }}
                  >
                    {/* Der ganze linke Teil klappt -- Pfeil und Name. Das
                        Umbenennen hat jetzt einen eigenen Knopf: eine
                        Beschriftung, die beim Anklicken ein Eingabefeld
                        aufmacht, ist nicht das, was man von einer Überschrift
                        in einer Leiste erwartet. */}
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
                    {/* Sagt ohne Umweg, dass hier alle mitlesen. Der Zusatz
                        steht im title, weil "offen" allein nicht verrät, ob
                        auch geschrieben werden darf. */}
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
                      {/* Verwalten-Knöpfe nur für die, die es dürfen. Das
                          Backend prüft es ohnehin noch einmal; hier geht es
                          darum, niemandem einen Knopf hinzustellen, der bei
                          jedem Druck abgewiesen wird. */}
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
                      {/* Ein normaler Verweis auf die Adresse, kein fetch: so
                          setzt der Browser den Dateinamen aus dem
                          Content-Disposition-Kopf und lädt den Strom direkt
                          auf die Platte, statt ihn erst in den Speicher zu
                          holen. */}
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

                  {/* Kleines Menü statt eines Dreifach-Umschalters: welche der
                      drei Stufen gerade gilt, soll man sehen und nicht durch
                      Weiterklicken herausfinden müssen. */}
                  {sichtbarkeitFuer === sp.id && (
                    <div className="sichtbarkeit-menue">
                      <div className="vorlagenliste-titel">Wer sieht diese Ablage?</div>
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
                          <span className="sichtbarkeit-titel">{titel}</span>
                          <span className="muted small">{erklaerung}</span>
                        </button>
                      ))}
                      {/* Der Satz steht bewusst da: "öffentlich" heißt in
                          Nexora nicht "im Internet". Wer eine Seite anonym
                          erreichbar machen will, nimmt den Freigabelink der
                          Seite. */}
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

            {/* Der Rest der Ablagen, hinter einem Klick. Die Zahl steht dabei,
                damit man weiß, ob sich das Aufklappen lohnt. */}
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

            {/* Der Auffangabschnitt für Seiten, die in keiner Ablage liegen.
                Er erscheint nur, wenn er etwas enthält -- oder wenn gerade eine
                Seite gezogen wird, denn dann ist er das Ziel, mit dem man eine
                Seite wieder aus ihrer Ablage herausholt. Eine leere Überschrift
                "Seiten" zwischen benannten Ablagen sagte nichts und stand nur
                im Weg. */}
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
                  {/* Ohne den Pfeil würde React das Klickereignis als erstes
                      Argument durchreichen -- und das wäre dann die
                      Vorlagen-Kennung. */}
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

            {/* Ein frischer Arbeitsbereich: keine Ablage, keine Seite, nichts
                geteilt. Statt drei leerer Überschriften steht hier, was als
                Nächstes zu tun ist. */}
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
                    // Anklickbar, und die Zahl dahinter sagt, ob etwas
                    // dranhängt. Ohne beides war das hier nur Zierde: eine
                    // Beschriftung, der man nicht folgen kann, verspricht eine
                    // Ordnung, die es gar nicht gibt.
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

            {/* Was zum eigenen Arbeiten gehört. Die Überschrift hieß einmal
                "Workspace" -- ein englischer Rest, der außerdem nichts über
                den Inhalt sagte: darunter standen Posteingang und Papierkorb
                neben der Nutzerverwaltung. Die Verwaltung steht deshalb jetzt
                für sich, siehe unten. */}
            <div className="sidebar-section">
              <Klapptitel marke="workspace" zu={zu} klappen={klappen}>
                Arbeitsbereich
              </Klapptitel>
              <div
                className={"tree-row" + (currentPath === "/postfach" ? " active" : "")}
                onClick={() => onNavigate("/postfach")}
              >
                <span className="tree-label">Postfach</span>
                {/* Die Zahl steht nur da, wenn sie etwas sagt. Eine Null neben
                    dem Eintrag wäre eine Aufforderung ohne Anlass. */}
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

            {/* Verwaltung. Nur für Administratoren sichtbar -- das räumt allein
                die Oberfläche auf, das Backend prüft die Rolle bei jedem Aufruf
                erneut. Das Protokoll ist zusätzlich Zusatzumfang: ein Eintrag,
                der ohnehin abgewiesen würde, wird gar nicht erst angeboten.

                Einstellungen, Nutzer und Gruppen haben keine eigene Zeile mehr:
                sie liegen alle hinter dem Zahnrad in der Überschrift. Was in
                der Liste steht, ist zum Nachlesen da, nicht zum Einstellen. */}
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

// Klapptitel ist die Überschrift eines Leistenabschnitts, der nur auf- und
// zuklappt und sonst nichts kann. Die Ablagen und der Abschnitt "Seiten" haben
// ihre eigene, ausgeschriebene Überschrift: dort hängen Ziehziele und
// Verwaltungsknöpfe dran, die hier nur im Weg wären.
// Ein kleines Zahnrad für die Verwaltung. Von Hand gezeichnet statt aus einer
// Bibliothek geholt: die Leiste braucht genau dieses eine Sinnbild, und ein
// ganzes Symbolpaket dafür mitzuschleppen wäre unverhältnismäßig.
//
// Nabe und Kranz als Kreise, die Zähne als acht kurze Striche nach außen. Bei
// dreizehn Pixeln Kantenlänge liest sich das deutlicher als ein fein
// ausgearbeiteter Umriss, der auf dieser Größe nur verschmiert.
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
  // Sinnbild vor dem Namen. Nur dort gesetzt, wo es etwas unterscheidet --
  // ein Symbol neben jeder Überschrift wäre Zierrat und keine Hilfe.
  symbol?: React.ReactNode;
  symbolTitel?: string;
  // Ist eine Aktion hinterlegt, wird aus dem Sinnbild eine Schaltfläche. Ohne
  // sie bleibt es reine Beschriftung.
  symbolAktion?: () => void;
  // Hebt das Sinnbild hervor, solange man auf der Ansicht steht, die es
  // öffnet. Ohne das verlöre man beim Klick die Anzeige, wo man ist -- die
  // Zeile, die das sonst übernahm, gibt es nicht mehr.
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
            // Sonst klappt die Überschrift zusätzlich zu, weil der Klick
            // weiterläuft und dort das Auf und Zu auslöst.
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
      {/* Die Zahl nur im eingeklappten Zustand: aufgeklappt zählt man selbst. */}
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
// therefore escaped, which is the whole reason this is safe -- the database
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
