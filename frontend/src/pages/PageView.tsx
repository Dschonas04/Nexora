import { useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import type { Block, BlockNoteEditor } from "@blocknote/core";
import { Graph, Page, PageMeta, PagePatch, Tag, api } from "../api/client";
import Editor from "../components/Editor";
import VersionPanel from "../components/VersionPanel";
import ShareDialog from "../components/ShareDialog";
import Attachments from "../components/Attachments";
import LocalGraph from "../components/LocalGraph";

interface Props {
  allTags: Tag[];
  onMetaChange: () => void;
  onFavChange: () => void;
  onTagsChange: () => void;
  onDelete: (id: string) => void;
}

const PALETTE = ["#e0507a", "#e08a2b", "#3aa675", "#2383e2", "#8b5cf6", "#6b7280"];
const randomColor = () => PALETTE[Math.floor(Math.random() * PALETTE.length)];

export default function PageView({ allTags, onMetaChange, onFavChange, onTagsChange, onDelete }: Props) {
  const { id } = useParams();
  const nav = useNavigate();
  const [page, setPage] = useState<Page | null>(null);
  const [backlinks, setBacklinks] = useState<PageMeta[]>([]);
  const [links, setLinks] = useState<PageMeta[]>([]);
  const [graph, setGraph] = useState<Graph>({ nodes: [], edges: [] });
  const [loading, setLoading] = useState(true);
  const [showVersions, setShowVersions] = useState(false);
  const [showShare, setShowShare] = useState(false);
  const saveTimer = useRef<number | undefined>(undefined);
  const editorRef = useRef<BlockNoteEditor | null>(null);

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    setShowVersions(false);
    setShowShare(false);
    api
      .getPage(id)
      .then(setPage)
      .catch(() => setPage(null))
      .finally(() => setLoading(false));
    api.backlinks(id).then(setBacklinks).catch(() => setBacklinks([]));
    api.listLinks(id).then(setLinks).catch(() => setLinks([]));
    api.graph().then(setGraph).catch(() => setGraph({ nodes: [], edges: [] }));
    return () => window.clearTimeout(saveTimer.current);
  }, [id]);

  // Resolve a [[Title]] to a page id (case-insensitive) so wiki-links in the
  // editor become clickable.
  const titleToId = (() => {
    const m: Record<string, string> = {};
    for (const n of graph.nodes) m[(n.title || "").toLowerCase().trim()] = n.id;
    return m;
  })();
  const resolveLink = (t: string) => titleToId[t.toLowerCase().trim()] ?? null;

  const scheduleSave = (patch: PagePatch) => {
    if (!id) return;
    window.clearTimeout(saveTimer.current);
    saveTimer.current = window.setTimeout(async () => {
      try {
        await api.updatePage(id, patch);
        onMetaChange();
      } catch {
        /* ignore transient save errors */
      }
    }, 500);
  };

  if (loading) return <div className="empty-state">Lädt…</div>;
  if (!page) return <div className="empty-state">Seite nicht gefunden.</div>;

  const canEdit = page.canEdit;

  const setTitle = (title: string) => {
    setPage({ ...page, title });
    scheduleSave({ title });
  };
  const onContent = (blocks: Block[]) => {
    if (canEdit) scheduleSave({ content: blocks });
  };

  const toggleFav = async () => {
    if (page.isFavorite) await api.removeFavorite(page.id);
    else await api.addFavorite(page.id);
    setPage({ ...page, isFavorite: !page.isFavorite });
    onFavChange();
  };

  const exportMarkdown = async () => {
    const editor = editorRef.current;
    if (!editor) return;
    const md = await editor.blocksToMarkdownLossy(editor.document);
    const blob = new Blob([md], { type: "text/markdown" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${(page.title || "untitled").replace(/[^\w.-]+/g, "-")}.md`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const addTag = async () => {
    const name = prompt("Schlagwort:")?.trim();
    if (!name) return;
    let tag = allTags.find((t) => t.name.toLowerCase() === name.toLowerCase());
    if (!tag) {
      tag = await api.createTag(name, randomColor());
      onTagsChange();
    }
    await api.attachTag(page.id, tag.id);
    setPage({ ...page, tags: [...page.tags.filter((t) => t.id !== tag!.id), tag] });
  };

  const detachTag = async (tagId: string) => {
    await api.detachTag(page.id, tagId);
    setPage({ ...page, tags: page.tags.filter((t) => t.id !== tagId) });
  };

  const restoredFromVersion = (p: Page) => {
    setPage(p);
    setShowVersions(false);
    onMetaChange();
  };

  // Manual links: refresh links, backlinks and the graph together so the local
  // mini-graph and "linked from" list stay in sync after an edit.
  const refreshLinks = () => {
    if (!id) return;
    api.listLinks(id).then(setLinks).catch(() => {});
    api.backlinks(id).then(setBacklinks).catch(() => {});
    api.graph().then(setGraph).catch(() => {});
  };
  const addLink = async (targetId: string) => {
    if (!id || !targetId) return;
    await api.addLink(id, targetId);
    refreshLinks();
  };
  const removeLink = async (targetId: string) => {
    if (!id) return;
    await api.removeLink(id, targetId);
    refreshLinks();
  };
  const linkCandidates = graph.nodes
    .filter((n) => n.id !== page.id && !links.some((l) => l.id === n.id))
    .sort((a, b) => (a.title || "").localeCompare(b.title || ""));

  return (
    <div className="page-layout">
      <div className="page-main">
        <div className="topbar">
          <span className="topbar-title">{page.title || "Ohne Titel"}</span>
          <div className="topbar-actions">
            {!canEdit && <span className="pill readonly">Nur Lesen</span>}
            <button className={"btn" + (page.isFavorite ? " active" : "")} onClick={toggleFav}>
              {page.isFavorite ? "Favorisiert" : "Favorit"}
            </button>
            <button className="btn" onClick={() => setShowVersions((v) => !v)}>
              Verlauf
            </button>
            <button className="btn" onClick={exportMarkdown}>
              Export
            </button>
            {page.isOwner && (
              <button className={"btn" + (page.isPublic ? " active" : "")} onClick={() => setShowShare(true)}>
                Teilen
              </button>
            )}
            {page.isOwner && (
              <button className="btn" onClick={() => onDelete(page.id)}>
                Löschen
              </button>
            )}
          </div>
        </div>

        <div className="editor-scroll">
          <div className="page">
            <input
              className="page-title"
              value={page.title}
              placeholder="Ohne Titel"
              disabled={!canEdit}
              onChange={(e) => setTitle(e.target.value)}
            />
            <div className="page-tags">
              {page.tags.map((t) => (
                <span key={t.id} className="tag" style={{ background: t.color }}>
                  {t.name}
                  {canEdit && (
                    <span className="x" onClick={() => detachTag(t.id)}>
                      ✕
                    </span>
                  )}
                </span>
              ))}
              {canEdit && (
                <button className="tag-add" onClick={addTag}>
                  + Schlagwort
                </button>
              )}
            </div>
            <Editor
              key={page.id}
              initialContent={page.content}
              editable={canEdit}
              onChange={onContent}
              onEditorReady={(e) => (editorRef.current = e)}
              linkResolver={resolveLink}
              onOpenLink={(pid) => nav(`/page/${pid}`)}
            />
            <Attachments pageId={page.id} canEdit={canEdit} />
            {(links.length > 0 || canEdit) && (
              <div className="page-links">
                <div className="page-links-title">Verknüpfungen</div>
                <div className="page-links-list">
                  {links.map((l) => (
                    <span key={l.id} className="page-link-chip">
                      <button className="page-link-open" onClick={() => nav(`/page/${l.id}`)}>
                        {l.title || "Ohne Titel"}
                      </button>
                      {canEdit && (
                        <span className="x" title="Verknüpfung entfernen" onClick={() => removeLink(l.id)}>
                          ✕
                        </span>
                      )}
                    </span>
                  ))}
                  {canEdit && (
                    <select
                      className="page-link-add"
                      value=""
                      onChange={(e) => {
                        const v = e.target.value;
                        e.target.value = "";
                        addLink(v);
                      }}
                    >
                      <option value="">+ Verknüpfung…</option>
                      {linkCandidates.map((n) => (
                        <option key={n.id} value={n.id}>
                          {n.title || "Ohne Titel"}
                        </option>
                      ))}
                    </select>
                  )}
                </div>
                {links.length === 0 && (
                  <div className="page-links-hint">
                    Noch keine manuellen Verknüpfungen. Wähle oben eine Seite, um sie zu verbinden.
                  </div>
                )}
              </div>
            )}
            {backlinks.length > 0 && (
              <div className="backlinks">
                <div className="backlinks-title">
                  Verlinkt von {backlinks.length} {backlinks.length === 1 ? "Seite" : "Seiten"}
                </div>
                <div className="backlinks-list">
                  {backlinks.map((b) => (
                    <button key={b.id} className="backlink" onClick={() => nav(`/page/${b.id}`)}>
                      {b.title || "Ohne Titel"}
                    </button>
                  ))}
                </div>
              </div>
            )}
            <LocalGraph graph={graph} pageId={page.id} onOpen={(pid) => nav(`/page/${pid}`)} />
          </div>
        </div>
      </div>

      {showVersions && (
        <VersionPanel
          pageId={page.id}
          canEdit={canEdit}
          onRestored={restoredFromVersion}
          onClose={() => setShowVersions(false)}
        />
      )}

      {showShare && (
        <ShareDialog
          pageId={page.id}
          isPublic={page.isPublic}
          publicToken={page.publicToken}
          onPublicChange={(isPublic, token) => setPage({ ...page, isPublic, publicToken: token })}
          onClose={() => setShowShare(false)}
        />
      )}
    </div>
  );
}
