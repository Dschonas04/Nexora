import { useCallback, useEffect, useState } from "react";
import { Routes, Route, useNavigate, useLocation } from "react-router-dom";
import { PageMeta, Space, Tag, api } from "../api/client";
import Sidebar from "../components/Sidebar";
import PageView from "./PageView";
import TrashView from "./TrashView";
import GraphView from "./GraphView";
import AdminView from "./AdminView";

export default function Workspace() {
  const nav = useNavigate();
  const loc = useLocation();
  const [pages, setPages] = useState<PageMeta[]>([]);
  const [shared, setShared] = useState<PageMeta[]>([]);
  const [favorites, setFavorites] = useState<PageMeta[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [spaces, setSpaces] = useState<Space[]>([]);

  const refreshPages = useCallback(() => api.listPages().then(setPages).catch(() => {}), []);
  const refreshShared = useCallback(() => api.listShared().then(setShared).catch(() => {}), []);
  const refreshFav = useCallback(() => api.listFavorites().then(setFavorites).catch(() => {}), []);
  const refreshTags = useCallback(() => api.listTags().then(setTags).catch(() => {}), []);
  const refreshSpaces = useCallback(() => api.listSpaces().then(setSpaces).catch(() => {}), []);

  useEffect(() => {
    refreshPages();
    refreshShared();
    refreshFav();
    refreshTags();
    refreshSpaces();
  }, [refreshPages, refreshShared, refreshFav, refreshTags, refreshSpaces]);

  const createPage = async (parentId: string | null, spaceId: string | null = null) => {
    const p = await api.createPage(parentId, spaceId);
    await refreshPages();
    nav(`/page/${p.id}`);
  };

  const deletePage = async (id: string) => {
    if (!confirm("Move this page and its subpages to the trash?")) return;
    await api.deletePage(id);
    await refreshPages();
    await refreshFav();
    nav("/");
  };

  const createSpace = async () => {
    const name = prompt("Space name:")?.trim();
    if (!name) return;
    await api.createSpace(name);
    refreshSpaces();
  };

  const renameSpace = async (id: string, current: string) => {
    const name = prompt("Rename space:", current)?.trim();
    if (!name) return;
    await api.renameSpace(id, name);
    refreshSpaces();
  };

  const deleteSpace = async (id: string) => {
    if (!confirm("Delete this space? Its pages are kept and become ungrouped.")) return;
    await api.deleteSpace(id);
    await refreshSpaces();
    await refreshPages();
  };

  const match = loc.pathname.match(/^\/page\/(.+)$/);
  const activeId = match ? match[1] : undefined;

  return (
    <div className="app">
      <Sidebar
        pages={pages}
        shared={shared}
        favorites={favorites}
        tags={tags}
        spaces={spaces}
        activeId={activeId}
        onSelect={(id) => nav(`/page/${id}`)}
        onCreateRoot={() => createPage(null)}
        onCreateChild={(pid) => createPage(pid)}
        onCreateInSpace={(sid) => createPage(null, sid)}
        onDelete={deletePage}
        onCreateSpace={createSpace}
        onRenameSpace={renameSpace}
        onDeleteSpace={deleteSpace}
        onNavigate={(to) => nav(to)}
        currentPath={loc.pathname}
      />
      <div className="main">
        <Routes>
          <Route index element={<EmptyState onCreate={() => createPage(null)} />} />
          <Route
            path="page/:id"
            element={
              <PageView
                allTags={tags}
                onMetaChange={() => {
                  refreshPages();
                  refreshShared();
                }}
                onFavChange={refreshFav}
                onTagsChange={refreshTags}
                onDelete={deletePage}
              />
            }
          />
          <Route
            path="trash"
            element={
              <TrashView
                onChange={() => {
                  refreshPages();
                  refreshFav();
                }}
              />
            }
          />
          <Route path="graph" element={<GraphView />} />
          <Route path="admin" element={<AdminView />} />
        </Routes>
      </div>
    </div>
  );
}

function EmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="empty-state">
      <div>Nothing open yet.</div>
      <button className="btn btn-primary" onClick={onCreate}>
        Create a page
      </button>
    </div>
  );
}
