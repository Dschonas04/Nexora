import { useEffect, useState } from "react";
import { PageMeta, Space, Tag, api } from "../api/client";
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
    onNavigate,
    currentPath,
  } = props;
  const { user, logout } = useAuth();
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [q, setQ] = useState("");
  const [results, setResults] = useState<PageMeta[] | null>(null);

  const toggle = (id: string) =>
    setExpanded((prev) => {
      const n = new Set(prev);
      n.has(id) ? n.delete(id) : n.add(id);
      return n;
    });

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

  const flatRow = (p: PageMeta) => (
    <div
      key={p.id}
      className={"tree-row" + (activeId === p.id ? " active" : "")}
      onClick={() => onSelect(p.id)}
    >
      <span className="tree-label">{p.title || "Ohne Titel"}</span>
    </div>
  );

  const ungrouped = pages.filter((p) => !p.spaceId);

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
            {results.map(flatRow)}
          </div>
        ) : (
          <>
            {favorites.length > 0 && (
              <div className="sidebar-section">
                <div className="sidebar-section-title">Favoriten</div>
                {favorites.map(flatRow)}
              </div>
            )}

            {spaces.map((sp) => {
              const spacePages = pages.filter((p) => p.spaceId === sp.id);
              return (
                <div className="sidebar-section" key={sp.id}>
                  <div className="sidebar-section-title">
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
                  {spacePages.length === 0 ? (
                    <div className="tree-row muted">Leer</div>
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
                    />
                  )}
                </div>
              );
            })}

            <div className="sidebar-section">
              <div className="sidebar-section-title">
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
                <div className="tree-row muted">Noch keine Seiten</div>
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
                  <div key={t.id} className="tree-row">
                    <span className="tag-dot" style={{ background: t.color }} />
                    <span className="tree-label">{t.name}</span>
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
              {user?.role === "admin" && (
                <div
                  className={"tree-row" + (currentPath === "/admin" ? " active" : "")}
                  onClick={() => onNavigate("/admin")}
                >
                  <span className="tree-label">Nutzer &amp; Rollen</span>
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
