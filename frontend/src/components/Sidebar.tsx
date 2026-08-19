// The sidebar: search, favorites, spaces, the page tree, shared pages, tags and
// the workspace links. It owns no data of its own, everything arrives as props
// from Workspace; what it does own is view state such as which branches are
// open and what is currently being dragged.
import { useEffect, useState } from "react";
import { PageMeta, SearchHit, Space, Tag, api } from "../api/client";
import { useLizenz } from "../lizenz";
import { useAuth } from "../auth";
import PageTree from "./PageTree";

interface Props {
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
  onMovePage: (id: string, parentId: string | null, spaceId: string | null) => void;
  onNavigate: (to: string) => void;
  currentPath: string;
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
    onMovePage,
    onNavigate,
    currentPath,
  } = props;
  const { user, logout } = useAuth();
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [q, setQ] = useState("");
  const { frei } = useLizenz();
  const [results, setResults] = useState<SearchHit[] | null>(null);
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
      api.search(q).then(setResults).catch(() => setResults([]));
    }, 250);
    return () => clearTimeout(t);
  }, [q]);

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
      {h.ausschnitt.trim() !== "" && (
        <div className="treffer-ausschnitt">{markiere(h.ausschnitt)}</div>
      )}
    </div>
  );

  // Pages outside every space. They get their own section below the spaces, so
  // no page can become invisible by not belonging anywhere.
  const ungrouped = pages.filter((p) => !p.spaceId);

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
        <button className="icon-btn" title="Neue Seite" onClick={onCreateRoot}>
          +
        </button>
      </div>

      <div className="search-box">
        <input placeholder="Suchen…" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>

      <div className="sidebar-scroll">
        {results !== null ? (
          <div className="sidebar-section">
            <div className="sidebar-section-title">Ergebnisse</div>
            {results.length === 0 && <div className="tree-row muted">Keine Treffer</div>}
            {results.map(trefferRow)}
          </div>
        ) : (
          <>
            {favorites.length > 0 && (
              <div className="sidebar-section">
                <div className="sidebar-section-title">Favoriten</div>
                {favorites.map(flatRow)}
              </div>
            )}

            {/* One section per space, each with its own tree. Passing only that
                space's pages means the tree component never has to know spaces
                exist. */}
            {spaces.map((sp) => {
              const spacePages = pages.filter((p) => p.spaceId === sp.id);
              return (
                <div className="sidebar-section" key={sp.id}>
                  <div
                    className={"sidebar-section-title" + (dropTarget === "space:" + sp.id ? " drop-target" : "")}
                    onDragOver={(e) => {
                      if (!dragId) return;
                      e.preventDefault();
                      setDropTarget("space:" + sp.id);
                    }}
                    onDragLeave={() => setDropTarget((t) => (t === "space:" + sp.id ? null : t))}
                    onDrop={(e) => {
                      e.preventDefault();
                      dropOnSpace(sp.id);
                    }}
                  >
                    <span onClick={() => onRenameSpace(sp.id, sp.name)} style={{ cursor: "pointer" }}>
                      {sp.name}
                    </span>
                    <span className="tree-actions" style={{ display: "flex" }}>
                      <button className="icon-btn" title="Neue Seite" onClick={() => onCreateInSpace(sp.id)}>
                        +
                      </button>
                      <button className="icon-btn" title="Space löschen" onClick={() => onDeleteSpace(sp.id)}>
                        ✕
                      </button>
                    </span>
                  </div>
                  {/* An empty space still needs a drop area, otherwise there
                      would be no way to drag the first page into it. */}
                  {spacePages.length === 0 ? (
                    <div
                      className={"tree-row muted" + (dropTarget === "space:" + sp.id ? " drop-target" : "")}
                      onDragOver={(e) => {
                        if (!dragId) return;
                        e.preventDefault();
                        setDropTarget("space:" + sp.id);
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

            <div className="sidebar-section">
              <div
                className={"sidebar-section-title" + (dropTarget === "root" ? " drop-target" : "")}
                onDragOver={(e) => {
                  if (!dragId) return;
                  e.preventDefault();
                  setDropTarget("root");
                }}
                onDragLeave={() => setDropTarget((t) => (t === "root" ? null : t))}
                onDrop={(e) => {
                  e.preventDefault();
                  dropOnSpace(null);
                }}
              >
                Seiten
                <span className="tree-actions" style={{ display: "flex", gap: 8 }}>
                  <button className="text-btn" title="Neuer Space" onClick={onCreateSpace}>
                    + Space
                  </button>
                  <button className="icon-btn" title="Neue Seite" onClick={onCreateRoot}>
                    +
                  </button>
                </span>
              </div>
              {ungrouped.length === 0 ? (
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
                  Noch keine Seiten
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

            {shared.length > 0 && (
              <div className="sidebar-section">
                <div className="sidebar-section-title">Mit mir geteilt</div>
                {shared.map(flatRow)}
              </div>
            )}

            {tags.length > 0 && (
              <div className="sidebar-section">
                <div className="sidebar-section-title">Schlagwörter</div>
                {tags.map((t) => (
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
              </div>
            )}

            <div className="sidebar-section">
              <div className="sidebar-section-title">Workspace</div>
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
              {/* Admin view is hidden for everyone else. This only tidies the
                  UI: the backend checks the role again on every call. */}
              {user?.role === "admin" && (
                <div
                  className={"tree-row" + (currentPath === "/admin" ? " active" : "")}
                  onClick={() => onNavigate("/admin")}
                >
                  <span className="tree-label">Nutzer &amp; Rollen</span>
                </div>
              )}
              {/* Die Prüfspur ist Admin UND Zusatzumfang. Beides wird auch im
                  Backend geprüft; hier geht es nur darum, keinen Eintrag
                  anzubieten, der ohnehin abgewiesen würde. */}
              {user?.role === "admin" && (
                <div
                  className={"tree-row" + (currentPath === "/einstellungen" ? " active" : "")}
                  onClick={() => onNavigate("/einstellungen")}
                >
                  <span className="tree-label">Einstellungen</span>
                </div>
              )}
              {user?.role === "admin" && frei("pruefspur") && (
                <div
                  className={"tree-row" + (currentPath === "/pruefspur" ? " active" : "")}
                  onClick={() => onNavigate("/pruefspur")}
                >
                  <span className="tree-label">Prüfspur</span>
                </div>
              )}
            </div>
          </>
        )}
      </div>

      <div className="sidebar-footer">
        <span className="tree-label">{user?.name}</span>
        <button className="btn" onClick={logout}>
          Abmelden
        </button>
      </div>
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
