// A single open page: title, editor, tags, attachments, links and the panels
// for version history and sharing. The largest view in the app, because a page
// is where nearly every feature meets.
import { useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import type { Block, BlockNoteEditor } from "@blocknote/core";
import { Graph, Page, PageMeta, PagePatch, Tag, api } from "../api/client";
import Editor from "../components/Editor";
import VersionPanel from "../components/VersionPanel";
import ShareDialog from "../components/ShareDialog";
import Attachments from "../components/Attachments";
import { useLizenz } from "../lizenz";
import LocalGraph from "../components/LocalGraph";

interface Props {
  allTags: Tag[];
  onMetaChange: () => void;
  onFavChange: () => void;
  onTagsChange: () => void;
  onDelete: (id: string) => void;
}

// New tags get a random color from a fixed palette, so they are distinguishable
// at a glance without asking the user to pick one.
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
  // Paid extras. Hidden rather than disabled: a greyed-out button that never
  // becomes usable is noise. The backend refuses the same calls with 402
  // anyway, so this is about the interface not lying, not about protection.
  const { frei } = useLizenz();

  // Der Stand, von dem dieser Editor ausgeht. Als ref, nicht als state: er
  // wird beim Speichern gelesen, soll aber kein neues Rendern auslösen.
  const basisRef = useRef<string | undefined>(undefined);
  const [konflikt, setKonflikt] = useState(false);

  const [showVersions, setShowVersions] = useState(false);
  const [showShare, setShowShare] = useState(false);
  const [linkQuery, setLinkQuery] = useState("");
  const [linkOpen, setLinkOpen] = useState(false);
  const [editorKey, setEditorKey] = useState(0);
  // Refs rather than state: changing either must not trigger a render. The timer
  // drives the debounced autosave, the editor handle is needed for the markdown
  // export.
  const saveTimer = useRef<number | undefined>(undefined);
  const editorRef = useRef<BlockNoteEditor | null>(null);

  // Reload everything when the route changes to another page. The cleanup
  // cancels a pending save, so a half-typed edit cannot land on the page that
  // was just left.
  //
  // The full graph is fetched as well, which is what makes [[wiki-links]] in the
  // text resolvable to page ids without a lookup per link.
  useEffect(() => {
    if (!id) return;
    setLoading(true);
    setShowVersions(false);
    setShowShare(false);
    setKonflikt(false);
    api
      .getPage(id)
      .then((p) => {
        setPage(p);
        // Der Ausgangsstand für die Konflikterkennung. Ab hier gilt: was
        // dieser Editor speichert, baut auf genau diesem Stand auf.
        basisRef.current = p.updatedAt;
      })
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

  // Autosave, debounced by half a second. Every change resets the timer, so
  // continuous typing produces one request when the user pauses.
  //
  // A failed save stays silent -- the next keystroke retries anyway, and an
  // error banner during typing would be more disruptive than useful. One
  // failure is the exception: a 409 means somebody else saved in between, and
  // swallowing that is exactly how their work disappears. It stops the autosave
  // and asks.
  const scheduleSave = (patch: PagePatch) => {
    if (!id) return;
    window.clearTimeout(saveTimer.current);
    saveTimer.current = window.setTimeout(async () => {
      try {
        // basis carries the state this editor started from. The backend
        // compares and refuses if the page moved on.
        const frisch = await api.updatePage(id, { ...patch, basis: basisRef.current });
        // Die Basis wandert mit: sonst meldete der nächste Autosave einen
        // Konflikt mit dem eigenen vorherigen Speichern.
        basisRef.current = frisch.updatedAt;
        onMetaChange();
      } catch (e) {
        const err = e as Error & { status?: number };
        if (err.status === 409) {
          setKonflikt(true);
          return;
        }
        /* ignore transient save errors */
      }
    }, 500);
  };

  // Reload and drop the local edit. The safe way out of a conflict: the other
  // version wins and this editor starts again from it.
  const neuLaden = () => {
    setKonflikt(false);
    window.location.reload();
  };

  // Force the local version through. basis is left out, so the backend does not
  // check -- the user has been told and decided.
  const trotzdemSpeichern = async () => {
    if (!id || !page) return;
    try {
      await api.updatePage(id, { title: page.title, content: page.content, icon: page.icon });
      setKonflikt(false);
      onMetaChange();
    } catch {
      /* bleibt stehen, der Hinweis auch */
    }
  };

  if (loading) return <div className="empty-state">Lädt…</div>;
  if (!page) return <div className="empty-state">Seite nicht gefunden.</div>;

  const canEdit = page.canEdit;

  // Update the local copy immediately and let the save follow, so typing stays
  // responsive instead of waiting for a round trip.
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

  // Export through the editor rather than the stored JSON, since only BlockNote
  // knows how to render its own document. Lossy by name and by nature: anything
  // markdown cannot express is dropped. The blob URL is revoked right after the
  // click so it does not stay attached to the document.
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

  // Tags are matched by name, case-insensitively, and created only when none
  // exists. Otherwise typing "Projekt" twice would produce two separate tags.
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

  // Links, backlinks and the graph are refreshed together: adding one link
  // changes all three views, and reloading them separately would briefly show a
  // page linking to something the mini-graph does not know about yet.
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

  // Outgoing links written into the text as [[Title]], which is also what an
  // @-mention inserts. Extracted by running the regex over the stringified
  // document: the brackets only ever occur in visible text, never in the
  // structural JSON, so this cannot pick up a false positive. Deduped
  // case-insensitively while keeping the spelling of the first occurrence.
  const textLinkTitles = (() => {
    const seen = new Set<string>();
    const out: string[] = [];
    const re = /\[\[([^[\]]+)\]\]/g;
    let m: RegExpExecArray | null;
    const s = JSON.stringify((page.content as unknown) ?? []);
    while ((m = re.exec(s))) {
      const raw = m[1].trim();
      const k = raw.toLowerCase();
      if (raw && !seen.has(k)) {
        seen.add(k);
        out.push(raw);
      }
    }
    return out;
  })();

  // Removing a text link strips the brackets but keeps the word, so the sentence
  // still reads. Every text field in the document is walked, because the title
  // may appear in several blocks.
  //
  // The editor is remounted afterwards (editorKey) rather than updated in place:
  // BlockNote takes its content once at mount and would otherwise keep showing
  // the old text.
  const removeTextLink = async (title: string) => {
    if (!id) return;
    // Escape the title before building the pattern: a page called "C++ (Notes)"
    // would otherwise be an invalid regular expression.
    const esc = title.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const re = new RegExp(`\\[\\[\\s*${esc}\\s*\\]\\]`, "gi");
    const strip = (v: unknown): unknown => {
      if (Array.isArray(v)) return v.map(strip);
      // Only the "text" fields carry visible content; everything else is walked
      // but left alone, so styles and block ids survive untouched.
      if (v && typeof v === "object") {
        const o = { ...(v as Record<string, unknown>) };
        if (typeof o.text === "string") {
          o.text = o.text.replace(re, (mm) => mm.replace(/^\[\[\s*/, "").replace(/\s*\]\]$/, ""));
        }
        for (const key of Object.keys(o)) if (key !== "text") o[key] = strip(o[key]);
        return o;
      }
      return v;
    };
    const newContent = strip(page.content);
    await api.updatePage(id, { content: newContent });
    setPage({ ...page, content: newContent });
    setEditorKey((k) => k + 1);
    refreshLinks();
  };
  // Candidates for a new link: every visible page except this one and the ones
  // already linked. Capped at eight after filtering, so the list stays a
  // suggestion rather than a full directory.
  const linkCandidates = graph.nodes
    .filter((n) => n.id !== page.id && !links.some((l) => l.id === n.id))
    .sort((a, b) => (a.title || "").localeCompare(b.title || ""));
  const filteredCandidates = linkCandidates
    .filter((n) => (n.title || "").toLowerCase().includes(linkQuery.toLowerCase()))
    .slice(0, 8);
  // Picking a suggestion adds the link and closes the box, so the next one
  // starts from an empty query.
  const pickLink = (targetId: string) => {
    addLink(targetId);
    setLinkQuery("");
    setLinkOpen(false);
  };

  return (
    <div className="page-layout">
      <div className="page-main">
        <div className="topbar">
          <span className="topbar-title">{page.title || "Ohne Titel"}</span>
          <div className="topbar-actions">
            {/* Say plainly that this is a read-only share, instead of leaving
                the user to wonder why nothing saves. */}
            {!canEdit && <span className="pill readonly">Nur Lesen</span>}
            <button className={"btn" + (page.isFavorite ? " active" : "")} onClick={toggleFav}>
              {page.isFavorite ? "Favorisiert" : "Favorit"}
            </button>
            {frei("versionen") && (
              <button className="btn" onClick={() => setShowVersions((v) => !v)}>
                Verlauf
              </button>
            )}
            <button className="btn" onClick={exportMarkdown}>
              Export
            </button>
            {page.isOwner && frei("freigeben") && (
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
              key={`${page.id}:${editorKey}`}
              initialContent={page.content}
              editable={canEdit}
              onChange={onContent}
              onEditorReady={(e) => (editorRef.current = e)}
              linkResolver={resolveLink}
              onOpenLink={(pid) => nav(`/page/${pid}`)}
              mentionTargets={graph.nodes.filter((n) => n.id !== page.id)}
            />
            {frei("anhaenge") && <Attachments pageId={page.id} canEdit={canEdit} />}
            {(links.length > 0 || textLinkTitles.length > 0 || canEdit) && (
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
                  {textLinkTitles.map((t) => {
                    const tid = resolveLink(t);
                    return (
                      <span
                        key={"t:" + t}
                        className="page-link-chip page-link-chip-text"
                        title="Aus dem Text ([[…]] / @-Erwähnung)"
                      >
                        {tid ? (
                          <button className="page-link-open" onClick={() => nav(`/page/${tid}`)}>
                            {t}
                          </button>
                        ) : (
                          <span className="page-link-open muted">{t}</span>
                        )}
                        {canEdit && (
                          <span
                            className="x"
                            title="Verknüpfung aus dem Text entfernen"
                            onClick={() => removeTextLink(t)}
                          >
                            ✕
                          </span>
                        )}
                      </span>
                    );
                  })}
                  {canEdit && (
                    <div className="page-link-picker">
                      <input
                        className="page-link-search"
                        placeholder="+ Seite suchen & verknüpfen…"
                        value={linkQuery}
                        onChange={(e) => {
                          setLinkQuery(e.target.value);
                          setLinkOpen(true);
                        }}
                        onFocus={() => setLinkOpen(true)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter" && filteredCandidates[0]) pickLink(filteredCandidates[0].id);
                          if (e.key === "Escape") setLinkOpen(false);
                        }}
                        onBlur={() => setTimeout(() => setLinkOpen(false), 150)}
                      />
                      {linkOpen && filteredCandidates.length > 0 && (
                        <div className="page-link-dropdown">
                          {filteredCandidates.map((n) => (
                            <button
                              key={n.id}
                              className="page-link-option"
                              onMouseDown={(e) => e.preventDefault()}
                              onClick={() => pickLink(n.id)}
                            >
                              {n.title || "Ohne Titel"}
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                </div>
                {links.length === 0 && textLinkTitles.length === 0 && (
                  <div className="page-links-hint">
                    Noch keine Verknüpfungen. Wähle oben eine Seite, oder tippe @ bzw. [[…]] im Text.
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

      {konflikt && (
        <div className="konflikt-banner">
          <div>
            <strong>Die Seite wurde inzwischen an anderer Stelle geändert.</strong>
            <div className="muted small">
              Automatisches Speichern ist angehalten, damit nichts überschrieben wird.
            </div>
          </div>
          <div className="konflikt-aktionen">
            <button className="btn" onClick={neuLaden}>
              Neu laden
            </button>
            <button className="btn warnend" onClick={trotzdemSpeichern}>
              Meine Fassung behalten
            </button>
          </div>
        </div>
      )}

      {showVersions && frei("versionen") && (
        <VersionPanel
          pageId={page.id}
          canEdit={canEdit}
          onRestored={restoredFromVersion}
          onClose={() => setShowVersions(false)}
        />
      )}

      {showShare && frei("freigeben") && (
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
