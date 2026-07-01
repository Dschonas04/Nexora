import { useCallback, useEffect, useState } from "react";
import { Routes, Route, useNavigate, useLocation } from "react-router-dom";
import { PageMeta, Tag, api } from "../api/client";
import Sidebar from "../components/Sidebar";
import PageView from "./PageView";

export default function Workspace() {
  const nav = useNavigate();
  const loc = useLocation();
  const [pages, setPages] = useState<PageMeta[]>([]);
  const [favorites, setFavorites] = useState<PageMeta[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);

  const refreshPages = useCallback(() => api.listPages().then(setPages).catch(() => {}), []);
  const refreshFav = useCallback(() => api.listFavorites().then(setFavorites).catch(() => {}), []);
  const refreshTags = useCallback(() => api.listTags().then(setTags).catch(() => {}), []);

  useEffect(() => {
    refreshPages();
    refreshFav();
    refreshTags();
  }, [refreshPages, refreshFav, refreshTags]);

  const createPage = async (parentId: string | null) => {
    const p = await api.createPage(parentId);
    await refreshPages();
    nav(`/page/${p.id}`);
  };

  const deletePage = async (id: string) => {
    if (!confirm("Delete this page and all its subpages?")) return;
    await api.deletePage(id);
    await refreshPages();
    await refreshFav();
    nav("/");
  };

  const match = loc.pathname.match(/^\/page\/(.+)$/);
  const activeId = match ? match[1] : undefined;

  return (
    <div className="app">
      <Sidebar
        pages={pages}
        favorites={favorites}
        tags={tags}
        activeId={activeId}
        onSelect={(id) => nav(`/page/${id}`)}
        onCreateRoot={() => createPage(null)}
        onCreateChild={(pid) => createPage(pid)}
        onDelete={deletePage}
      />
      <div className="main">
        <Routes>
          <Route index element={<EmptyState onCreate={() => createPage(null)} />} />
          <Route
            path="page/:id"
            element={
              <PageView
                allTags={tags}
                onMetaChange={refreshPages}
                onFavChange={refreshFav}
                onTagsChange={refreshTags}
                onDelete={deletePage}
              />
            }
          />
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
