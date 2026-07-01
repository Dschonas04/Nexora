import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import type { Block } from "@blocknote/core";
import { Page, PagePatch, Tag, api } from "../api/client";
import Editor from "../components/Editor";

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
  const [page, setPage] = useState<Page | null>(null);
  const [loading, setLoading] = useState(true);
  const saveTimer = useRef<number | undefined>(undefined);

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    api
      .getPage(id)
      .then(setPage)
      .catch(() => setPage(null))
      .finally(() => setLoading(false));
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

  const setTitle = (title: string) => {
    setPage({ ...page, title });
    scheduleSave({ title });
  };
  const setIcon = (icon: string) => {
    setPage({ ...page, icon });
    scheduleSave({ icon });
  };
  const onContent = (blocks: Block[]) => scheduleSave({ content: blocks });

  const toggleFav = async () => {
    if (page.isFavorite) await api.removeFavorite(page.id);
    else await api.addFavorite(page.id);
    setPage({ ...page, isFavorite: !page.isFavorite });
    onFavChange();
  };

  const share = async () => {
    if (page.isPublic) {
      await api.unsharePage(page.id);
      setPage({ ...page, isPublic: false, publicToken: null });
    } else {
      const r = await api.sharePage(page.id);
      setPage({ ...page, isPublic: true, publicToken: r.publicToken });
      const url = `${window.location.origin}/share/${r.publicToken}`;
      try {
        await navigator.clipboard.writeText(url);
      } catch {
        /* clipboard may be unavailable */
      }
      alert("Public link copied to clipboard:\n" + url);
    }
  };

  const changeIcon = () => {
    const v = prompt("Emoji icon:", page.icon || "📄");
    if (v !== null) setIcon(v.trim());
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

  return (
    <>
      <div className="topbar">
        <span className="topbar-title">
          {page.icon || "📄"} {page.title || "Untitled"}
        </span>
        <div className="topbar-actions">
          <button className="btn" onClick={toggleFav}>
            {page.isFavorite ? "★ Favorited" : "☆ Favorite"}
          </button>
          <button className="btn" onClick={share}>
            {page.isPublic ? "🔗 Shared" : "Share"}
          </button>
          <button className="btn" onClick={() => onDelete(page.id)}>
            Delete
          </button>
        </div>
      </div>

      <div className="editor-scroll">
        <div className="page">
          <div className="page-icon" title="Change icon" onClick={changeIcon}>
            {page.icon || "📄"}
          </div>
          <input
            className="page-title"
            value={page.title}
            placeholder="Untitled"
            onChange={(e) => setTitle(e.target.value)}
          />
          <div className="page-tags">
            {page.tags.map((t) => (
              <span key={t.id} className="tag" style={{ background: t.color }}>
                {t.name}
                <span className="x" onClick={() => detachTag(t.id)}>
                  ✕
                </span>
              </span>
            ))}
            <button className="tag-add" onClick={addTag}>
              + Tag
            </button>
          </div>
          <Editor key={page.id} initialContent={page.content} onChange={onContent} />
        </div>
      </div>
    </>
  );
}
