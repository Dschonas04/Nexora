// The signed-in shell: sidebar on the left, routed view on the right.
//
// All shared lists (pages, shares, favorites, tags, spaces) are held here rather
// than in the sidebar, because the views on the right change them and the
// sidebar has to follow. The refresh callbacks passed down are how a view says
// "something you display has changed".
import { Suspense, lazy, useCallback, useEffect, useState } from "react";
import { Routes, Route, Link, Navigate, useNavigate, useLocation } from "react-router-dom";
import { PageMeta, Space, Tag, api } from "../api/client";
import Sidebar from "../components/Sidebar";
import { TreeGap } from "../components/PageTree";
import PageView from "./PageView";
import TagView from "./TagView";
import { useEingabe, useRueckfrage } from "../components/Rueckfrage";

// Loaded on demand instead of shipped along: these views are rarely opened, but
// their code sat in the same bundle as the page view and had to be transferred
// and parsed on every start of the application. The graph brings its own
// computation along, the settings their long forms.
//
// Whoever opens them waits briefly once for the load, and in exchange the
// application starts faster for everybody else.
const TrashView = lazy(() => import("./TrashView"));
const GraphView = lazy(() => import("./GraphView"));
const EinstellungenView = lazy(() => import("./EinstellungenView"));
const PostfachView = lazy(() => import("./PostfachView"));

export default function Workspace() {
  const nav = useNavigate();
  const loc = useLocation();
  const frage = useRueckfrage();
  const eingabe = useEingabe();
  const [pages, setPages] = useState<PageMeta[]>([]);
  const [shared, setShared] = useState<PageMeta[]>([]);
  const [favorites, setFavorites] = useState<PageMeta[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [spaces, setSpaces] = useState<Space[]>([]);
  // Number of unread messages. It hangs on no view but on the sidebar, and is
  // therefore kept here.
  const [ungelesen, setUngelesen] = useState(0);

  // useCallback keeps these stable, so the effect below runs once on mount
  // instead of on every render. A failed refresh is swallowed and simply leaves
  // the previous list in place, which beats emptying the sidebar on a hiccup.
  const refreshPages = useCallback(() => api.listPages().then(setPages).catch(() => {}), []);
  // Sharing is a paid extra: without a license the call answers 402. The empty
  // list that follows is the wanted outcome, the sidebar hides its "shared"
  // section when there is nothing in it, so the interface stays coherent
  // instead of showing an error for a feature this installation does not have.
  const refreshShared = useCallback(() => api.listShared().then(setShared).catch(() => setShared([])), []);
  const refreshFav = useCallback(() => api.listFavorites().then(setFavorites).catch(() => {}), []);
  const refreshTags = useCallback(() => api.listTags().then(setTags).catch(() => {}), []);
  const refreshSpaces = useCallback(() => api.listSpaces().then(setSpaces).catch(() => {}), []);
  const refreshPostfach = useCallback(
    () => api.postfachAnzahl().then((a) => setUngelesen(a.ungelesen)).catch(() => {}),
    [],
  );

  useEffect(() => {
    refreshPages();
    refreshShared();
    refreshFav();
    refreshTags();
    refreshSpaces();
  }, [refreshPages, refreshShared, refreshFav, refreshTags, refreshSpaces]);

  // The inbox is checked regularly. One minute is the distance between "hears
  // about it too late" and "asks without occasion": this is about comments, not
  // about a conversation in real time. Only the number is fetched, not the list.
  useEffect(() => {
    refreshPostfach();
    const uhr = setInterval(refreshPostfach, 60_000);
    return () => clearInterval(uhr);
  }, [refreshPostfach]);

  // Navigate to the new page only after the tree has been refreshed, otherwise
  // the sidebar would briefly show no entry for the page now open.
  const createPage = async (parentId: string | null, spaceId: string | null = null) => {
    const p = await api.createPage(parentId, spaceId);
    await refreshPages();
    nav(`/page/${p.id}`);
  };

  // Deleting moves the page to the trash rather than removing it. Favorites are
  // refreshed too, since the page may have been pinned.
  const deletePage = async (id: string) => {
    if (
      !(await frage({
        titel: "Move page to trash",
        text: "This page and its subpages move to the trash. You can bring them back from there.",
        bestaetigen: "Move to trash",
      }))
    )
      return;
    await api.deletePage(id);
    await refreshPages();
    await refreshFav();
    nav("/");
  };

  const createSpace = async () => {
    const name = await eingabe({
      titel: "New space",
      text: "A space groups pages around one subject and carries the permissions for all of them.",
      feld: "Name",
      bestaetigen: "Create",
    });
    if (!name) return;
    await api.createSpace(name);
    refreshSpaces();
  };

  const renameSpace = async (id: string, current: string) => {
    const name = await eingabe({
      titel: "Rename space",
      feld: "Name",
      vorgabe: current,
      bestaetigen: "Rename",
    });
    if (!name || name === current) return;
    await api.renameSpace(id, name);
    refreshSpaces();
  };

  // The pages survive and become ungrouped, which the confirmation spells out
  // so nobody expects a space to take its content with it.
  const deleteSpace = async (id: string) => {
    if (
      !(await frage({
        titel: "Delete space",
        text: "The space is deleted. Its pages remain and appear under \u201eWithout a space\u201c afterwards; permissions granted on the space expire.",
        bestaetigen: "Delete space",
        gefaehrlich: true,
      }))
    )
      return;
    await api.deleteSpace(id);
    await refreshSpaces();
    await refreshPages();
  };

  // Switch the visibility of a space. Afterwards the page list is fetched anew
  // as well: an opened space brings other people's pages along, a closed one
  // takes them away again, and the sidebar would otherwise be wrong until
  // somebody reloads the page.
  const setSpaceOeffentlich = async (id: string, wert: "nein" | "lesen" | "schreiben") => {
    await api.spaceOeffentlich(id, wert);
    await refreshSpaces();
    await refreshPages();
  };

  // The colour of a space. Fetch only the spaces again and not the pages:
  // nothing changes on the pages, and a second request would make the tree
  // flicker for no reason.
  // Reparent or move a page after a sidebar drag. Both are one update, since a
  // page dropped into a different space usually changes its parent as well.
  // parentId null means top level, spaceId null means no space.
  const movePage = async (id: string, parentId: string | null, spaceId: string | null) => {
    await api.updatePage(id, { parentId, spaceId });
    await refreshPages();
  };

  // Dropped between two rows: hang it there AND put it in that place. One call,
  // because the backend has to write both in one transaction anyway -- a page
  // that hangs in the new spot but stands in the old order would be worse than
  // no move at all.
  const ordnePage = async (id: string, ziel: TreeGap) => {
    await api.seiteVerschieben(id, {
      elternId: ziel.elternId,
      spaceId: ziel.elternId === null ? (ziel.spaceId ?? null) : undefined,
      vorId: ziel.vorId,
    });
    await refreshPages();
  };

  // The order of the spaces. It is kept per account, so this changes nobody
  // else's sidebar.
  const ordneSpaces = async (ids: string[]) => {
    await api.spacesOrdnen(ids);
    await refreshSpaces();
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
        onCreateRoot={() => createPage(null, null)}
        onCreateChild={(pid) => createPage(pid)}
        onCreateInSpace={(sid) => createPage(null, sid)}
        onDelete={deletePage}
        onCreateSpace={createSpace}
        onRenameSpace={renameSpace}
        onDeleteSpace={deleteSpace}
        onSpaceOeffentlich={setSpaceOeffentlich}
        onMovePage={movePage}
        onOrdnePage={ordnePage}
        onOrdneSpaces={ordneSpaces}
        onNavigate={(to) => nav(to)}
        currentPath={loc.pathname}
        ungelesen={ungelesen}
        onEingefuehrt={() => {
          refreshPages();
          refreshSpaces();
          refreshTags();
        }}
      />
      <div className="main">
        <ZweitfaktorMahnung />
        {/* Until a lazily loaded part is there the waiting text stands here,
            faded in with a delay so that a fast switch shows nothing. */}
        <Suspense fallback={<div className="empty-state spaet">Loading…</div>}>
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
                  onCreateChild={(pid) => createPage(pid)}
                />
              }
            />
            <Route path="postfach" element={<PostfachView onGelesen={refreshPostfach} />} />
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
            {/* Accounts and groups now live in the settings. The old addresses
                stay valid and lead there; a bookmark shall not run into the
                void. */}
            <Route path="admin" element={<Navigate to="/einstellungen/nutzer" replace />} />
            {/* The audit trail now sits in the administration. The old address
                stays valid: it stood in the sidebar, and whoever has it as a
                bookmark should not land on an empty page. */}
            <Route path="pruefspur" element={<Navigate to="/einstellungen/protokoll" replace />} />
            <Route path="einstellungen" element={<EinstellungenView />} />
            <Route path="einstellungen/:bereich" element={<EinstellungenView />} />
            <Route path="gruppen" element={<Navigate to="/einstellungen/gruppen" replace />} />
            <Route
              path="tag/:tagId"
              element={<TagView allTags={tags} onTagsChange={refreshTags} />}
            />
          </Routes>
        </Suspense>
      </div>
    </div>
  );
}

/**
 * The hint shown when the instance demands the second factor and one's own
 * account has none.
 *
 * A stripe and not a bar: whoever works here is already signed in, and locking
 * them out before their work helps nobody. It disappears by itself as soon as
 * the factor is set up -- without a button to dismiss it, because then it would
 * stand there exactly once and never again.
 */
function ZweitfaktorMahnung() {
  const [noetig, setNoetig] = useState(false);
  const loc = useLocation();
  useEffect(() => {
    api
      .zweitfaktorStand()
      .then((z) => setNoetig(z.pflicht && !z.aktiv))
      .catch(() => setNoetig(false));
    // Ask again after a change of address: whoever has just set it up in the
    // settings should be rid of the stripe when they leave the page.
  }, [loc.pathname]);

  if (!noetig) return null;
  return (
    <div className="mahnstreifen">
      Diese Instanz verlangt einen zweiten Faktor. Solange keiner steht, genügt zum
      Anmelden dein Passwort allein.{" "}
      <Link to="/einstellungen/zugang">Set it up now</Link>
    </div>
  );
}

function EmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="empty-state">
      <div>Nothing open yet.</div>
      <button className="btn btn-primary" onClick={onCreate}>
        Seite erstellen
      </button>
    </div>
  );
}
