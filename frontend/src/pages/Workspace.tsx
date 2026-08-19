// The signed-in shell: sidebar on the left, routed view on the right.
//
// All shared lists (pages, shares, favorites, tags, spaces) are held here rather
// than in the sidebar, because the views on the right change them and the
// sidebar has to follow. The refresh callbacks passed down are how a view says
// "something you display has changed".
import { useCallback, useEffect, useState } from "react";
import { Routes, Route, useNavigate, useLocation } from "react-router-dom";
import { PageMeta, Space, Tag, api } from "../api/client";
import Sidebar from "../components/Sidebar";
import PageView from "./PageView";
import TrashView from "./TrashView";
import GraphView from "./GraphView";
import AdminView from "./AdminView";
import PruefspurView from "./PruefspurView";
import EinstellungenView from "./EinstellungenView";
import TagView from "./TagView";

export default function Workspace() {
  const nav = useNavigate();
  const loc = useLocation();
  const [pages, setPages] = useState<PageMeta[]>([]);
  const [shared, setShared] = useState<PageMeta[]>([]);
  const [favorites, setFavorites] = useState<PageMeta[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [spaces, setSpaces] = useState<Space[]>([]);

  // useCallback keeps these stable, so the effect below runs once on mount
  // instead of on every render. A failed refresh is swallowed and simply leaves
  // the previous list in place, which beats emptying the sidebar on a hiccup.
  const refreshPages = useCallback(() => api.listPages().then(setPages).catch(() => {}), []);
  // Sharing is a paid extra: without a license the call answers 402. The empty
  // list that follows is the wanted outcome -- the sidebar hides its "shared"
  // section when there is nothing in it, so the interface stays coherent
  // instead of showing an error for a feature this installation does not have.
  const refreshShared = useCallback(() => api.listShared().then(setShared).catch(() => setShared([])), []);
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

  // Navigate to the new page only after the tree has been refreshed, otherwise
  // the sidebar would briefly show no entry for the page now open.
  const createPage = async (
    parentId: string | null,
    spaceId: string | null = null,
    vorlageId?: string,
  ) => {
    const p = await api.createPage(parentId, spaceId, vorlageId);
    await refreshPages();
    nav(`/page/${p.id}`);
  };

  // Deleting moves the page to the trash rather than removing it. Favorites are
  // refreshed too, since the page may have been pinned.
  const deletePage = async (id: string) => {
    if (!confirm("Diese Seite und ihre Unterseiten in den Papierkorb verschieben?")) return;
    await api.deletePage(id);
    await refreshPages();
    await refreshFav();
    nav("/");
  };

  const createSpace = async () => {
    const name = prompt("Space-Name:")?.trim();
    if (!name) return;
    await api.createSpace(name);
    refreshSpaces();
  };

  const renameSpace = async (id: string, current: string) => {
    const name = prompt("Space umbenennen:", current)?.trim();
    if (!name) return;
    await api.renameSpace(id, name);
    refreshSpaces();
  };

  // The pages survive and become ungrouped, which the confirmation spells out
  // so nobody expects a space to take its content with it.
  const deleteSpace = async (id: string) => {
    if (!confirm("Diesen Space löschen? Seine Seiten bleiben erhalten und werden gruppenlos.")) return;
    await api.deleteSpace(id);
    await refreshSpaces();
    await refreshPages();
  };

  // Reparent or move a page after a sidebar drag. Both are one update, since a
  // page dropped into a different space usually changes its parent as well.
  // parentId null means top level, spaceId null means no space.
  const movePage = async (id: string, parentId: string | null, spaceId: string | null) => {
    await api.updatePage(id, { parentId, spaceId });
    await refreshPages();
  };

  // Read the open page from the URL instead of tracking it in state, so the
  // sidebar highlight stays right after a back or forward navigation.
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
        onCreateRoot={(vorlageId?: string) => createPage(null, null, vorlageId)}
        onCreateChild={(pid) => createPage(pid)}
        onCreateInSpace={(sid) => createPage(null, sid)}
        onDelete={deletePage}
        onCreateSpace={createSpace}
        onRenameSpace={renameSpace}
        onDeleteSpace={deleteSpace}
        onMovePage={movePage}
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
          <Route path="pruefspur" element={<PruefspurView />} />
          <Route path="einstellungen" element={<EinstellungenView />} />
          <Route
            path="tag/:tagId"
            element={<TagView allTags={tags} onTagsChange={refreshTags} />}
          />
        </Routes>
      </div>
    </div>
  );
}

function EmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="empty-state">
      <div>Noch nichts geöffnet.</div>
      <button className="btn btn-primary" onClick={onCreate}>
        Seite erstellen
      </button>
    </div>
  );
}
