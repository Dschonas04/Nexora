// Typed client for the backend API, plus the wire types it returns. These
// mirror the Go structs in backend/internal/models: changing a JSON tag there
// means changing the matching field here.

// User is the public view of an account; the password hash never leaves the
// backend, so there is no field for it.
export interface User {
  id: string;
  email: string;
  name: string;
  role: string;
  createdAt: string;
}

export interface Tag {
  id: string;
  name: string;
  color: string;
  /** How many pages carry this tag. Zero means it is orphaned. */
  anzahl: number;
}

/** Die Konfigurationsdatei, wie die Wartungsseite sie sieht. */
export interface KonfigDatei {
  pfad: string;
  /** Inhalt mit unkenntlich gemachten Zugangsdaten. */
  inhalt: string;
  gefunden: boolean;
  schreibbar: boolean;
  hinweise: string[];
  /** Alle Schlüssel, die diese Fassung auswertet. */
  schluessel: string[];
  /** Schlüssel, deren Werte versteckt werden. */
  geheimnisse: string[];
}

export interface Space {
  id: string;
  ownerId: string;
  name: string;
  createdAt: string;
  /** True when the space belongs to someone else and is visible through a right. */
  fremd: boolean;
  /**
   * Sichtbarkeit der Ablage für die übrigen angemeldeten Konten der Instanz.
   * "nein" heißt: nur Eigentümer und ausdrücklich Berechtigte. Mit dem
   * Freigabelink einer einzelnen Seite hat das nichts zu tun -- öffentlich
   * meint hier die Instanz, nicht das Internet.
   */
  oeffentlich: "nein" | "lesen" | "schreiben";
  /** Ob dieses Konto die Ablage verwalten darf (Sichtbarkeit, Rechte). */
  darfVerwalten: boolean;
}

// PageMeta is the light shape used for the sidebar, search results and lists.
// It has no content, which is what keeps those requests small.
export interface PageMeta {
  id: string;
  parentId: string | null;
  spaceId: string | null;
  title: string;
  icon: string;
  shared: boolean;
  updatedAt: string;
}

// Page is one page in full. content is unknown because it is a BlockNote
// document that only the editor interprets. canEdit, isOwner and isFavorite are
// computed per viewer, so the same page arrives differently for different users.
export interface Page {
  id: string;
  ownerId: string;
  parentId: string | null;
  spaceId: string | null;
  title: string;
  content: unknown;
  icon: string;
  isPublic: boolean;
  publicToken: string | null;
  tags: Tag[];
  isFavorite: boolean;
  canEdit: boolean;
  isOwner: boolean;
  /** Whether this page is offered as a template when creating new ones. */
  istVorlage: boolean;
  createdAt: string;
  updatedAt: string;
}

// PublicPage is the reduced shape behind a public link: no ids, no owner,
// nothing that would give away workspace structure.
export interface PublicPage {
  title: string;
  content: unknown;
  icon: string;
  updatedAt: string;
}

// PageVersion is one entry in a page's history. content is optional because the
// list endpoint omits it and only a single fetched version carries it.
export interface PageVersion {
  id: string;
  title: string;
  content?: unknown;
  icon: string;
  authorName: string;
  createdAt: string;
}

export interface Attachment {
  id: string;
  pageId: string;
  filename: string;
  mime: string;
  size: number;
  createdAt: string;
}

export interface ShareEntry {
  userId: string;
  name: string;
  email: string;
  permission: string;
}

export interface GraphNode {
  id: string;
  title: string;
  spaceId: string | null;
  space: string;
}

// GraphEdge connects two nodes; kind is "parent" for nesting and "link" for an
// explicit or typed link.
export interface GraphEdge {
  source: string;
  target: string;
  kind: string;
}

export interface Graph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

// req is the single fetch wrapper every call goes through.
//
// credentials: "include" sends the session cookie, which is the only reason
// requests are authenticated at all: the token is httpOnly and unreadable here.
//
// It unwraps the backend's {"error": "..."} shape into a thrown Error and hangs
// the HTTP status on it, so callers can tell a 403 from a 404 when they care.
async function req<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(`/api${path}`, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...opts,
  });
  if (!res.ok) {
    // Fall back to the status text when the body is not the expected JSON,
    // which happens for errors produced by the proxy rather than the backend.
    let msg = res.statusText;
    try {
      const j = await res.json();
      if (j && j.error) msg = j.error;
    } catch {
      /* ignore */
    }
    const err = new Error(msg) as Error & { status?: number };
    err.status = res.status;
    throw err;
  }
  // Some endpoints answer 200 with an empty body; JSON.parse would throw on it.
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

// Lizenz reports which paid extras this installation has unlocked. The names
// match the constants in the backend's internal/lizenz package; the browser
// deliberately keeps no list of its own, it renders whatever the server sends.
export interface Lizenz {
  gueltig: boolean;
  inhaber: string;
  laeuft_ab: string;
  grund: string;
  alle_extras: string[];
  freigeschaltet: string[];
}

// Spureintrag is one row of the audit trail. Names and titles are frozen copies
// taken when the action happened, so an entry stays readable after the account
// or page it refers to is gone.
export interface Spureintrag {
  id: number;
  zeitpunkt: string;
  akteurId: string;
  akteurName: string;
  akteurEmail: string;
  aktion: string;
  objektArt: string;
  objektId: string;
  objektTitel: string;
  details: Record<string, unknown>;
  ip: string;
}

// Kommentar is one entry on a page. A deleted one keeps its place in the thread
// with an empty text, so replies hanging off it do not lose their context.
export interface Kommentar {
  id: string;
  elternId: string | null;
  autorId: string;
  autorName: string;
  text: string;
  erledigt: boolean;
  erstelltAm: string;
  geaendertAm: string | null;
  geloescht: boolean;
  /** Whether the signed-in account may edit or delete this one. */
  darf: boolean;
}

// Einstellung is one runtime setting, with the description the backend supplies.
// The page renders whatever it gets rather than keeping its own list, which
// would drift from the server's.
export interface Einstellung {
  schluessel: string;
  wert: string;
  art: "janein" | "zahl" | "text" | "liste";
  titel: string;
  erklaerung: string;
  warnung?: string;
  vorgabe: string;
  /** true while the value still comes from config.conf, untouched here. */
  ausDatei: boolean;
  geaendertVon?: string;
  geaendertAm?: string;
}

// SystemZustand is the read-only half of the settings page.
export interface SystemZustand {
  lizenz: {
    gueltig: boolean;
    inhaber: string;
    laeuftAb: string;
    grund: string;
    freigeschaltet: string[];
    alle: number;
  };
  zahlen: Record<string, number>;
  datenbank: {
    groesse: string;
    version: string;
    tabellen: { name: string; zeilen: number; platz: string }[];
  };
  anhaengeBytes: number;
  sicherheit: {
    admins: { name: string; email: string }[];
    letzteAnmeldung: string;
    letzterFehlversuch: string;
    fehlversuche24h: number;
    registrierungOffen: boolean;
    erlaubteDomaenen: string[] | null;
    sitzungTage: number;
  };
  nurInDerDatei: {
    port: string;
    datenVerzeichnis: string;
    oeffentlicheUrl: string;
    ldapAktiv: boolean;
    ldapServer: string;
    oidcAktiv: boolean;
    oidcAussteller: string;
  };
  warnungen: string[];
}

// Gruppe is a group of accounts. Groups belong to the installation, not to an
// account: a department is not a private matter.
export interface Gruppe {
  id: string;
  name: string;
  beschreibung: string;
  erstelltAm: string;
  mitglieder: number;
}

export interface Mitglied {
  id: string;
  name: string;
  email: string;
  rolle: string;
  /** Whether this account is currently in the group. */
  drin: boolean;
}

// SpaceRecht grants a group or a single account access to a space.
// recht is a ladder: lesen < schreiben < verwalten.
export interface SpaceRecht {
  gruppeId: string | null;
  gruppeName: string;
  userId: string | null;
  userName: string;
  recht: "lesen" | "schreiben" | "verwalten";
  erteiltAm: string;
}

// SearchHit is one full text result. Unlike PageMeta it carries the snippet the
// database produced, so the sidebar can show where the term actually sat.
// EinfuhrBericht sagt, was aus einer Einfuhr geworden ist. Die Warnungen sind
// der wichtigste Teil: was übergangen wurde, muss sichtbar sein, sonst hält man
// eine halbe Einfuhr für eine ganze.
export interface EinfuhrBericht {
  seiten: number;
  anhaenge: number;
  wurzeln: string[];
  warnungen: string[];
  /** Gesetzt, wenn die Einfuhr eine eigene Ablage angelegt hat. */
  ablage?: { id: string; name: string };
}

// EinfuhrAst ist ein Knoten der Vorschau -- der Baum, wie er entstehen würde.
// PapierkorbSeite trägt zusätzlich den Tag, an dem sie von selbst verschwindet.
// null heißt: die Instanz löscht nichts von selbst.
export interface PapierkorbSeite extends PageMeta {
  verfaelltAm: string | null;
}

export interface EinfuhrAst {
  titel: string;
  quelle: string;
  kinder?: EinfuhrAst[];
}

export interface EinfuhrVorschau {
  seiten: number;
  beilagen: number;
  baum: EinfuhrAst[];
  warnungen: string[];
  /** Name der Ablage, die entstehen würde. */
  ablage?: string;
}

// SuchFilter grenzt eine Suche ein. Alle Felder sind freiwillig; leer heißt
// "keine Einschränkung".
// Nachricht ist ein Eintrag im Postfach.
export interface Nachricht {
  id: string;
  art: "kommentar" | "antwort" | "erwaehnung" | "freigabe";
  pageId: string | null;
  kommentarId: string | null;
  ausloeserName: string;
  seitenTitel: string;
  text: string;
  gelesenAm: string | null;
  erstelltAm: string;
}

export interface SuchFilter {
  /** Kennung einer Ablage, oder "ohne" für Seiten ohne Ablage. */
  space?: string;
  tag?: string;
  /** Nur Seiten, die in den letzten n Tagen geändert wurden. */
  tage?: number;
  /** "ich" beschränkt auf eigene Seiten. */
  wer?: string;
}

export interface SearchHit {
  id: string;
  parentId: string | null;
  title: string;
  icon: string;
  /**
   * The snippet, with <b> around the matched words. Everything else is already
   * escaped by the database, but it is still rendered as text and marked up by
   * the component -- never fed to dangerouslySetInnerHTML.
   */
  ausschnitt: string;
  /** false means the page was reached through a share or as an admin. */
  eigen: boolean;
  /**
   * Name of the attachment the hit came from, empty when the page text itself
   * matched. Without it a snippet from a PDF would read like page content that
   * is nowhere to be seen on the page.
   */
  quelle: string;
  updatedAt: string;
}

// PagePatch is a partial update: an omitted field stays as it is, while an
// explicit null on parentId or spaceId clears it. That distinction is why the
// autosave can send just the content without touching the page's position.
export interface PagePatch {
  title?: string;
  content?: unknown;
  icon?: string;
  parentId?: string | null;
  spaceId?: string | null;
  /**
   * The updatedAt this editor last saw. The backend refuses with 409 when the
   * page moved on since, instead of overwriting a colleague's edit in silence.
   * Omit it to force the write through.
   */
  basis?: string;
}

// One entry per endpoint. Deliberately a plain object of thin functions rather
// than a generated client, so the API surface stays readable at a glance.
export const api = {
  me: () => req<User>("/auth/me"),
  login: (email: string, password: string) =>
    req<User>("/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }),
  register: (email: string, name: string, password: string) =>
    req<User>("/auth/register", { method: "POST", body: JSON.stringify({ email, name, password }) }),
  logout: () => req<void>("/auth/logout", { method: "POST" }),

  // Read once after sign-in. Hiding locked features is a courtesy to the
  // reader, not a protection: the backend refuses the same calls with 402
  // regardless of what the interface shows.
  lizenz: () => req<Lizenz>("/lizenz"),

  pruefspur: (p: { aktion?: string; akteur?: string; objekt?: string; limit?: number } = {}) => {
    const q = new URLSearchParams();
    if (p.aktion) q.set("aktion", p.aktion);
    if (p.akteur) q.set("akteur", p.akteur);
    if (p.objekt) q.set("objekt", p.objekt);
    if (p.limit) q.set("limit", String(p.limit));
    return req<Spureintrag[]>(`/pruefspur?${q.toString()}`);
  },
  pruefspurAktionen: () => req<{ aktion: string; anzahl: number }[]>("/pruefspur/aktionen"),

  design: () => req<{ grundton: string; akzent: string }>("/design"),

  einstellungen: () => req<Einstellung[]>("/einstellungen"),
  einstellungSetzen: (schluessel: string, wert: string) =>
    req<{ wert: string }>("/einstellungen", {
      method: "PUT",
      body: JSON.stringify({ schluessel, wert }),
    }),
  einstellungZuruecksetzen: (schluessel: string) =>
    req<{ wert: string }>(`/einstellungen?schluessel=${encodeURIComponent(schluessel)}`, {
      method: "DELETE",
    }),
  systemZustand: () => req<SystemZustand>("/system"),
  suchindexNeu: () => req<{ ohneSuchtext: number }>("/system/suchindex", { method: "POST" }),
  anhangindexNachziehen: () =>
    req<{ betrachtet: number; gelesen: number; ohneText: number }>("/system/anhangindex", {
      method: "POST",
    }),
  ablageZustand: () => req<{ ablage: string }>("/system/ablage"),
  ablageTesten: (p: {
    endpunkt: string;
    bucket: string;
    zugriff: string;
    geheimnis: string;
    region: string;
    tls: boolean;
    pfadstil: boolean;
  }) =>
    req<{ ok: boolean; ablage?: string; schritt?: string; grund?: string; anmerkung?: string }>(
      "/system/ablage/test",
      { method: "POST", body: JSON.stringify(p) },
    ),

  kommentare: (pageId: string) => req<Kommentar[]>(`/pages/${pageId}/kommentare`),
  kommentarAnlegen: (pageId: string, text: string, elternId?: string) =>
    req<Kommentar>(`/pages/${pageId}/kommentare`, {
      method: "POST",
      body: JSON.stringify({ text, elternId: elternId ?? null }),
    }),
  kommentarAendern: (id: string, text: string) =>
    req<void>(`/kommentare/${id}`, { method: "PUT", body: JSON.stringify({ text }) }),
  kommentarLoeschen: (id: string) => req<void>(`/kommentare/${id}`, { method: "DELETE" }),
  kommentarErledigt: (id: string) =>
    req<{ erledigt: boolean }>(`/kommentare/${id}/erledigt`, { method: "POST" }),

  listPages: () => req<PageMeta[]>("/pages"),
  listShared: () => req<PageMeta[]>("/pages/shared"),
  listTrash: () => req<PapierkorbSeite[]>("/pages/trash"),
  createPage: (parentId?: string | null, spaceId?: string | null, vorlageId?: string) =>
    req<Page>("/pages", {
      method: "POST",
      body: JSON.stringify({
        parentId: parentId ?? null,
        spaceId: spaceId ?? null,
        vorlageId: vorlageId ?? "",
      }),
    }),

  vorlagen: () => req<PageMeta[]>("/vorlagen"),

  gruppen: () => req<Gruppe[]>("/gruppen"),
  gruppeAnlegen: (name: string, beschreibung: string) =>
    req<Gruppe>("/gruppen", { method: "POST", body: JSON.stringify({ name, beschreibung }) }),
  gruppeLoeschen: (id: string) => req<void>(`/gruppen/${id}`, { method: "DELETE" }),
  gruppenMitglieder: (id: string) => req<Mitglied[]>(`/gruppen/${id}/mitglieder`),
  mitgliedSetzen: (gruppeId: string, userId: string, drin: boolean) =>
    req<{ drin: boolean }>(`/gruppen/${gruppeId}/mitglieder`, {
      method: "PUT",
      body: JSON.stringify({ userId, drin }),
    }),

  spaceRechte: (spaceId: string) => req<SpaceRecht[]>(`/spaces/${spaceId}/rechte`),
  spaceRechtSetzen: (
    spaceId: string,
    ziel: { gruppeId?: string; userId?: string },
    recht: string,
  ) =>
    req<{ recht: string }>(`/spaces/${spaceId}/rechte`, {
      method: "PUT",
      body: JSON.stringify({ gruppeId: ziel.gruppeId ?? "", userId: ziel.userId ?? "", recht }),
    }),
  vorlageUmschalten: (id: string) =>
    req<{ istVorlage: boolean }>(`/pages/${id}/vorlage`, { method: "POST" }),
  getPage: (id: string) => req<Page>(`/pages/${id}`),
  backlinks: (id: string) => req<PageMeta[]>(`/pages/${id}/backlinks`),

  // Manual page-to-page links (edited via the UI, independent of [[wiki-links]])
  listLinks: (id: string) => req<PageMeta[]>(`/pages/${id}/links`),
  addLink: (id: string, targetId: string) =>
    req<void>(`/pages/${id}/links`, { method: "POST", body: JSON.stringify({ targetId }) }),
  removeLink: (id: string, targetId: string) =>
    req<void>(`/pages/${id}/links/${targetId}`, { method: "DELETE" }),
  updatePage: (id: string, patch: PagePatch) =>
    req<Page>(`/pages/${id}`, { method: "PUT", body: JSON.stringify(patch) }),
  deletePage: (id: string) => req<void>(`/pages/${id}`, { method: "DELETE" }),
  restorePage: (id: string) => req<void>(`/pages/${id}/restore`, { method: "POST" }),
  purgePage: (id: string) => req<void>(`/pages/${id}/purge`, { method: "DELETE" }),

  addFavorite: (id: string) => req<void>(`/pages/${id}/favorite`, { method: "POST" }),
  removeFavorite: (id: string) => req<void>(`/pages/${id}/favorite`, { method: "DELETE" }),
  listFavorites: () => req<PageMeta[]>("/favorites"),

  sharePage: (id: string) =>
    req<{ isPublic: boolean; publicToken: string }>(`/pages/${id}/share`, { method: "POST" }),
  unsharePage: (id: string) => req<{ isPublic: boolean }>(`/pages/${id}/share`, { method: "DELETE" }),

  // Version history
  listVersions: (id: string) => req<PageVersion[]>(`/pages/${id}/versions`),
  getVersion: (id: string, versionId: string) =>
    req<PageVersion>(`/pages/${id}/versions/${versionId}`),
  restoreVersion: (id: string, versionId: string) =>
    req<Page>(`/pages/${id}/versions/${versionId}/restore`, { method: "POST" }),

  // Attachments
  listAttachments: (id: string) => req<Attachment[]>(`/pages/${id}/attachments`),
  // Uploads bypass req because they send FormData: setting Content-Type by hand
  // would omit the multipart boundary the browser has to generate.
  uploadAttachment: async (id: string, file: File) => {
    const body = new FormData();
    body.append("file", file);
    const res = await fetch(`/api/pages/${id}/attachments`, {
      method: "POST",
      credentials: "include",
      body,
    });
    if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || res.statusText);
    return (await res.json()) as Attachment;
  },
  // Used directly as an img/iframe src, so the browser fetches it with the
  // session cookie and the backend still checks access.
  attachmentUrl: (id: string, attId: string) => `/api/pages/${id}/attachments/${attId}`,
  deleteAttachment: (id: string, attId: string) =>
    req<void>(`/pages/${id}/attachments/${attId}`, { method: "DELETE" }),

  // Einfuhr: eine oder mehrere Markdown-Dateien, oder ein ZIP mit Struktur.
  // Wie beim Anhang an req vorbei, aus demselben Grund -- FormData setzt der
  // Browser samt Grenzmarke selbst.
  importieren: async (
    dateien: File[],
    // neueAblage schließt die beiden anderen aus: das Archiv bringt dann
    // seine eigene Ablage mit, statt sich in eine vorhandene zu mischen.
    ziel: { parentId?: string; spaceId?: string; neueAblage?: string },
    vorschau = false,
  ) => {
    const body = new FormData();
    for (const d of dateien) body.append("file", d);
    if (ziel.parentId) body.append("parentId", ziel.parentId);
    if (ziel.spaceId) body.append("spaceId", ziel.spaceId);
    if (ziel.neueAblage) body.append("neueAblage", ziel.neueAblage);
    // Derselbe Aufruf mit demselben Inhalt, nur ohne Folgen -- der Server
    // rechnet denselben Plan und legt nichts an.
    if (vorschau) body.append("vorschau", "1");
    const res = await fetch(`/api/import`, { method: "POST", credentials: "include", body });
    if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || res.statusText);
    return (await res.json()) as EinfuhrBericht & EinfuhrVorschau;
  },

  // Postfach
  postfach: (nurUngelesen = false) =>
    req<Nachricht[]>(`/postfach${nurUngelesen ? "?ungelesen=1" : ""}`),
  postfachAnzahl: () => req<{ ungelesen: number }>("/postfach/anzahl"),
  postfachGelesen: (id?: string) =>
    req<{ ok: boolean }>(id ? `/postfach/${id}/gelesen` : "/postfach/gelesen", { method: "POST" }),
  postfachLeeren: () => req<{ geloescht: number }>("/postfach", { method: "DELETE" }),

  // Per-user sharing + roles
  listShares: (id: string) => req<ShareEntry[]>(`/pages/${id}/shares`),
  addShare: (id: string, email: string, permission: string) =>
    req<{ userId: string; permission: string }>(`/pages/${id}/shares`, {
      method: "POST",
      body: JSON.stringify({ email, permission }),
    }),
  removeShare: (id: string, userId: string) =>
    req<void>(`/pages/${id}/shares/${userId}`, { method: "DELETE" }),
  listUsers: () => req<User[]>("/users"),
  createUser: (email: string, name: string, password: string, role: string) =>
    req<User>("/users", {
      method: "POST",
      body: JSON.stringify({ email, name, password, role }),
    }),
  deleteUser: (id: string) => req<void>(`/users/${id}`, { method: "DELETE" }),
  setUserRole: (id: string, role: string) =>
    req<{ role: string }>(`/users/${id}/role`, { method: "PUT", body: JSON.stringify({ role }) }),

  // Wartung: Konfigurationsdatei, Neustart, Papierkorb der Instanz
  konfigLesen: () => req<KonfigDatei>("/system/konfig"),
  konfigPruefen: (inhalt: string) =>
    req<{ hinweise: string[] }>("/system/konfig", {
      method: "PUT",
      body: JSON.stringify({ inhalt, nurPruefen: true }),
    }),
  konfigSchreiben: (inhalt: string) =>
    req<{ hinweise: string[]; sicherung: string; neustartNoetig: boolean }>("/system/konfig", {
      method: "PUT",
      body: JSON.stringify({ inhalt }),
    }),
  neustarten: () =>
    req<{ ok: boolean }>("/system/neustart", {
      method: "POST",
      body: JSON.stringify({ bestaetigung: "neustart" }),
    }),
  papierkorbLeeren: () =>
    req<{ geloescht: number }>("/system/papierkorb", { method: "POST" }),

  // Spaces
  listSpaces: () => req<Space[]>("/spaces"),
  createSpace: (name: string) =>
    req<Space>("/spaces", { method: "POST", body: JSON.stringify({ name }) }),
  renameSpace: (id: string, name: string) =>
    req<void>(`/spaces/${id}`, { method: "PUT", body: JSON.stringify({ name }) }),
  deleteSpace: (id: string) => req<void>(`/spaces/${id}`, { method: "DELETE" }),
  spaceOeffentlich: (id: string, oeffentlich: "nein" | "lesen" | "schreiben") =>
    req<{ oeffentlich: string }>(`/spaces/${id}/oeffentlich`, {
      method: "PUT",
      body: JSON.stringify({ oeffentlich }),
    }),

  // Knowledge graph
  graph: () => req<Graph>("/graph"),

  listTags: () => req<Tag[]>("/tags"),
  createTag: (name: string, color: string) =>
    req<Tag>("/tags", { method: "POST", body: JSON.stringify({ name, color }) }),
  seitenZuTag: (id: string) => req<PageMeta[]>(`/tags/${id}/pages`),
  deleteTag: (id: string) => req<void>(`/tags/${id}`, { method: "DELETE" }),
  attachTag: (pageId: string, tagId: string) =>
    req<void>(`/pages/${pageId}/tags`, { method: "POST", body: JSON.stringify({ tagId }) }),
  detachTag: (pageId: string, tagId: string) =>
    req<void>(`/pages/${pageId}/tags/${tagId}`, { method: "DELETE" }),

  // encodeURIComponent matters here: a query may contain &, # or a slash.
  // Filter als Parameter statt als Suchsprache im Feld: wer "space:technik"
  // tippen muss, tippt es falsch.
  search: (q: string, filter?: SuchFilter) => {
    const p = new URLSearchParams({ q });
    if (filter?.space) p.set("space", filter.space);
    if (filter?.tag) p.set("tag", filter.tag);
    if (filter?.tage) p.set("tage", String(filter.tage));
    if (filter?.wer) p.set("wer", filter.wer);
    return req<SearchHit[]>(`/search?${p.toString()}`);
  },
  getPublicPage: (token: string) => req<PublicPage>(`/public/${token}`),
};
