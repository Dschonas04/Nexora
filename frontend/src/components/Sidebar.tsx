// The sidebar: search, favorites, spaces, the page tree, shared pages, tags and
// the workspace links. It owns no data of its own, everything arrives as props
// from Workspace; what it does own is view state such as which branches are
// open and what is currently being dragged.
import { useEffect, useRef, useState } from "react";
import { PageMeta, SearchHit, Space, SuchFilter, Tag, api } from "../api/client";
import { useLizenz } from "../lizenz";
import { useAuth } from "../auth";
import PageTree, { TreeGap } from "./PageTree";
import { useAussenklick } from "../klappen";
import SpaceRechte from "./SpaceRechte";
import { ABLAGE_FARBEN } from "../ablagefarben";
import Einfuhr from "./Einfuhr";
import PasswortDialog from "./PasswortDialog";
import ProfilDialog from "./ProfilDialog";
import Profilbild from "./Profilbild";

// Keys in the browser's storage. Collapsed is remembered, not open: that way a
// newly created space is expanded by itself without having to be recorded
// anywhere.
const ZU_SCHLUESSEL = "nexora.leiste.eingeklappt";
const VERSTECKT_SCHLUESSEL = "nexora.leiste.versteckt";
const BREITE_SCHLUESSEL = "nexora.leiste.breite";
// Width constraints. Narrower than 180 pixels page titles are truncated; wider
// than 520 pixels the page area would encroach on the space the sidebar owns.
const BREITE_VORGABE = 260;
const BREITE_MIN = 180;
const BREITE_MAX = 520;

function gemerkteZu(): Set<string> {
  try {
    const roh = localStorage.getItem(ZU_SCHLUESSEL);
    return new Set<string>(roh ? (JSON.parse(roh) as string[]) : []);
  } catch {
    // If storage is unavailable (private mode) or the entry is corrupted,
    // do not break the sidebar; default to everything expanded.
    return new Set<string>();
  }
}

function gemerkteBreite(): number {
  try {
    const roh = Number(localStorage.getItem(BREITE_SCHLUESSEL));
    if (Number.isFinite(roh) && roh >= BREITE_MIN && roh <= BREITE_MAX) return roh;
  } catch {
    // see above
  }
  return BREITE_VORGABE;
}

function gemerktVersteckt(): boolean {
  try {
    return localStorage.getItem(VERSTECKT_SCHLUESSEL) === "ja";
  } catch {
    return false;
  }
}

interface Props {
    /** Unread messages: the count is provided from above so it is fetched once
      and not on every view. */
  ungelesen: number;
  pages: PageMeta[];
  shared: PageMeta[];
  favorites: PageMeta[];
  tags: Tag[];
  spaces: Space[];
  activeId?: string;
  onSelect: (id: string) => void;
  onCreateRoot: () => void;
  onCreateChild: (parentId: string) => void;
  onCreateInSpace: (spaceId: string) => void;
  onDelete: (id: string) => void;
  onCreateSpace: () => void;
  onRenameSpace: (id: string, current: string) => void;
  onDeleteSpace: (id: string) => void;
  onSpaceOeffentlich: (id: string, wert: "nein" | "lesen" | "schreiben") => void;
  /** Die Farbe einer Ablage. Leerer Wert heißt: zurück zur Farbe aus der Reihe. */
  onSpaceFarbe: (id: string, farbe: string) => void;
  onMovePage: (id: string, parentId: string | null, spaceId: string | null) => void;
    /** Reparenting AND reordering in one: the target names the parent level and
      the sibling before which the page will be placed. */
  onOrdnePage: (id: string, ziel: TreeGap) => void;
    /** The full order of spaces, complete and in the new sequence. */
  onOrdneSpaces: (ids: string[]) => void;
  onNavigate: (to: string) => void;
  currentPath: string;
  // After an import pages, spaces and tags are stale; the sidebar cannot
  // reload them itself because it does not own that data.
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
    onSpaceFarbe,
    onMovePage,
    onOrdnePage,
    onOrdneSpaces,
    onNavigate,
    currentPath,
    onEingefuehrt,
    ungelesen,
  } = props;
  const { user, logout } = useAuth();
  const [passwortOffen, setPasswortOffen] = useState(false);
  const [profilOffen, setProfilOffen] = useState(false);
  // The account menu in the bottom-left. It stays closed until the avatar
  // is clicked: the sidebar is narrow and three side-by-side buttons would
  // crowd out the username the menu serves.
  const [kontoMenue, setKontoMenue] = useState(false);
  const kontoEcke = useRef<HTMLDivElement>(null);

  // Clicking outside or pressing Escape closes the menu. Without this it
  // would remain open while the user does something else, obscuring the lower
  // part of the tree.
  useEffect(() => {
    if (!kontoMenue) return;
    const daneben = (e: MouseEvent) => {
      if (!kontoEcke.current?.contains(e.target as Node)) setKontoMenue(false);
    };
    const taste = (e: KeyboardEvent) => e.key === "Escape" && setKontoMenue(false);
    document.addEventListener("mousedown", daneben);
    document.addEventListener("keydown", taste);
    return () => {
      document.removeEventListener("mousedown", daneben);
      document.removeEventListener("keydown", taste);
    };
  }, [kontoMenue]);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  // Collapsed sections of the sidebar. Distinct from `expanded`, which
  // controls branches within a tree; this concerns whole sections.
  const [zu, setZu] = useState<Set<string>>(gemerkteZu);
  // Whether the entire sidebar is hidden. Separate from `zu`: that collapses
  // sections, this tucks the whole sidebar aside to leave only the page.
  const [versteckt, setVersteckt] = useState<boolean>(gemerktVersteckt);
  // The sidebar width in pixels. Stored in state because it is resized by
  // dragging; the value is persisted only on pointer release to avoid
  // spamming storage during a drag.
  const [breite, setBreite] = useState<number>(gemerkteBreite);
  const zug = useRef<{ startX: number; startBreite: number } | null>(null);
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

  // Toggle all sections at once. As long as at least one is open the control
  // closes them — pressing it a second time performs the inverse of the first
  // action instead of repeating it.
  const alleMarken = () => {
    const marken = ["favoriten", "geteilt", "schlagwoerter", "workspace", "verwaltung", "root"];
    for (const sp of spaces) marken.push("space:" + sp.id);
    return marken;
  };
  const alleKlappen = () => {
    const marken = alleMarken();
    const zumachen = marken.some((m) => !zu.has(m));
    const n = zumachen ? new Set([...zu, ...marken]) : new Set<string>();
    setZu(n);
    try {
      localStorage.setItem(ZU_SCHLUESSEL, JSON.stringify([...n]));
    } catch {
      // Failing to persist is no reason not to collapse.
    }
  };
  const alleZu = alleMarken().every((m) => zu.has(m));

  const leisteUmschalten = () =>
    setVersteckt((v) => {
      try {
        localStorage.setItem(VERSTECKT_SCHLUESSEL, v ? "nein" : "ja");
      } catch {
        // s.o.
      }
      return !v;
    });

  // Drag from the right edge. During drag the handle captures the pointer so
  // a quick movement that leaves the sidebar still arrives; text selection is
  // disabled to prevent half the sidebar from being highlighted while dragging.
  const zugBeginnen = (e: React.PointerEvent) => {
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    zug.current = { startX: e.clientX, startBreite: breite };
    document.body.style.userSelect = "none";
  };
  const zugBewegen = (e: React.PointerEvent) => {
    const z = zug.current;
    if (!z) return;
    const neu = Math.min(BREITE_MAX, Math.max(BREITE_MIN, z.startBreite + (e.clientX - z.startX)));
    setBreite(neu);
  };
  const zugBeenden = (e: React.PointerEvent) => {
    if (!zug.current) return;
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
    zug.current = null;
    document.body.style.userSelect = "";
    try {
      localStorage.setItem(BREITE_SCHLUESSEL, String(breite));
    } catch {
      // Nicht merken zu koennen ist kein Grund, nicht zu ziehen.
    }
  };
  // Double-clicking the handle resets the width. If someone has accidentally
  // resized the sidebar this restores the default without needing to know it.
  const zugZuruecksetzen = () => {
    setBreite(BREITE_VORGABE);
    try {
      localStorage.setItem(BREITE_SCHLUESSEL, String(BREITE_VORGABE));
    } catch {
      // s.o.
    }
  };

  // Ctrl + backslash toggles the sidebar visibility. Not Ctrl+B because that
  // is bold in the editor; double-binding a shortcut leads to surprising
  // behavior.
  useEffect(() => {
    const auf = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "\\") {
        e.preventDefault();
        leisteUmschalten();
      }
    };
    window.addEventListener("keydown", auf);
    return () => window.removeEventListener("keydown", auf);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
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

  const [rechteFuer, setRechteFuer] = useState<{ id: string; name: string } | null>(null);
  // Id of the space whose visibility menu is open, at most one at a time, hence
  // a single value and not a set.
  const [sichtbarkeitFuer, setSichtbarkeitFuer] = useState<string | null>(null);
  // Dasselbe für das Farbmenü. Zwei getrennte Werte und nicht einer mit einer
  // Art dazu: die beiden Menüs schließen einander aus, und das steht so
  // deutlicher da, als wenn ein Feld beides bedeuten müsste.
  const [farbeFuer, setFarbeFuer] = useState<string | null>(null);
  // The same for the export menu of a space.
  // The two menus in the toolbar. `exportFuer` holds the selected space so
  // the same control can switch to showing export formats instead of a list.
  const [einfuhrOffen, setEinfuhrOffen] = useState(false);
  const [ausfuhrOffen, setAusfuhrOffen] = useState(false);
  const [exportFuer, setExportFuer] = useState<string | null>(null);
  // Clicking outside or pressing Escape closes these menus. Without this
  // they'd remain over the page tree and intercept clicks.
  // Only spaces that actually contain pages are exportable. Requesting an
  // empty space previously led to a raw server error page because the app
  // navigated away; pages without a space are offered as a separate entry.
  const ausgebbar = spaces.filter((sp) => pages.some((p) => (p.spaceId ?? null) === sp.id));
  const ohneAblage = pages.some((p) => (p.spaceId ?? null) === null);

  const einfuhrBereich = useAussenklick<HTMLDivElement>(einfuhrOffen, () => setEinfuhrOffen(false));
  const ausfuhrBereich = useAussenklick<HTMLDivElement>(ausfuhrOffen, () => {
    setAusfuhrOffen(false);
    setExportFuer(null);
  });
  // Target for the import while its panel is open.
  const [einfuhrZiel, setEinfuhrZiel] = useState<
    { ziel: { parentId?: string; spaceId?: string }; name: string } | null
  >(null);
  // Long lists are cut to four entries. A sidebar filled by fifteen spaces and
  // thirty tags is no longer an overview; one scrolls past everything one is
  // looking for. The rest is one click away and the number beside it says how
  // much is waiting there.
  const [alleSpaces, setAlleSpaces] = useState(false);
  const [alleTags, setAlleTags] = useState(false);
  const [results, setResults] = useState<SearchHit[] | null>(null);
  // Search filters. Kept separate from the query so a chosen filter remains
  // while the user continues typing; narrow once and search repeatedly.
  const [filter, setFilter] = useState<SuchFilter>({});
  const [filterOffen, setFilterOffen] = useState(false);
  const suchfeld = useRef<HTMLInputElement>(null);

  // Clear the search: query, filter, and the filter UI together. Deleting
  // only the query would silently limit the next search while hiding the
  // filter controls.
  const suchtLoeschen = () => {
    setQ("");
    setFilter({});
    setFilterOffen(false);
    suchfeld.current?.focus();
  };
  const filterAktiv = Boolean(filter.space || filter.tag || filter.tage || filter.wer);
  const [dragId, setDragId] = useState<string | null>(null);
  // One drop target for three kinds of destination, encoded as a string: a bare
  // page id, "space:<id>" for a space header, or "root" for the ungrouped
  // section. A single value guarantees only one target can be highlighted.
  const [dropTarget, setDropTarget] = useState<string | null>(null);
  // The gap a page is hovering over. Kept apart from dropTarget on purpose: a
  // gap and a row mean two different things -- put between, or hang below -- and
  // one value for both would light up the wrong one of the two.
  const [luecke, setLuecke] = useState<TreeGap | null>(null);
  // The space being dragged, and the one it is hovering over. Spaces are moved
  // in the same gesture as pages, but they are a list, not a tree, so they need
  // no target of their own beyond "in front of this one".
  const [spaceDrag, setSpaceDrag] = useState<string | null>(null);
  const [spaceZiel, setSpaceZiel] = useState<string | null>(null);

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
  // Dropping into a gap: same page, but the place among the siblings is named
  // as well. A gap inside one's own subtree is refused for the same reason a
  // row there is.
  const dropInLuecke = (ziel: TreeGap) => {
    if (dragId && (ziel.elternId === null || canDropOnPage(ziel.elternId))) {
      onOrdnePage(dragId, ziel);
    }
    setDragId(null);
    setDropTarget(null);
    setLuecke(null);
  };

  // Spaces: the whole list is sent, in the order it stands in afterwards. The
  // sidebar knows that order anyway, and a complete list needs no arithmetic
  // over neighbours on either side.
  const dropSpaceVor = (vorId: string | null) => {
    const gezogen = spaceDrag;
    setSpaceDrag(null);
    setSpaceZiel(null);
    if (!gezogen) return;
    const ohne = spaces.filter((sp) => sp.id !== gezogen).map((sp) => sp.id);
    const stelle = vorId === null ? ohne.length : ohne.indexOf(vorId);
    if (stelle < 0) return;
    onOrdneSpaces([...ohne.slice(0, stelle), gezogen, ...ohne.slice(stelle)]);
  };

  const toggle = (id: string) =>
    setExpanded((prev) => {
      const n = new Set(prev);
      n.has(id) ? n.delete(id) : n.add(id);
      return n;
    });

  // Automatically expand the path to the active page.
  //
  // Without this a page could be hidden under collapsed ancestors when it was
  // reached not via the tree — for example via search, a link, or after
  // creating a child. The page would be created and opened but remain hidden
  // in the tree, which looks like nothing happened.
  //
  // Only expand ancestors; never auto-collapse something the user opened by
  // hand.
  useEffect(() => {
    if (!activeId) return;
    const eltern = new Map(pages.map((p) => [p.id, p.parentId ?? null]));
    const weg: string[] = [];
    let lauf = eltern.get(activeId) ?? null;
    // Die Grenze fängt einen Kreis ab. Der sollte nicht entstehen, das Backend
    // weist das Umhängen unter die eigene Unterseite ab; eine Endlosschleife in
    // der Seitenleiste wäre aber ein eingefrorener Browser und nicht bloß ein
    // falscher Baum.
    for (let i = 0; lauf && i < 100; i++) {
      weg.push(lauf);
      lauf = eltern.get(lauf) ?? null;
    }
    if (weg.length === 0) return;
    setExpanded((prev) => {
      if (weg.every((id) => prev.has(id))) return prev;
      const n = new Set(prev);
      weg.forEach((id) => n.add(id));
      return n;
    });
  }, [activeId, pages]);

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
    luecke,
    onDragOverLuecke: (l: TreeGap) => {
      // A gap lights up instead of a row, never both.
      setDropTarget(null);
      setLuecke(l);
    },
    onDragLeaveLuecke: () => setLuecke(null),
    onDropLuecke: (l: TreeGap) => dropInLuecke(l),
  };

  // Tucked-away sidebar: a slim strip that only serves to pull it back out.
  // It must not disappear completely — otherwise the only way back would be
  // a keyboard shortcut that only someone who read the docs knows.
  if (versteckt) {
    return (
      <div className="sidebar schmal">
        <button
          className="icon-btn leiste-griff"
          title="Leiste einblenden (Strg + \)"
          aria-label="Leiste einblenden"
          onClick={leisteUmschalten}
        >
          »
        </button>
      </div>
    );
  }

  return (
    <div className="sidebar" style={{ width: breite, minWidth: breite }}>
        {/* The drag handle sits on the edge, not beside it: the edge is the
          spot the user reaches for. */}
      <div
        className="leiste-kante"
        title="Breite ziehen, Doppelklick setzt zurück"
        onPointerDown={zugBeginnen}
        onPointerMove={zugBewegen}
        onPointerUp={zugBeenden}
        onPointerCancel={zugBeenden}
        onDoubleClick={zugZuruecksetzen}
      />
      <div className="sidebar-header">
        <span className="brand">Nexora</span>
        {/* Placed on the far right because it affects the entire sidebar and
          not its contents. */}
        <button
          className="icon-btn leiste-griff"
          title="Leiste ausblenden (Strg + \)"
          aria-label="Leiste ausblenden"
          onClick={leisteUmschalten}
        >
          «
        </button>
      </div>

      <div className="search-box">
        <input
          ref={suchfeld}
          className={q !== "" ? "hat-knoepfe" : ""}
          placeholder="Suchen…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          // Pressing Escape clears the search without needing to click the
          // cross button. Searchers typically have their hands on the keyboard.
          onKeyDown={(e) => {
            if (e.key === "Escape" && q !== "") {
              e.preventDefault();
              suchtLoeschen();
            }
          }}
        />
        {/* Both buttons appear only while the field contains text: without a
          query there is nothing to clear and nothing to filter by. */}
        {q !== "" && (
          <div className="such-knoepfe">
            <button className="icon-btn" title="Suche leeren (Esc)" aria-label="Suche leeren" onClick={suchtLoeschen}>
              ✕
            </button>
            {/* The dot beside it indicates that a filter is active — otherwise
              one could be puzzled by unexpectedly few results. */}
            <button
              className={"icon-btn" + (filterAktiv ? " aktiv" : "")}
              title={filterAktiv ? "Filter (aktiv)" : "Treffer eingrenzen"}
              onClick={() => setFilterOffen((v) => !v)}
            >
              ⚙
            </button>
          </div>
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
            {/* Everything that creates or exports content is grouped on this
              single line: new page, new space, import, export. Previously the
              new-page action lived as an unlabeled plus in the header — an
              icon without a word, far from the three other actions that do
              the same. */}
          <button className="text-btn" title="Neue Seite ganz oben anlegen" onClick={() => onCreateRoot()}>
            + Seite
          </button>
          <button className="text-btn" title="Neue Ablage anlegen" onClick={onCreateSpace}>
            + Ablage
          </button>
          {/* Labelled instead of an arrow: an icon alone does not say that a
              whole archive can be read in here, and since the import can create
              a space of its own, this is the way back out of an export. */}
            {/* The import first asks where to put the content. Previously each
              space had its own arrow; the question is the same but is now
              asked once instead of on every heading. */}
          <div className="klappmenue" ref={einfuhrBereich}>
            <button
              className="text-btn"
              title="Markdown, HTML oder ein ZIP importieren, wahlweise als eigene Ablage"
              onClick={() => {
                setAusfuhrOffen(false);
                setEinfuhrOffen((v) => !v);
              }}
            >
              Import
            </button>
            {einfuhrOffen && (
              <div className="klappliste" onMouseLeave={() => setEinfuhrOffen(false)}>
                <div className="klappliste-titel">Wohin importieren</div>
                <button
                  className="klappeintrag"
                  onClick={() => {
                    setEinfuhrOffen(false);
                    setEinfuhrZiel({ ziel: {}, name: "Seiten" });
                  }}
                >
                  Als neue Ablage oder ohne
                </button>
                {spaces.map((sp) => (
                  <button
                    key={sp.id}
                    className="klappeintrag"
                    onClick={() => {
                      setEinfuhrOffen(false);
                      setEinfuhrZiel({ ziel: { spaceId: sp.id }, name: sp.name });
                    }}
                  >
                    In: {sp.name}
                  </button>
                ))}
              </div>
            )}
          </div>
            {/* A normal navigation to the URL, not a fetch: the browser can pick
              up the filename from the Content-Disposition header and stream
              the download directly to disk instead of loading it into memory. */}
          {frei("export") && (ausgebbar.length > 0 || ohneAblage) && (
            <div className="klappmenue" ref={ausfuhrBereich}>
              <button
                className="text-btn"
                title="Eine ganze Ablage exportieren, als ZIP, PDF oder Word"
                onClick={() => {
                  setEinfuhrOffen(false);
                  setExportFuer(null);
                  setAusfuhrOffen((v) => !v);
                }}
              >
                Export
              </button>
              {ausfuhrOffen && (
                <div
                  className="klappliste"
                  onMouseLeave={() => {
                    setAusfuhrOffen(false);
                    setExportFuer(null);
                  }}
                >
                  {exportFuer === null ? (
                    <>
                      <div className="klappliste-titel">Welche Ablage</div>
                      {ausgebbar.map((sp) => (
                        <button
                          key={sp.id}
                          className="klappeintrag"
                          onClick={() => setExportFuer(sp.id)}
                        >
                          {sp.name}
                        </button>
                      ))}
                      {ohneAblage && (
                        <button className="klappeintrag" onClick={() => setExportFuer("ohne")}>
                          Seiten ohne Ablage
                        </button>
                      )}
                    </>
                  ) : (
                    <>
                      <div className="klappliste-titel">
                        {exportFuer === "ohne"
                          ? "Seiten ohne Ablage"
                          : (spaces.find((sp) => sp.id === exportFuer)?.name ?? "Ablage")}{" "}
                        exportieren
                      </div>
                      {(
                        [
                          ["", "Markdown-Dateien (.zip)"],
                          ["pdf", "Ein PDF mit allen Seiten"],
                          ["word", "Ein Word-Dokument"],
                        ] as const
                      ).map(([form, titel]) => (
                        <button
                          key={titel}
                          className="klappeintrag"
                          onClick={() => {
                            const id = exportFuer;
                            setAusfuhrOffen(false);
                            setExportFuer(null);
                            window.location.href =
                              `/api/spaces/${id}/export` + (form ? `?format=${form}` : "");
                          }}
                        >
                          {titel}
                        </button>
                      ))}
                      <button className="klappeintrag" onClick={() => setExportFuer(null)}>
                        ← Andere Ablage
                      </button>
                    </>
                  )}
                </div>
              )}
            </div>
          )}
            {/* Collapsing every space individually would be a dozen clicks with a
              dozen spaces. This button does it in one. */}
          <button
            className="text-btn klapp-alles"
            title={alleZu ? "Alle Abschnitte wieder aufklappen" : "Alle Abschnitte zuklappen"}
            onClick={alleKlappen}
          >
            {alleZu ? "▾ Ausklappen" : "▸ Einklappen"}
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
                    className={
                      "sidebar-section-title" +
                      (dropTarget === marke ? " drop-target" : "") +
                      (spaceZiel === sp.id ? " space-davor" : "") +
                      (spaceDrag === sp.id ? " dragging" : "")
                    }
                    // The heading is the handle for the space itself. Draggable
                    // only when no page is in flight, otherwise the browser
                    // would start a second drag out of the first.
                    draggable={!dragId}
                    onDragStart={(e) => {
                      e.dataTransfer.setData("text/plain", "space:" + sp.id);
                      e.dataTransfer.effectAllowed = "move";
                      setSpaceDrag(sp.id);
                    }}
                    onDragEnd={() => {
                      setSpaceDrag(null);
                      setSpaceZiel(null);
                    }}
                    onDragOver={(e) => {
                      // Two kinds of freight land on this heading: a space,
                      // which sorts itself in front of this one, and a page,
                      // which moves into it.
                      if (spaceDrag) {
                        if (spaceDrag === sp.id) return;
                        e.preventDefault();
                        e.dataTransfer.dropEffect = "move";
                        setSpaceZiel(sp.id);
                        return;
                      }
                      if (!dragId) return;
                      e.preventDefault();
                      setDropTarget(marke);
                      // A collapsed space opens as soon as one hovers over it
                      // with a page: otherwise one drags onto a target whose
                      // content one cannot see.
                      aufklappen(marke);
                    }}
                    onDragLeave={() => {
                      setDropTarget((t) => (t === marke ? null : t));
                      setSpaceZiel((z) => (z === sp.id ? null : z));
                    }}
                    onDrop={(e) => {
                      e.preventDefault();
                      if (spaceDrag) {
                        dropSpaceVor(sp.id);
                        return;
                      }
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
                    {/* The count is shown only while collapsed: when expanded you
                      can already see the pages. */}
                    {eingeklappt && spacePages.length > 0 && (
                      <span className="tag-anzahl muted small">{spacePages.length}</span>
                    )}
                    {/* Without a fixed display these controls would follow the
                      same rule as the lines below: they appear on hover.
                      Otherwise every space would show a row of controls one
                      rarely needs. */}
                    <span className="tree-actions">
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
                          onClick={() => {
                            setFarbeFuer(null);
                            setSichtbarkeitFuer((v) => (v === sp.id ? null : sp.id));
                          }}
                        >
                          ◎
                        </button>
                      )}
                      {sp.darfVerwalten && (
                        <button
                          className="icon-btn"
                          title="Farbe dieser Ablage"
                          onClick={() => {
                            setSichtbarkeitFuer(null);
                            setFarbeFuer((v) => (v === sp.id ? null : sp.id));
                          }}
                        >
                          ◐
                        </button>
                      )}
                      {frei("gruppen") && sp.darfVerwalten && (
                        <button
                          className="icon-btn"
                          title="Rechte an dieser Ablage"
                          onClick={() => setRechteFuer({ id: sp.id, name: sp.name })}
                        >
                          ⚿
                        </button>
                      )}
                        {/* Import and export used to live here as extra icons on every
                          heading. They now sit in the top-left tool row and
                          ask for the target space there — see the tool section. */}
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

                  {/* Die Farbtafel. Eine feste Auswahl statt eines freien
                      Farbwählers: die zehn Töne sind gegeneinander
                      unterscheidbar und auf beiden Grundtönen lesbar, und wer
                      frei wählen darf, trifft irgendwann zweimal fast dasselbe
                      Grau. Wer es doch genau wissen will, findet den freien
                      Wähler am Punkt in der Legende des Grafen. */}
                  {farbeFuer === sp.id && (
                    <div className="sichtbarkeit-menue farbmenue">
                      <div className="sichtbarkeit-kopf">Farbe dieser Ablage</div>
                      <div className="farbmenue-tafel">
                        {ABLAGE_FARBEN.map((f) => (
                          <button
                            key={f}
                            className={"farbmenue-feld" + (sp.farbe === f ? " gewaehlt" : "")}
                            style={{ background: f }}
                            title={f}
                            aria-label={"Farbe " + f}
                            aria-pressed={sp.farbe === f}
                            onClick={() => {
                              onSpaceFarbe(sp.id, f);
                              setFarbeFuer(null);
                            }}
                          />
                        ))}
                      </div>
                      {/* Zurücksetzen und nicht "keine Farbe": ohne eigene Wahl
                          bekommt die Ablage eine aus der Reihe, sie ist also
                          nie farblos. */}
                      <button
                        className="sichtbarkeit-eintrag"
                        onClick={() => {
                          onSpaceFarbe(sp.id, "");
                          setFarbeFuer(null);
                        }}
                      >
                        <span className="sichtbarkeit-text">
                          <span className="sichtbarkeit-titel">Zurücksetzen</span>
                          <span className="sichtbarkeit-erklaerung">
                            Wieder die Farbe, die sich aus der Ablage selbst ergibt
                          </span>
                        </span>
                        <span className="sichtbarkeit-haken" aria-hidden="true">
                          {sp.farbe ? "" : "✓"}
                        </span>
                      </button>
                      <div className="sichtbarkeit-hinweis muted small">
                        Zu sehen ist die Farbe im Grafen: dort trägt jede Seite die Farbe
                        ihrer Ablage. In der Leiste bleibt es beim Namen.
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
                      spaceId={sp.id}
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
            {(ungrouped.length > 0 || dragId || spaceDrag) && (
            <div className="sidebar-section">
              <div
                className={
                  "sidebar-section-title" +
                  (dropTarget === "root" ? " drop-target" : "") +
                  (spaceZiel === "ende" ? " space-davor" : "")
                }
                onDragOver={(e) => {
                  // This heading stands below every space, so a space dropped
                  // here goes to the end of the list -- the one place no other
                  // heading can stand for.
                  if (spaceDrag) {
                    e.preventDefault();
                    e.dataTransfer.dropEffect = "move";
                    setSpaceZiel("ende");
                    return;
                  }
                  if (!dragId) return;
                  e.preventDefault();
                  setDropTarget("root");
                  aufklappen("root");
                }}
                onDragLeave={() => {
                  setDropTarget((t) => (t === "root" ? null : t));
                  setSpaceZiel((z) => (z === "ende" ? null : z));
                }}
                onDrop={(e) => {
                  e.preventDefault();
                  if (spaceDrag) {
                    dropSpaceVor(null);
                    return;
                  }
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
                  spaceId={null}
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
                  Ablage importieren
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
              {!zu.has("workspace") && (
                <>
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
                    <span className="tree-label">Graf</span>
                  </div>
                  <div
                    className={"tree-row" + (currentPath === "/trash" ? " active" : "")}
                    onClick={() => onNavigate("/trash")}
                  >
                    <span className="tree-label">Papierkorb</span>
                  </div>
                </>
              )}
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
                  symbolTitel="Verwaltung"
                  symbolAktion={() => onNavigate("/einstellungen")}
                  symbolAktiv={currentPath.startsWith("/einstellungen")}
                  nameFolgtSymbol
                >
                  Verwaltung
                </Klapptitel>
                {!zu.has("verwaltung") && frei("pruefspur") && (
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

      <div className="sidebar-footer" ref={kontoEcke}>
        <button
          className="konto-knopf"
          onClick={() => setKontoMenue((auf) => !auf)}
          aria-haspopup="menu"
          aria-expanded={kontoMenue}
          title={user?.email}
        >
          <Profilbild
            id={user?.id ?? ""}
            name={user?.name ?? ""}
            email={user?.email}
            stand={user?.bildStand}
          />
          <span className="konto-name">{user?.name}</span>
          <span className="konto-pfeil" aria-hidden="true">
            {kontoMenue ? "\u25be" : "\u25b4"}
          </span>
        </button>

        {kontoMenue && (
          <div className="konto-menue" role="menu">
            <div className="konto-menue-kopf">
              <Profilbild
                id={user?.id ?? ""}
                name={user?.name ?? ""}
                email={user?.email}
                stand={user?.bildStand}
                groesse={40}
              />
              <div>
                <strong>{user?.name}</strong>
                <div className="muted small">{user?.email}</div>
              </div>
            </div>
            {/* Drei gleiche Knöpfe untereinander: Profil, Passwort, Abmelden
                sind derselbe Rang, und einer davon sah bisher aus wie ein
                Nebensatz. */}
            <button
              className="konto-menue-zeile"
              role="menuitem"
              onClick={() => {
                setKontoMenue(false);
                setProfilOffen(true);
              }}
            >
              Profil bearbeiten
            </button>
            <button
              className="konto-menue-zeile"
              role="menuitem"
              onClick={() => {
                setKontoMenue(false);
                setPasswortOffen(true);
              }}
            >
              Passwort ändern
            </button>
            <button
              className="konto-menue-zeile"
              role="menuitem"
              onClick={() => {
                setKontoMenue(false);
                logout();
              }}
            >
              Abmelden
            </button>
          </div>
        )}
      </div>

      {passwortOffen && <PasswortDialog onClose={() => setPasswortOffen(false)} />}
      {profilOffen && <ProfilDialog onClose={() => setProfilOffen(false)} />}
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
  nameFolgtSymbol,
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
  // Lets the name do what the symbol does, instead of folding. Where a heading
  // leads somewhere, the word beside the icon is the first thing one clicks --
  // and folding is then still one click away, on the triangle.
  nameFolgtSymbol?: boolean;
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
      <span
        className={"klapp-name" + (nameFolgtSymbol && symbolAktion ? " klapp-name-fuehrt" : "")}
        title={nameFolgtSymbol && symbolAktion ? symbolTitel : undefined}
        onClick={() => (nameFolgtSymbol && symbolAktion ? symbolAktion() : klappen(marke))}
      >
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
