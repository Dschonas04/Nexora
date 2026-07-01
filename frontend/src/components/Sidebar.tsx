import { useEffect, useState } from "react";
import { PageMeta, Tag, api } from "../api/client";
import { useAuth } from "../auth";
import PageTree from "./PageTree";

interface Props {
  pages: PageMeta[];
  favorites: PageMeta[];
  tags: Tag[];
  activeId?: string;
  onSelect: (id: string) => void;
  onCreateRoot: () => void;
  onCreateChild: (parentId: string) => void;
  onDelete: (id: string) => void;
}

export default function Sidebar(props: Props) {
  const { pages, favorites, tags, activeId, onSelect, onCreateRoot, onCreateChild, onDelete } = props;
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
      <span className="tree-icon">{p.icon || "📄"}</span>
      <span className="tree-label">{p.title || "Untitled"}</span>
    </div>
  );

  return (
    <div className="sidebar">
      <div className="sidebar-header">
        <span className="brand">Nexora</span>
        <button className="icon-btn" title="New page" onClick={onCreateRoot}>
          +
        </button>
      </div>

      <div className="search-box">
        <input placeholder="Search…" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>

      <div className="sidebar-scroll">
        {results !== null ? (
          <div className="sidebar-section">
            <div className="sidebar-section-title">Results</div>
            {results.length === 0 && <div className="tree-row muted">No matches</div>}
            {results.map(flatRow)}
          </div>
        ) : (
          <>
            {favorites.length > 0 && (
              <div className="sidebar-section">
                <div className="sidebar-section-title">Favorites</div>
                {favorites.map(flatRow)}
              </div>
            )}

            <div className="sidebar-section">
              <div className="sidebar-section-title">
                Pages
                <button className="icon-btn" title="New page" onClick={onCreateRoot}>
                  +
                </button>
              </div>
              {pages.length === 0 ? (
                <div className="tree-row muted">No pages yet</div>
              ) : (
                <PageTree
                  pages={pages}
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

            {tags.length > 0 && (
              <div className="sidebar-section">
                <div className="sidebar-section-title">Tags</div>
                {tags.map((t) => (
                  <div key={t.id} className="tree-row">
                    <span className="tree-icon" style={{ color: t.color }}>
                      ●
                    </span>
                    <span className="tree-label">{t.name}</span>
                  </div>
                ))}
              </div>
            )}
          </>
        )}
      </div>

      <div className="sidebar-footer">
        <span className="tree-label">{user?.name}</span>
        <button className="btn" onClick={logout}>
          Sign out
        </button>
      </div>
    </div>
  );
}
