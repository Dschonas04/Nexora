// The signed-in shell: sidebar on the left, routed view on the right.
//
// All shared lists (pages, shares, favorites, tags, spaces) are held here rather
// than in the sidebar, because the views on the right change them and the
// sidebar has to follow. The refresh callbacks passed down are how a view says
// "something you display has changed".
import { Suspense, lazy, useCallback, useEffect, useState } from "react";
import { Routes, Route, Navigate, useNavigate, useLocation } from "react-router-dom";
import { PageMeta, Space, Tag, api } from "../api/client";
import Sidebar from "../components/Sidebar";
import PageView from "./PageView";
import TagView from "./TagView";
import { useEingabe, useRueckfrage } from "../components/Rueckfrage";

// Nachgeladen statt mitgeliefert: diese Ansichten ruft man selten auf, ihr Code
// steckte aber im selben Bündel wie die Seitenansicht und musste bei jedem
// Aufruf der Anwendung mit übertragen und ausgewertet werden. Der Graph bringt
// dabei seine eigene Rechnerei mit, die Einstellungen ihre langen Formulare.
//
// Wer sie öffnet, wartet einmal kurz auf das Nachladen -- dafür startet die
// Anwendung für alle anderen schneller.
const TrashView = lazy(() => import("./TrashView"));
const GraphView = lazy(() => import("./GraphView"));
const PruefspurView = lazy(() => import("./PruefspurView"));
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
  // Zahl der ungelesenen Nachrichten. Sie hängt an keiner Ansicht, sondern an
  // der Leiste, und wird deshalb hier gehalten.
  const [ungelesen, setUngelesen] = useState(0);

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

  // Das Postfach wird regelmäßig nachgesehen. Eine Minute ist der Abstand
  // zwischen "erfährt es zu spät" und "fragt ohne Anlass": es geht um
  // Kommentare, nicht um eine Unterhaltung in Echtzeit. Geholt wird nur die
  // Zahl, nicht die Liste.
  useEffect(() => {
    refreshPostfach();
    const uhr = setInterval(refreshPostfach, 60_000);
    return () => clearInterval(uhr);
  }, [refreshPostfach]);

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
    if (
      !(await frage({
        titel: "Seite in den Papierkorb",
        text: "Diese Seite und ihre Unterseiten wandern in den Papierkorb. Von dort lassen sie sich zurückholen.",
        bestaetigen: "In den Papierkorb",
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
      titel: "Neue Ablage",
      text: "Eine Ablage bündelt Seiten zu einem Thema und trägt die Rechte für alle darin.",
      feld: "Name",
      bestaetigen: "Anlegen",
    });
    if (!name) return;
    await api.createSpace(name);
    refreshSpaces();
  };

  const renameSpace = async (id: string, current: string) => {
    const name = await eingabe({
      titel: "Ablage umbenennen",
      feld: "Name",
      vorgabe: current,
      bestaetigen: "Umbenennen",
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
        titel: "Ablage löschen",
        text: "Die Ablage wird gelöscht. Ihre Seiten bleiben erhalten und stehen danach unter „Ohne Ablage“; erteilte Rechte an der Ablage verfallen.",
        bestaetigen: "Ablage löschen",
        gefaehrlich: true,
      }))
    )
      return;
    await api.deleteSpace(id);
    await refreshSpaces();
    await refreshPages();
  };

  // Sichtbarkeit einer Ablage umstellen. Danach wird auch die Seitenliste neu
  // geholt: eine geöffnete Ablage bringt fremde Seiten mit, eine geschlossene
  // nimmt sie wieder mit -- die Leiste wäre sonst so lange falsch, bis jemand
  // die Seite neu lädt.
  const setSpaceOeffentlich = async (id: string, wert: "nein" | "lesen" | "schreiben") => {
    await api.spaceOeffentlich(id, wert);
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
        onSpaceOeffentlich={setSpaceOeffentlich}
        onMovePage={movePage}
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
        {/* Bis ein nachgeladener Teil da ist, steht hier der Wartetext --
            verzögert eingeblendet, damit ein schneller Wechsel nichts zeigt. */}
        <Suspense fallback={<div className="empty-state spaet">Lädt…</div>}>
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
            {/* Konten und Gruppen liegen jetzt in den Einstellungen. Die alten
                Adressen bleiben gültig und führen dorthin -- ein Lesezeichen
                soll nicht ins Leere laufen. */}
            <Route path="admin" element={<Navigate to="/einstellungen/nutzer" replace />} />
            <Route path="pruefspur" element={<PruefspurView />} />
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
