import { useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import type { Block, BlockNoteEditor } from "@blocknote/core";
import { Page, PageMeta, PagePatch, Tag, api } from "../api/client";
import Editor from "../components/Editor";
import VersionPanel from "../components/VersionPanel";
import ShareDialog from "../components/ShareDialog";
import Attachments from "../components/Attachments";

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
    return () => window.clearTimeout(saveTimer.current);
  }, [id]);

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

  if (loading) return <div className="empty-state">Loading…</div>;
  if (!page) return <div className="empty-state">Page not found.</div>;

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
    const name = prompt("Tag name:")?.trim();
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

  return (
    <div className="page-layout">
      <div className="page-main">
        <div className="topbar">
          <span className="topbar-title">{page.title || "Untitled"}</span>
          <div className="topbar-actions">
            {!canEdit && <span className="pill readonly">Read-only</span>}
            <button className={"btn" + (page.isFavorite ? " active" : "")} onClick={toggleFav}>
              {page.isFavorite ? "Favorited" : "Favorite"}
            </button>
            <button className="btn" onClick={() => setShowVersions((v) => !v)}>
              History
            </button>
            <button className="btn" onClick={exportMarkdown}>
              Export
            </button>
            {page.isOwner && (
              <button className={"btn" + (page.isPublic ? " active" : "")} onClick={() => setShowShare(true)}>
                Share
              </button>
            )}
            {page.isOwner && (
              <button className="btn" onClick={() => onDelete(page.id)}>
                Delete
              </button>
            )}
          </div>
        </div>

        <div className="editor-scroll">
          <div className="page">
            <input
              className="page-title"
              value={page.title}
              placeholder="Untitled"
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
                  + Tag
                </button>
              )}
            </div>
            <Editor
              key={page.id}
              initialContent={page.content}
              editable={canEdit}
              onChange={onContent}
              onEditorReady={(e) => (editorRef.current = e)}
            />
            <Attachments pageId={page.id} canEdit={canEdit} />
            {backlinks.length > 0 && (
              <div className="backlinks">
                <div className="backlinks-title">
                  Linked from {backlinks.length} {backlinks.length === 1 ? "page" : "pages"}
                </div>
                <div className="backlinks-list">
                  {backlinks.map((b) => (
                    <button key={b.id} className="backlink" onClick={() => nav(`/page/${b.id}`)}>
                      {b.title || "Untitled"}
                    </button>
                  ))}
                </div>
              </div>
            )}
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
