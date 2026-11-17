// Nexora without a server: the pages live on this device, in the browser
// storage of the app (IndexedDB), for the person signed in to the device. No
// account, no password; whoever uses the device's user account uses this
// Nexora.
//
// Used by the apps (desktop and Android) when "Without a server" was chosen
// at setup. api/client.ts hands every call to anfrage() instead of the
// network. What only makes sense with a server (sharing, groups, comments,
// administration, attachments) answers with an empty list or a short note.

const MODUS_SCHLUESSEL = "nexora-modus";
const DB = "nexora-lokal";
const ABLAGE = "stand";

/** Whether this window runs Nexora without a server. */
export function lokalModus(): boolean {
  if (typeof window === "undefined") return false;
  if (location.protocol === "app:") return true;
  try {
    return location.hostname === "localhost" && localStorage.getItem(MODUS_SCHLUESSEL) === "lokal";
  } catch {
    return false;
  }
}

interface LSeite {
  id: string;
  parentId: string | null;
  spaceId: string | null;
  title: string;
  content: unknown;
  icon: string;
  width: string;
  position: number;
  createdAt: string;
  updatedAt: string;
  deletedAt: string | null;
  tags: string[];
  links: string[];
}

interface LVersion {
  id: string;
  title: string;
  icon: string;
  content: unknown;
  createdAt: string;
}

interface Stand {
  name: string;
  seiten: Record<string, LSeite>;
  versionen: Record<string, LVersion[]>;
  spaces: { id: string; name: string; color: string; createdAt: string }[];
  tags: { id: string; name: string; color: string }[];
  favoriten: string[];
  vorlagen: { id: string; title: string; icon: string; content: unknown; createdAt: string }[];
  design: { baseColor: string; accent: string; pageWidth: string; language?: string };
}

const leer = (): Stand => ({
  name: "",
  seiten: {},
  versionen: {},
  spaces: [],
  tags: [],
  favoriten: [],
  vorlagen: [],
  design: { baseColor: "", accent: "", pageWidth: "normal" },
});

// ---------------------------------------------------------------- storage --

function oeffnen(): Promise<IDBDatabase> {
  return new Promise((ok, nein) => {
    const r = indexedDB.open(DB, 1);
    r.onupgradeneeded = () => r.result.createObjectStore(ABLAGE);
    r.onsuccess = () => ok(r.result);
    r.onerror = () => nein(r.error);
  });
}

let stand: Stand | null = null;
let laden: Promise<Stand> | null = null;

async function holen(): Promise<Stand> {
  if (stand) return stand;
  laden ??= (async () => {
    const db = await oeffnen();
    const wert = await new Promise<Stand | undefined>((ok, nein) => {
      const r = db.transaction(ABLAGE).objectStore(ABLAGE).get("stand");
      r.onsuccess = () => ok(r.result as Stand | undefined);
      r.onerror = () => nein(r.error);
    });
    stand = { ...leer(), ...(wert || {}) };
    if (!stand.name) stand.name = await geraeteName();
    return stand;
  })();
  return laden;
}

// Writes one after another, so a later state never lands before an earlier.
let schreibKette: Promise<void> = Promise.resolve();
function sichern(): Promise<void> {
  const kopie = JSON.parse(JSON.stringify(stand));
  schreibKette = schreibKette.then(async () => {
    const db = await oeffnen();
    await new Promise<void>((ok, nein) => {
      const t = db.transaction(ABLAGE, "readwrite");
      t.objectStore(ABLAGE).put(kopie, "stand");
      t.oncomplete = () => ok();
      t.onerror = () => nein(t.error);
    });
  });
  return schreibKette;
}

declare global {
  interface Window {
    nexoraEinrichtung?: { benutzer?: () => Promise<string>; zurueck?: () => Promise<void> };
  }
}

async function geraeteName(): Promise<string> {
  try {
    const n = await window.nexoraEinrichtung?.benutzer?.();
    if (n) return n;
  } catch {
    /* no desktop bridge */
  }
  return "Ich";
}

/** Leave the mode without a server; the pages stay on the device. */
export async function lokalVerlassen() {
  try {
    localStorage.removeItem(MODUS_SCHLUESSEL);
  } catch {
    /* nothing to remove */
  }
  if (window.nexoraEinrichtung?.zurueck) await window.nexoraEinrichtung.zurueck();
  else location.replace("/");
}

export function lokalWaehlen() {
  localStorage.setItem(MODUS_SCHLUESSEL, "lokal");
}

// ---------------------------------------------------------------- shapes --

const jetzt = () => new Date().toISOString();
const neueId = () => crypto.randomUUID();
const ICH = "lokal";

function ich(s: Stand) {
  return { id: ICH, email: "", name: s.name, username: "", role: "user", createdAt: "" };
}

function meta(s: LSeite) {
  return {
    id: s.id,
    parentId: s.parentId,
    spaceId: s.spaceId,
    title: s.title,
    icon: s.icon,
    shared: false,
    updatedAt: s.updatedAt,
  };
}

function lebende(st: Stand) {
  return Object.values(st.seiten)
    .filter((s) => !s.deletedAt)
    .sort((a, b) => a.position - b.position || a.createdAt.localeCompare(b.createdAt));
}

function voll(st: Stand, s: LSeite) {
  return {
    id: s.id,
    ownerId: ICH,
    parentId: s.parentId,
    spaceId: s.spaceId,
    title: s.title,
    content: s.content,
    icon: s.icon,
    isPublic: false,
    publicToken: null,
    tags: s.tags.map((t) => st.tags.find((x) => x.id === t)).filter(Boolean).map((t) => ({ ...t!, count: zahl(st, t!.id) })),
    isFavorite: st.favoriten.includes(s.id),
    watching: false,
    canEdit: true,
    isOwner: true,
    collaborative: false,
    width: s.width || "normal",
    createdAt: s.createdAt,
    updatedAt: s.updatedAt,
  };
}

const zahl = (st: Stand, tagId: string) => lebende(st).filter((s) => s.tags.includes(tagId)).length;

/** All text of a BlockNote document, for search and backlinks. */
function text(inhalt: unknown): string {
  const teile: string[] = [];
  const gehen = (x: unknown) => {
    if (!x) return;
    if (typeof x === "string") return;
    if (Array.isArray(x)) return x.forEach(gehen);
    if (typeof x === "object") {
      const o = x as Record<string, unknown>;
      if (o.type === "text" && typeof o.text === "string") teile.push(o.text);
      gehen(o.content);
      gehen(o.children);
      if (o.content && typeof o.content === "object" && "rows" in (o.content as object)) {
        gehen((o.content as { rows: unknown }).rows);
      }
      if (Array.isArray(o.cells)) gehen(o.cells);
    }
  };
  gehen(inhalt);
  return teile.join(" ");
}

class Fehler extends Error {
  status: number;
  constructor(status: number, nachricht: string) {
    super(nachricht);
    this.status = status;
  }
}

const NUR_SERVER = "Without a server this is not available.";

// ---------------------------------------------------------------- the API --

export async function anfrage<T>(pfad: string, opts: RequestInit = {}): Promise<T> {
  const st = await holen();
  const m = (opts.method || "GET").toUpperCase();
  const url = new URL(pfad, "http://lokal");
  const p = url.pathname;
  const k = typeof opts.body === "string" && opts.body ? JSON.parse(opts.body) : {};
  const teil = p.split("/").filter(Boolean);
  const seite = teil[0] === "pages" && teil[1] && st.seiten[teil[1]] ? st.seiten[teil[1]] : null;
  const r = (x: unknown) => x as T;

  const veraendert = async (x: unknown) => {
    await sichern();
    return r(x);
  };

  // --- account and look
  if (p === "/auth/me") return r(ich(st));
  if (p === "/auth/logout") {
    await lokalVerlassen();
    return r(undefined);
  }
  if (p === "/auth/sso") return r({ oidc: false, oidcButtonText: "", ldap: false, registration: false });
  if (p === "/auth/two-factor") return r({ enabled: false, required: false });
  if (p === "/profile" && m === "PUT") {
    st.name = String(k.name || "").trim() || st.name;
    return veraendert({ ok: true, name: st.name });
  }
  if (p === "/design" && m === "GET") return r(st.design);
  if (p === "/design" && m === "PUT") {
    st.design = { ...st.design, baseColor: k.baseColor ?? "", accent: k.accent ?? "" };
    return veraendert({ baseColor: st.design.baseColor, accent: st.design.accent });
  }
  if (p === "/design/language") {
    st.design = { ...st.design, language: k.language ?? "" };
    return veraendert({ language: st.design.language });
  }
  if (p === "/license") {
    return r({ valid: false, holder: "", expiresAt: "", reason: "", allExtras: [], unlocked: [] });
  }
  if (p === "/account/notifications") return r({ email: false, available: false, address: "" });
  if (p === "/inbox/count") return r({ unread: 0 });
  if (p.startsWith("/postfach") || p === "/sessions" || p === "/inbox") return r(m === "DELETE" ? { deleted: 0 } : []);

  // --- lists
  if (p === "/pages" && m === "GET") return r(lebende(st).map(meta));
  if (p === "/pages/shared") return r([]);
  if (p === "/pages/trash") {
    return r(
      Object.values(st.seiten)
        .filter((s) => s.deletedAt)
        .map((s) => ({ ...meta(s), expiresAt: null })),
    );
  }
  if (p === "/favorites") return r(st.favoriten.map((id) => st.seiten[id]).filter((s) => s && !s.deletedAt).map((s) => meta(s!)));
  if (p === "/graph") {
    const leben = lebende(st);
    const raum = (id: string | null) => st.spaces.find((s) => s.id === id)?.name ?? "";
    const kanten: { source: string; target: string; kind: string }[] = [];
    for (const s of leben) {
      if (s.parentId && st.seiten[s.parentId] && !st.seiten[s.parentId].deletedAt) {
        kanten.push({ source: s.parentId, target: s.id, kind: "parent" });
      }
      for (const z of s.links) if (st.seiten[z] && !st.seiten[z].deletedAt) kanten.push({ source: s.id, target: z, kind: "link" });
    }
    return r({ nodes: leben.map((s) => ({ id: s.id, title: s.title, spaceId: s.spaceId, space: raum(s.spaceId) })), edges: kanten });
  }
  if (p === "/search") {
    const q = (url.searchParams.get("q") || "").toLowerCase().trim();
    const tag = url.searchParams.get("tag");
    const raum = url.searchParams.get("space");
    const tage = Number(url.searchParams.get("days") || 0);
    const grenze = tage ? Date.now() - tage * 86400000 : 0;
    const treffer = lebende(st).filter((s) => {
      if (tag && !s.tags.includes(tag)) return false;
      if (raum && s.spaceId !== raum) return false;
      if (grenze && Date.parse(s.updatedAt) < grenze) return false;
      return !q || s.title.toLowerCase().includes(q) || text(s.content).toLowerCase().includes(q);
    });
    return r(
      treffer.slice(0, 50).map((s) => {
        const t = text(s.content);
        const i = q ? t.toLowerCase().indexOf(q) : -1;
        const stueck = i >= 0 ? t.slice(Math.max(0, i - 40), i + q.length + 60) : t.slice(0, 100);
        return { id: s.id, parentId: s.parentId, title: s.title, icon: s.icon, snippet: stueck, own: true, source: "", updatedAt: s.updatedAt };
      }),
    );
  }

  // --- pages
  if (p === "/pages" && m === "POST") {
    const zeit = jetzt();
    const neu: LSeite = {
      id: neueId(),
      parentId: k.parentId ?? null,
      spaceId: k.spaceId ?? (k.parentId && st.seiten[k.parentId] ? st.seiten[k.parentId].spaceId : null),
      title: "",
      content: [],
      icon: "",
      width: "",
      position: Math.max(0, ...Object.values(st.seiten).map((s) => s.position)) + 1,
      createdAt: zeit,
      updatedAt: zeit,
      deletedAt: null,
      tags: [],
      links: [],
    };
    st.seiten[neu.id] = neu;
    return veraendert(voll(st, neu));
  }
  if (teil[0] === "pages" && teil[1] && !seite) throw new Fehler(404, "Page not found.");
  if (seite) {
    const rest = teil.slice(2).join("/");
    if (rest === "" && m === "GET") return r(voll(st, seite));
    if (rest === "" && m === "PUT") {
      // A version before the change, at most one every five minutes, so the
      // history does not fill up with one entry per keystroke.
      const v = (st.versionen[seite.id] ||= []);
      const letzte = v[v.length - 1];
      if (!letzte || Date.now() - Date.parse(letzte.createdAt) > 300000) {
        v.push({ id: neueId(), title: seite.title, icon: seite.icon, content: seite.content, createdAt: jetzt() });
        if (v.length > 50) v.shift();
      }
      for (const f of ["title", "content", "icon"] as const) if (f in k) (seite as unknown as Record<string, unknown>)[f] = k[f];
      if ("parentId" in k) seite.parentId = k.parentId;
      if ("spaceId" in k) seite.spaceId = k.spaceId;
      seite.updatedAt = jetzt();
      return veraendert(voll(st, seite));
    }
    if (rest === "" && m === "DELETE") {
      const weg = (id: string) => {
        st.seiten[id].deletedAt = jetzt();
        for (const s of Object.values(st.seiten)) if (s.parentId === id && !s.deletedAt) weg(s.id);
      };
      weg(seite.id);
      return veraendert(undefined);
    }
    if (rest === "restore") {
      seite.deletedAt = null;
      if (seite.parentId && st.seiten[seite.parentId]?.deletedAt) seite.parentId = null;
      return veraendert(undefined);
    }
    if (rest === "purge") {
      delete st.seiten[seite.id];
      delete st.versionen[seite.id];
      st.favoriten = st.favoriten.filter((x) => x !== seite.id);
      return veraendert(undefined);
    }
    if (rest === "width") {
      seite.width = k.width;
      return veraendert({ width: k.width });
    }
    if (rest === "order") {
      if ("parentId" in k) seite.parentId = k.parentId ?? null;
      if ("spaceId" in k) seite.spaceId = k.spaceId ?? null;
      const geschwister = lebende(st).filter((s) => s.id !== seite.id && s.parentId === seite.parentId && s.spaceId === seite.spaceId);
      const vor = k.beforeId ? geschwister.findIndex((s) => s.id === k.beforeId) : -1;
      geschwister.splice(vor >= 0 ? vor : geschwister.length, 0, seite);
      geschwister.forEach((s, i) => (s.position = i + 1));
      seite.updatedAt = jetzt();
      return veraendert(voll(st, seite));
    }
    if (rest === "favorite") {
      st.favoriten = st.favoriten.filter((x) => x !== seite.id);
      if (m === "POST") st.favoriten.push(seite.id);
      return veraendert(undefined);
    }
    if (rest === "watch") return r(undefined);
    if (rest === "tags" && m === "POST") {
      if (!seite.tags.includes(k.tagId)) seite.tags.push(k.tagId);
      return veraendert(undefined);
    }
    if (teil[2] === "tags" && teil[3] && m === "DELETE") {
      seite.tags = seite.tags.filter((t) => t !== teil[3]);
      return veraendert(undefined);
    }
    if (rest === "links" && m === "GET") return r(seite.links.map((z) => st.seiten[z]).filter((s) => s && !s.deletedAt).map((s) => meta(s!)));
    if (rest === "links" && m === "POST") {
      if (!seite.links.includes(k.targetId)) seite.links.push(k.targetId);
      return veraendert(undefined);
    }
    if (teil[2] === "links" && teil[3] && m === "DELETE") {
      seite.links = seite.links.filter((z) => z !== teil[3]);
      return veraendert(undefined);
    }
    if (rest === "backlinks") {
      const titel = seite.title.trim().toLowerCase();
      return r(
        lebende(st)
          .filter((s) => s.id !== seite.id)
          .filter((s) => {
            if (s.links.includes(seite.id)) return true;
            const roh = JSON.stringify(s.content);
            if (roh.includes(seite.id)) return true;
            return !!titel && text(s.content).toLowerCase().includes(`[[${titel}]]`);
          })
          .map(meta),
      );
    }
    if (rest === "versions") {
      return r(
        [...(st.versionen[seite.id] || [])]
          .reverse()
          .map((v) => ({ id: v.id, title: v.title, icon: v.icon, authorName: st.name, createdAt: v.createdAt })),
      );
    }
    if (teil[2] === "versions" && teil[3]) {
      const v = (st.versionen[seite.id] || []).find((x) => x.id === teil[3]);
      if (!v) throw new Fehler(404, "Version not found.");
      if (teil[4] === "restore") {
        seite.title = v.title;
        seite.icon = v.icon;
        seite.content = v.content;
        seite.updatedAt = jetzt();
        return veraendert(voll(st, seite));
      }
      return r({ ...v, authorName: st.name });
    }
    if (rest === "collaborators") return r({ count: 0, possible: false });
    if (["attachments", "comments", "mentionable", "shares"].includes(rest) && m === "GET") return r([]);
    throw new Fehler(503, NUR_SERVER);
  }

  // --- spaces
  if (p === "/spaces" && m === "GET") {
    return r(
      st.spaces.map((s) => ({ ...s, ownerId: ICH, foreign: false, publicAccess: "nein", canManage: true })),
    );
  }
  if (p === "/spaces" && m === "POST") {
    const neu = { id: neueId(), name: String(k.name || "").trim(), color: "", createdAt: jetzt() };
    st.spaces.push(neu);
    return veraendert({ ...neu, ownerId: ICH, foreign: false, publicAccess: "nein", canManage: true });
  }
  if (p === "/spaces/order") {
    const ids: string[] = k.ids || [];
    st.spaces.sort((a, b) => ids.indexOf(a.id) - ids.indexOf(b.id));
    return veraendert(undefined);
  }
  if (teil[0] === "spaces" && teil[1]) {
    const raum = st.spaces.find((s) => s.id === teil[1]);
    if (!raum) throw new Fehler(404, "Space not found.");
    if (teil.length === 2 && m === "PUT") {
      raum.name = String(k.name || "").trim() || raum.name;
      return veraendert(undefined);
    }
    if (teil.length === 2 && m === "DELETE") {
      st.spaces = st.spaces.filter((s) => s.id !== raum.id);
      for (const s of Object.values(st.seiten)) if (s.spaceId === raum.id) s.deletedAt ??= jetzt();
      return veraendert(undefined);
    }
    if (teil[2] === "color") {
      raum.color = k.color || "";
      return veraendert({ color: raum.color });
    }
    if (teil[2] === "permissions" && m === "GET") return r([]);
    throw new Fehler(503, NUR_SERVER);
  }

  // --- tags
  if (p === "/tags" && m === "GET") return r(st.tags.map((t) => ({ ...t, count: zahl(st, t.id) })));
  if (p === "/tags" && m === "POST") {
    const name = String(k.name || "").trim();
    if (st.tags.some((t) => t.name.toLowerCase() === name.toLowerCase())) {
      throw new Fehler(409, "A tag with this name already exists.");
    }
    const neu = { id: neueId(), name, color: k.color || "#6b7280" };
    st.tags.push(neu);
    return veraendert({ ...neu, count: 0 });
  }
  if (teil[0] === "tags" && teil[1]) {
    const t = st.tags.find((x) => x.id === teil[1]);
    if (!t) throw new Fehler(404, "Tag not found.");
    if (teil[2] === "pages") return r(lebende(st).filter((s) => s.tags.includes(t.id)).map(meta));
    if (m === "PATCH") {
      if (k.name) {
        const name = String(k.name).trim();
        if (st.tags.some((x) => x.id !== t.id && x.name.toLowerCase() === name.toLowerCase())) {
          throw new Fehler(409, "A tag with this name already exists.");
        }
        t.name = name;
      }
      if (k.color) t.color = k.color;
      return veraendert({ ...t, count: zahl(st, t.id) });
    }
    if (m === "DELETE") {
      st.tags = st.tags.filter((x) => x.id !== t.id);
      for (const s of Object.values(st.seiten)) s.tags = s.tags.filter((x) => x !== t.id);
      return veraendert(undefined);
    }
  }

  // --- templates
  if (p === "/templates" && m === "GET") {
    return r(st.vorlagen.map((v) => ({ ...v, forAll: false, own: true, ownerName: st.name })));
  }
  if (p === "/templates" && m === "POST") {
    const neu = { id: neueId(), title: k.title || "", icon: k.icon || "", content: k.content, createdAt: jetzt() };
    st.vorlagen.push(neu);
    return veraendert({ ...neu, forAll: false, own: true, ownerName: st.name });
  }
  if (teil[0] === "templates" && teil[1] && m === "DELETE") {
    st.vorlagen = st.vorlagen.filter((v) => v.id !== teil[1]);
    return veraendert(undefined);
  }
  if (p === "/system/trash") {
    const weg = Object.values(st.seiten).filter((s) => s.deletedAt);
    for (const s of weg) delete st.seiten[s.id];
    return veraendert({ deleted: weg.length });
  }

  if (m === "GET" && ["/groups", "/users", "/api-keys", "/settings"].includes(p)) return r([]);
  throw new Fehler(503, NUR_SERVER);
}
