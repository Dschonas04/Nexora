// A single open page: title, editor, tags, attachments, links and the panels
// for version history and sharing. The largest view in the app, because a page
// is where nearly every feature meets.
import { Suspense, lazy, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import type { Block, BlockNoteEditor, PartialBlock } from "@blocknote/core";
import { Graph, Page, PageMeta, PagePatch, Tag, api } from "../api/client";
// The editor brings BlockNote along, and that is the largest single piece of
// the bundle. Whoever leafs through the list, looks at the graph or goes into
// the settings needs none of it.
const Editor = lazy(() => import("../components/Editor"));
import { useMitschrift } from "../mitschrift";
import VersionPanel from "../components/VersionPanel";
import ShareDialog from "../components/ShareDialog";
import Fehlergrenze from "../components/Fehlergrenze";
import Attachments from "../components/Attachments";
import Kommentare from "../components/Kommentare";
import { useLizenz } from "../lizenz";
import { useAuth } from "../auth";
import { useDesign } from "../design";
import { schriftAuf } from "../farbe";
import LocalGraph from "../components/LocalGraph";
import { useEingabe } from "../components/Rueckfrage";
import { useAussenklick } from "../klappen";

interface Props {
  allTags: Tag[];
  onMetaChange: () => void;
  onFavChange: () => void;
  onTagsChange: () => void;
  onDelete: (id: string) => void;
  // Creates a subpage and jumps into it. The sidebar can already do this; the
  // same thing stands up here because one notices while writing that a subpage
  // is missing, and is then inside the page and not in the sidebar.
  onCreateChild: (parentId: string) => void;
}

// New tags get a random color from a fixed palette, so they are distinguishable
// at a glance without asking the user to pick one.
const PALETTE = [
  "#e0507a",
  "#e08a2b",
  "#3aa675",
  "#2383e2",
  "#8b5cf6",
  "#6b7280",
];
const randomColor = () => PALETTE[Math.floor(Math.random() * PALETTE.length)];

// The skeleton stands while a page is being fetched: sidebar, title and a few
// text bars at exactly the places where the content will be.
//
// Before this a bare "Loading…" stood here. With it the whole interface right
// of the sidebar disappeared while paging from one page to the next and built
// itself up again a blink later, a flicker in the home network, a jump over a
// slow line where one loses the place of one's text.
//
// The bars fade in with a delay. A page that is there within a few milliseconds
// therefore shows no waiting pattern at all, only the calm frame it falls into.
function Geruest() {
  return (
    <div className="page-layout">
      <div className="page-main">
        <div className="topbar">
          <span className="geruest-strich" style={{ width: 140 }} />
        </div>
        <div className="editor-scroll">
          <div className="page">
            <div className="geruest-strich geruest-titel" />
            <div className="geruest-absatz">
              <span className="geruest-strich" style={{ width: "94%" }} />
              <span className="geruest-strich" style={{ width: "88%" }} />
              <span className="geruest-strich" style={{ width: "61%" }} />
            </div>
            <div className="geruest-absatz">
              <span className="geruest-strich" style={{ width: "91%" }} />
              <span className="geruest-strich" style={{ width: "43%" }} />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

// istLeer says whether a document still contains nothing. Not "no blocks":
// BlockNote puts an empty paragraph into a fresh document of its own accord,
// and that does not count as content.
function istLeer(bloecke: Block[]): boolean {
  return bloecke.every((b) => {
    const inhalt = b.content as unknown;
    if (Array.isArray(inhalt) && inhalt.length > 0) return false;
    if (Array.isArray(b.children) && b.children.length > 0) return false;
    // A picture or a file has no text content and is something all the same.
    return b.type === "paragraph" || b.type === "heading";
  });
}

export default function PageView({
  allTags,
  onMetaChange,
  onFavChange,
  onTagsChange,
  onDelete,
  onCreateChild,
}: Props) {
  const { id } = useParams();
  const nav = useNavigate();
  const eingabe = useEingabe();
  const [page, setPage] = useState<Page | null>(null);
  const [backlinks, setBacklinks] = useState<PageMeta[]>([]);
  const [links, setLinks] = useState<PageMeta[]>([]);
  const [graph, setGraph] = useState<Graph>({ nodes: [], edges: [] });
  const [loading, setLoading] = useState(true);
  // Paid extras. Hidden rather than disabled: a greyed-out button that never
  // becomes usable is noise. The backend refuses the same calls with 402
  // anyway, so this is about the interface not lying, not about protection.
  const { frei } = useLizenz();
  // Who sits here. When writing together the name stands at the other person's
  // cursor, so one sees whose the caret running along there is.
  const { user } = useAuth();
  // The instance default for pages that say nothing themselves.
  const { design } = useDesign();
  const vorgabe = design.seitenbreite || "voll";
  const vorgabeName =
    vorgabe === "normal"
      ? "normal"
      : vorgabe === "breit"
        ? "breit"
        : "volle Breite";

  // The state this editor starts from. As a ref, not as state: it is read while
  // saving but shall not trigger a new render.
  const basisRef = useRef<string | undefined>(undefined);
  const [konflikt, setKonflikt] = useState(false);

  const [showVersions, setShowVersions] = useState(false);
  const [showShare, setShowShare] = useState(false);
  const [linkQuery, setLinkQuery] = useState("");
  const [linkOpen, setLinkOpen] = useState(false);
  const [editorKey, setEditorKey] = useState(0);
  // State of the file drop and of the export. By subject matter they belong
  // further down, but they stand up here because React has to see the states of
  // a view in the same number and order in every pass. Further down they sat
  // behind the early returns for "loading" and "not found", so the first pass
  // counted three states fewer than the second, and React aborted the view with
  // error 310: the page stayed empty.
  //
  // A counter instead of a flag: dragleave also fires when moving from one child
  // element to the next, otherwise the highlight would flicker.
  const [ueberSeite, setUeberSeite] = useState(0);
  const [eingeworfen, setEingeworfen] = useState<FileList | null>(null);
  // Counts up whenever the editor has uploaded a picture: the list below the
  // page reads its own data and would otherwise not know about it until the next
  // time the page is opened.
  const [anhangTick, setAnhangTick] = useState(0);
  const [exportOffen, setExportOffen] = useState(false);
  const [breiteOffen, setBreiteOffen] = useState(false);
  // Here too: a click beside it and Escape close it. An open menu above the
  // text is worse than one in the sidebar, because the editor lies underneath
  // it.
  const exportBereich = useAussenklick<HTMLDivElement>(exportOffen, () =>
    setExportOffen(false),
  );
  const breiteBereich = useAussenklick<HTMLDivElement>(breiteOffen, () =>
    setBreiteOffen(false),
  );
  // Refs rather than state: changing either must not trigger a render. The timer
  // drives the debounced autosave, the editor handle is needed for the markdown
  // export.
  const saveTimer = useRef<number | undefined>(undefined);
  const editorRef = useRef<BlockNoteEditor | null>(null);

  // Writing together. It only runs when more than one account may write on
  // this page at all: the service decides that and says so in page.gemeinsam. A
  // page belonging only to its owner does not open a session a second person
  // will never sit in.
  const mitschrift = useMitschrift(
    id,
    !!page?.gemeinsam && !!page?.canEdit,
    user,
  );
  // For which page the seed has already been sown. Without that, every render
  // would sow again.
  const saatRef = useRef<string | null>(null);
  // Through a ref, because the deferred saving only runs once the render that
  // set it going is long over.
  const mitschriftRef = useRef(mitschrift);
  mitschriftRef.current = mitschrift;

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
        // The starting state for the conflict detection. From here on: whatever
        // this editor saves builds on exactly this state.
        basisRef.current = p.updatedAt;
      })
      .catch(() => setPage(null))
      .finally(() => setLoading(false));
    api
      .backlinks(id)
      .then(setBacklinks)
      .catch(() => setBacklinks([]));
    api
      .listLinks(id)
      .then(setLinks)
      .catch(() => setLinks([]));
    api
      .graph()
      .then(setGraph)
      .catch(() => setGraph({ nodes: [], edges: [] }));
    return () => window.clearTimeout(saveTimer.current);
  }, [id]);

  // Put the first revision into a fresh shared document.
  //
  // A room comes into being empty and dies as soon as the last person leaves
  // it. So the text sits in the database only, and somebody has to carry it in,
  // otherwise everybody sits in front of an empty page that is nothing of the
  // sort. The leader does it, and leaves a marker in the document itself: it
  // travels along to everybody who joins later, and nobody carries it in a
  // second time.
  //
  // The short wait is against the one case in which both would go wrong: two
  // browsers open the page in the same second and know nothing of each other at
  // the moment of the sync, so both take themselves for the leader. After half
  // a moment they know each other.
  useEffect(() => {
    if (!mitschrift?.bereit || !mitschrift.fuehrend || !page || !id) return;
    if (saatRef.current === id) return;
    const zeiger = window.setTimeout(() => {
      const marke = mitschrift.doc.getMap<boolean>("nexora");
      saatRef.current = id;
      if (marke.get("gesaet")) return;
      const ed = editorRef.current;
      if (!ed) return;
      // Only into an empty document. If text is already there, somebody else
      // carried it in, and theirs counts.
      if (!istLeer(ed.document)) {
        marke.set("gesaet", true);
        return;
      }
      const inhalt = page.content;
      if (Array.isArray(inhalt) && inhalt.length > 0) {
        ed.replaceBlocks(ed.document, inhalt as PartialBlock[]);
      }
      marke.set("gesaet", true);
    }, 500);
    return () => window.clearTimeout(zeiger);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mitschrift?.bereit, mitschrift?.fuehrend, id, page?.id]);

  // On switching to another page the seed starts over.
  useEffect(() => {
    saatRef.current = null;
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
  // A failed save stays silent, the next keystroke retries anyway, and an
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
        // Without a basis while writing together: the two cases it is meant to
        // distinguish no longer exist there. Simultaneous changes are already
        // merged by the time they arrive here, and when the lead changes
        // because somebody closes their tab, the one moving up starts with a
        // basis from a while ago and would be told of a conflict that does not
        // exist.
        const frisch = await api.updatePage(id, {
          ...patch,
          basis: mitschriftRef.current ? undefined : basisRef.current,
        });
        // The base moves along: otherwise the next autosave would report a
        // conflict with its own previous save.
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
  // check, the user has been told and decided.
  const trotzdemSpeichern = async () => {
    if (!id || !page) return;
    try {
      await api.updatePage(id, {
        title: page.title,
        content: page.content,
        icon: page.icon,
      });
      setKonflikt(false);
      onMetaChange();
    } catch {
      /* stays put, and so does the hint */
    }
  };

  if (loading) return <Geruest />;
  if (!page) return <div className="empty-state">Page not found.</div>;

  const canEdit = page.canEdit;

  // Update the local copy immediately and let the save follow, so typing stays
  // responsive instead of waiting for a round trip.
  const setTitle = (title: string) => {
    setPage({ ...page, title });
    scheduleSave({ title });
  };
  // When writing together exactly one saves, namely the leader. Otherwise
  // three browsers would send the same page three times, each with its own
  // state from a blink ago, and the conflict check would trip for two of
  // them.
  const onContent = (blocks: Block[]) => {
    if (!canEdit) return;
    if (mitschrift && !mitschrift.fuehrend) return;
    scheduleSave({ content: blocks });
  };

  const toggleFav = async () => {
    if (page.isFavorite) await api.removeFavorite(page.id);
    else await api.addFavorite(page.id);
    setPage({ ...page, isFavorite: !page.isFavorite });
    onFavChange();
  };

  // The export comes from the server, not from the editor.
  //
  // The editor's own conversion is called blocksToMarkdownLossy and lives up to
  // its name: nested lists, tables, task checkboxes and code blocks only
  // partially survive it. On the server the complete document is at hand, and
  // the same path later carries the export of a whole space as well.
  const exportieren = (form: "markdown" | "pdf" | "word") => {
    if (!id) return;
    setExportOffen(false);
    // An ordinary link instead of fetch: that way the browser takes the file
    // name from the Content-Disposition header, umlauts included.
    window.location.href = `/api/pages/${id}/${form}`;
  };

  // Tags are matched by name, case-insensitively, and created only when none
  // exists. Otherwise typing "Projekt" twice would produce two separate tags.
  const addTag = async () => {
    const name = await eingabe({
      titel: "Add tag",
      feld: "Name",
      bestaetigen: "Add",
    });
    if (!name) return;
    let tag = allTags.find((t) => t.name.toLowerCase() === name.toLowerCase());
    if (!tag) {
      tag = await api.createTag(name, randomColor());
      onTagsChange();
    }
    await api.attachTag(page.id, tag.id);
    setPage({
      ...page,
      tags: [...page.tags.filter((t) => t.id !== tag!.id), tag],
    });
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
    api
      .listLinks(id)
      .then(setLinks)
      .catch(() => {});
    api
      .backlinks(id)
      .then(setBacklinks)
      .catch(() => {});
    api
      .graph()
      .then(setGraph)
      .catch(() => {});
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
          o.text = o.text.replace(re, (mm) =>
            mm.replace(/^\[\[\s*/, "").replace(/\s*\]\]$/, ""),
          );
        }
        for (const key of Object.keys(o))
          if (key !== "text") o[key] = strip(o[key]);
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
    .filter((n) =>
      (n.title || "").toLowerCase().includes(linkQuery.toLowerCase()),
    )
    .slice(0, 8);
  // Picking a suggestion adds the link and closes the box, so the next one
  // starts from an empty query.
  const pickLink = (targetId: string) => {
    addLink(targetId);
    setLinkQuery("");
    setLinkOpen(false);
  };

  // Files dropped anywhere on the page. They are passed on to the attachment
  // list, which uploads them; the drop does not have to hit the narrow list at
  // the very bottom.
  const dateiAbwurfMoeglich = canEdit && frei("anhaenge");

  // A picture in the text is an attachment of this page, nothing else. It takes
  // the same route as any other file and therefore lies wherever the instance
  // keeps its attachments, on the disk or in the bucket, and it is reachable
  // only for whoever may read the page.
  //
  // That is the whole point of doing it this way: an image block wants an
  // address, and an address into somebody else's server means the picture is
  // gone the day that server is. The list under the page shows it as well, which
  // is honest, since that is exactly what it is.
  const bildHochladen = async (datei: File) => {
    const anhang = await api.uploadAttachment(page.id, datei);
    setAnhangTick((n) => n + 1);
    return `/api/pages/${page.id}/attachments/${anhang.id}`;
  };
  const sindDateien = (e: React.DragEvent) =>
    Array.from(e.dataTransfer.types).includes("Files");

  // The page's own choice beats the default; empty means: no choice.
  const breiteWirksam = page.breite || vorgabe;

  return (
    <div
      className={"page-layout" + (ueberSeite > 0 ? " datei-abwurf" : "")}
      onDragEnter={(e) => {
        if (dateiAbwurfMoeglich && sindDateien(e)) setUeberSeite((n) => n + 1);
      }}
      onDragOver={(e) => {
        if (!sindDateien(e)) return;
        // Even though nothing is accepted here: without preventDefault the
        // browser leaves the application and opens the file. Dropping a file
        // beside the target may at most do nothing, it must not throw the work
        // away.
        e.preventDefault();
        e.dataTransfer.dropEffect = dateiAbwurfMoeglich ? "copy" : "none";
      }}
      onDragLeave={() => setUeberSeite((n) => Math.max(0, n - 1))}
      onDrop={(e) => {
        if (!sindDateien(e)) return;
        e.preventDefault();
        setUeberSeite(0);
        if (dateiAbwurfMoeglich) setEingeworfen(e.dataTransfer.files);
      }}
    >
      {ueberSeite > 0 && (
        <div className="seiten-abwurf-schleier">
          {dateiAbwurfMoeglich
            ? "Drop to attach to this page"
            : "Attachments are not possible here"}
        </div>
      )}
      <div className="page-main">
        <div className="topbar">
          <span className="topbar-title">{page.title || "Untitled"}</span>
          <div className="topbar-actions">
            {/* Say plainly that this is a read-only share, instead of leaving
                the user to wonder why nothing saves. */}
            {!canEdit && <span className="pill readonly">Read only</span>}
            {/* Who else sits at this page. Only the others: one sees one's
                own name at one's own caret anyway. If the connection is not up,
                that is said instead of showing an empty row that looks like
                "nobody there" and in truth means "I do not know". */}
            {mitschrift && !mitschrift.verbunden && (
              <span
                className="pill readonly"
                title="The connection for editing together is down right now. Typing keeps working; it syncs again as soon as the connection is back."
              >
                Nicht verbunden
              </span>
            )}
            {mitschrift?.verbunden && mitschrift.anwesend.length > 1 && (
              <span
                className="mitschreibende"
                title="On this page right now"
              >
                {mitschrift.anwesend
                  .filter((a) => !a.ichSelbst)
                  .map((a) => (
                    <span
                      key={a.kennung}
                      className="mitschreibender"
                      style={{
                        background: a.farbe,
                        color: schriftAuf(a.farbe),
                      }}
                      title={a.name}
                    >
                      {(a.name.trim()[0] || "?").toUpperCase()}
                    </span>
                  ))}
              </span>
            )}
            {/* The new page comes into being UNDER this one, not beside it.
                Whoever creates it here is inside a page and means a subpage; a
                page on the topmost level is created in the sidebar, where one
                sees the levels. That is also why the button says "Subpage" and
                not "New page": it should say where it lands. */}
            {canEdit && (
              <button className="btn" onClick={() => onCreateChild(page.id)}>
                Subpage
              </button>
            )}
            <button
              className={"btn" + (page.isFavorite ? " active" : "")}
              onClick={toggleFav}
            >
              {page.isFavorite ? "Favourited" : "Favourite"}
            </button>
            {frei("versionen") && (
              <button
                className="btn"
                onClick={() => setShowVersions((v) => !v)}
              >
                Verlauf
              </button>
            )}
            {/* How wide the text should stand. The measure used to be
                fixed, and on a wide screen a hand's breadth of paper stayed
                empty left and right while the table beside it wrapped. The
                choice hangs on the page: everybody should see a table page the
                way its author set it. */}
            {canEdit && (
              <div className="exportmenue" ref={breiteBereich}>
                <button
                  className="btn"
                  title="How wide the text sits on this page"
                  onClick={() => {
                    setExportOffen(false);
                    setBreiteOffen((v) => !v);
                  }}
                >
                  Width ▾
                </button>
                {breiteOffen && (
                  <div
                    className="klappliste"
                    onMouseLeave={() => setBreiteOffen(false)}
                  >
                    {(
                      [
                        [
                          "",
                          "Default",
                          `As the instance sets it (${vorgabeName})`,
                        ],
                        ["normal", "Normal", "Set for reading"],
                        [
                          "breit",
                          "Wide",
                          "More room for tables and images",
                        ],
                        ["voll", "Full width", "As wide as the window"],
                      ] as const
                    ).map(([wert, titel, erklaerung]) => (
                      <button
                        key={wert}
                        className={
                          "klappeintrag" +
                          ((page.breite ?? "") === wert ? " gewaehlt" : "")
                        }
                        onClick={async () => {
                          setBreiteOffen(false);
                          // Show first, save after: the width is one
                          // adjustment of the type, and waiting for the server
                          // to answer felt like a loading time.
                          setPage({ ...page, breite: wert });
                          await api.seiteBreite(page.id, wert).catch(() => {});
                        }}
                      >
                        {titel}
                        <span className="klappeintrag-hinweis">
                          {erklaerung}
                        </span>
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )}
            {/* A menu instead of three buttons: the header is full already, and
                three formats side by side say no more than one with a
                choice. */}
            <div className="exportmenue" ref={exportBereich}>
              <button className="btn" onClick={() => setExportOffen((v) => !v)}>
                Export ▾
              </button>
              {exportOffen && (
                <div
                  className="klappliste"
                  onMouseLeave={() => setExportOffen(false)}
                >
                  <button
                    className="klappeintrag"
                    onClick={() => exportieren("markdown")}
                  >
                    Markdown (.md)
                  </button>
                  {/* PDF and Word belong to the paid extras. Markdown does not:
                      getting at one's own content must never depend on a
                      licence. */}
                  {frei("export") && (
                    <>
                      <button
                        className="klappeintrag"
                        onClick={() => exportieren("pdf")}
                      >
                        PDF (.pdf)
                      </button>
                      <button
                        className="klappeintrag"
                        onClick={() => exportieren("word")}
                      >
                        Word (.docx)
                      </button>
                    </>
                  )}
                </div>
              )}
            </div>
            {page.isOwner && frei("freigeben") && (
              <button
                className={"btn" + (page.isPublic ? " active" : "")}
                onClick={() => setShowShare(true)}
              >
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
          {/* Without a choice of its own the instance default applies, and
              that stands at "voll": the measure used to be fixed, and on a wide
              screen a hand's breadth of paper stayed empty left and right while
              the table beside it wrapped. Whoever wants it narrow says so on
              the page or for the whole instance. */}
          <div
            className={
              "page" + (breiteWirksam !== "normal" ? " " + breiteWirksam : "")
            }
          >
            <input
              className="page-title"
              value={page.title}
              placeholder="Untitled"
              disabled={!canEdit}
              onChange={(e) => setTitle(e.target.value)}
            />
            <div className="page-tags">
              {page.tags.map((t) => (
                <span
                  key={t.id}
                  className="tag"
                  // Text colour computed from the tag colour: fixed white was
                  // hard to read on the lighter tones of the palette.
                  style={{ background: t.color, color: schriftAuf(t.color) }}
                >
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
            {/* A block the editor cannot read would otherwise take the whole
                window with it: React unmounts everything on an uncaught render
                error. Here only the document goes, the page around it stays. */}
            <Fehlergrenze
              key={`grenze:${page.id}:${editorKey}:${mitschrift ? "gemeinsam" : "allein"}`}
              text="The content of this page could not be shown."
            >
              <Suspense fallback={<div className="qv-none">Loading…</div>}>
                <Editor
                  /* The key carries along whether there is writing together: the
                   editor is built once, and whether its text comes from the page
                   or from the shared document is decided at build time. The wire
                   is only up a blink of an eye after opening, so without the key
                   the decision always falls on "alone". */
                  key={`${page.id}:${editorKey}:${mitschrift ? "gemeinsam" : "allein"}`}
                  mitschrift={mitschrift ?? undefined}
                  initialContent={page.content}
                  editable={canEdit}
                  onChange={onContent}
                  onEditorReady={(e) => (editorRef.current = e)}
                  linkResolver={resolveLink}
                  onOpenLink={(pid) => nav(`/page/${pid}`)}
                  mentionTargets={graph.nodes.filter((n) => n.id !== page.id)}
                  dateiHochladen={
                    dateiAbwurfMoeglich ? bildHochladen : undefined
                  }
                />
              </Suspense>
            </Fehlergrenze>
            {frei("anhaenge") && (
              <Attachments
                pageId={page.id}
                neuLaden={anhangTick}
                canEdit={canEdit}
                eingeworfen={eingeworfen}
                onEingeworfenFertig={() => setEingeworfen(null)}
              />
            )}
            {frei("kommentare") && <Kommentare pageId={page.id} />}
            {(links.length > 0 || textLinkTitles.length > 0 || canEdit) && (
              <div className="page-links">
                <div className="page-links-title">Links</div>
                <div className="page-links-list">
                  {links.map((l) => (
                    <span key={l.id} className="page-link-chip">
                      <button
                        className="page-link-open"
                        onClick={() => nav(`/page/${l.id}`)}
                      >
                        {l.title || "Untitled"}
                      </button>
                      {canEdit && (
                        <span
                          className="x"
                          title="Remove link"
                          onClick={() => removeLink(l.id)}
                        >
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
                        title="From the text ([[…]] / @ mention)"
                      >
                        {tid ? (
                          <button
                            className="page-link-open"
                            onClick={() => nav(`/page/${tid}`)}
                          >
                            {t}
                          </button>
                        ) : (
                          <span className="page-link-open muted">{t}</span>
                        )}
                        {canEdit && (
                          <span
                            className="x"
                            title="Remove the link from the text"
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
                        placeholder="+ Find a page and link it…"
                        value={linkQuery}
                        onChange={(e) => {
                          setLinkQuery(e.target.value);
                          setLinkOpen(true);
                        }}
                        onFocus={() => setLinkOpen(true)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter" && filteredCandidates[0])
                            pickLink(filteredCandidates[0].id);
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
                              {n.title || "Untitled"}
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                </div>
                {links.length === 0 && textLinkTitles.length === 0 && (
                  <div className="page-links-hint">
                    No links yet. Pick a page above, or type @ or [[…]] in the
                    text.
                  </div>
                )}
              </div>
            )}
            {backlinks.length > 0 && (
              <div className="backlinks">
                <div className="backlinks-title">
                  Linked from {backlinks.length}{" "}
                  {backlinks.length === 1 ? "page" : "pages"}
                </div>
                <div className="backlinks-list">
                  {backlinks.map((b) => (
                    <button
                      key={b.id}
                      className="backlink"
                      onClick={() => nav(`/page/${b.id}`)}
                    >
                      {b.title || "Untitled"}
                    </button>
                  ))}
                </div>
              </div>
            )}
            <LocalGraph
              graph={graph}
              pageId={page.id}
              onOpen={(pid) => nav(`/page/${pid}`)}
            />
          </div>
        </div>
      </div>

      {konflikt && (
        <div className="konflikt-banner">
          <div>
            <strong>
              Die Seite wurde inzwischen an anderer Stelle geändert.
            </strong>
            <div className="muted small">
              Automatisches Speichern ist angehalten, damit nichts überschrieben
              wird.
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
          onPublicChange={(isPublic, token) =>
            setPage({ ...page, isPublic, publicToken: token })
          }
          onClose={() => setShowShare(false)}
        />
      )}
    </div>
  );
}
