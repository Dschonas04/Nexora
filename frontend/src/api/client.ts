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
  /** All keys this version evaluates. */
  schluessel: string[];
  /** Keys whose values are hidden. */
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
   * Visibility of the space to the remaining logged in accounts of the
   * instance. "nein" means: only the owner and those explicitly entitled. This
   * has nothing to do with the share link of a single page; public here means
   * the instance, not the internet.
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
  /** Satzspiegel: "normal", "breit" oder "voll". Gehört zur Seite, nicht zum Leser. */
  breite: Seitenbreite;
  createdAt: string;
  updatedAt: string;
}

// PublicPage is the reduced shape behind a public link: no ids, no owner,
// nothing that would give away workspace structure.
export interface PublicPage {
  title: string;
  content: unknown;
  icon: string;
  breite: Seitenbreite;
  updatedAt: string;
}

/** Die drei Satzspiegel. Eine feste Liste: hinter den Namen stehen Werte im
    Stilblatt, eine freie Pixelangabe wäre eine Zahl, die niemand mehr prüft. */
export type Seitenbreite = "normal" | "breit" | "voll";

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
  /** The tier that was sold, if the key names one. */
  stufe?: string;
  laeuft_ab: string;
  grund: string;
  alle_extras: string[];
  freigeschaltet: string[];
  /** What each tier contains, so the interface keeps no second table. */
  stufen?: { name: string; funktionen: string[] }[];
  /** True only at the issuer: that is where the private signing key lies. */
  ausstellbar?: boolean;
}

// Sitzung ist eine gespeicherte Anmeldung. "diese" markiert die, mit der
// gerade gearbeitet wird, ohne sie beendet man leicht sich selbst.
export interface Sitzung {
  id: string;
  angelegtAm: string;
  zuletztAm: string;
  laeuftAb: string;
  ip: string;
  browser: string;
  diese: boolean;
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
/** Ein Konto, so wie es in der @-Auswahl steht. Nur der Name: eine Erwähnung
    ist der Name im Text, und die Adresse ginge niemanden etwas an. */
export interface Person {
  name: string;
}

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

// Dienst is one part of the compound: the services the backend talks to. The
// Docker compound itself is NOT here; seeing it would mean giving the container
// the control channel, and the application is not worth that.
export interface Dienst {
  name: string;
  rolle: string;
  adresse: string;
  zustand: string;
  fassung?: string;
  antwort?: string;
  hinweis?: string;
  notwendig: boolean;
}

// SystemZustand is the read-only half of the settings page.
export interface SystemZustand {
  verbund?: Dienst[];
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
    sitzungStunden: number;
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

// EinfuhrBericht says what became of an import. The warnings are the most
// important part: what was skipped has to be visible, otherwise one takes half
// an import for a whole one.
export interface EinfuhrBericht {
  seiten: number;
  anhaenge: number;
  wurzeln: string[];
  warnungen: string[];
  /** Gesetzt, wenn die Einfuhr eine eigene Ablage angelegt hat. */
  ablage?: { id: string; name: string };
}

// EinfuhrAst is a node of the preview, the tree as it would come into being.
// PapierkorbSeite additionally carries the day on which it disappears by itself.
// null means: the instance deletes nothing by itself.
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
  /** Name of the space that would come into being. */
  ablage?: string;
}

// SuchFilter narrows a search. All fields are optional; empty means "no
// restriction".
// Nachricht is one entry in the inbox.
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
  /** Id of a space, or "ohne" for pages without a space. */
  space?: string;
  tag?: string;
  /** Only pages changed within the last n days. */
  tage?: number;
  /** "ich" restricts to one's own pages. */
  wer?: string;
}

// SearchHit is one full text result. Unlike PageMeta it carries the snippet the
// database produced, so the sidebar can show where the term actually sat.
export interface SearchHit {
  id: string;
  parentId: string | null;
  title: string;
  icon: string;
  /**
   * The snippet, with <b> around the matched words. Everything else is already
   * escaped by the database, but it is still rendered as text and marked up by
   * the component, never fed to dangerouslySetInnerHTML.
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

  // What the login page may offer. Public, because nobody is signed in here
  // yet.
  ssoZustand: () =>
    req<{
      oidc: boolean;
      oidcText: string;
      ldap: boolean;
      passwort: boolean;
      anbieter: string;
    }>("/auth/sso"),
  ldapAnmelden: (benutzer: string, passwort: string) =>
    req<User>("/auth/ldap", { method: "POST", body: JSON.stringify({ benutzer, passwort }) }),

  // Word attachments: read as editor blocks and write back as .docx.
  wordLesen: (seiteId: string, anhangId: string) =>
    req<{ titel: string; bloecke: unknown[] }>(`/pages/${seiteId}/attachments/${anhangId}/word`),
  wordSchreiben: (seiteId: string, anhangId: string, titel: string, bloecke: unknown) =>
    req<{ ok: boolean; bytes: number }>(`/pages/${seiteId}/attachments/${anhangId}/word`, {
      method: "PUT",
      body: JSON.stringify({ titel, bloecke }),
    }),

  sitzungen: () => req<Sitzung[]>("/sitzungen"),
  sitzungBeenden: (id: string) => req<void>(`/sitzungen/${id}`, { method: "DELETE" }),
  sitzungenBeenden: () => req<{ beendet: number }>("/sitzungen", { method: "DELETE" }),

  // Read once after sign-in. Hiding locked features is a courtesy to the
  // reader, not a protection: the backend refuses the same calls with 402
  // regardless of what the interface shows.
  lizenz: () => req<Lizenz>("/lizenz"),
  lizenzEinlesen: (schluessel: string) =>
    req<Lizenz>("/system/lizenz", { method: "PUT", body: JSON.stringify({ schluessel }) }),
  lizenzAusstellen: (p: {
    inhaber: string;
    stufe: string;
    funktionen?: string[];
    ablauf?: string;
  }) => req<{ schluessel: string }>("/system/lizenz/ausstellen", {
    method: "POST",
    body: JSON.stringify(p),
  }),

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
  // Wen man in einem Kommentar zu dieser Seite mit @ ansprechen kann: genau die
  // Konten, die sie lesen dürfen. Die anderen bekämen ohnehin keine Nachricht.
  erwaehnbare: (pageId: string) => req<Person[]>(`/pages/${pageId}/erwaehnbare`),
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
  createPage: (parentId?: string | null, spaceId?: string | null) =>
    req<Page>("/pages", {
      method: "POST",
      body: JSON.stringify({
        parentId: parentId ?? null,
        spaceId: spaceId ?? null,
      }),
    }),

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
  // Der Satzspiegel einer Seite. Wer schreiben darf, darf ihn setzen: es ist
  // eine Eigenschaft des Textes, keine der Freigabe.
  seiteBreite: (id: string, breite: Seitenbreite) =>
    req<{ breite: Seitenbreite }>(`/pages/${id}/breite`, {
      method: "PUT",
      body: JSON.stringify({ breite }),
    }),
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
  // Hanging a page somewhere else and placing it among its new siblings, in one
  // call: a drag in the sidebar is one gesture, and two requests could half
  // fail. vorId is the sibling it lands in front of; null means the end.
  seiteVerschieben: (
    id: string,
    ziel: { elternId?: string | null; spaceId?: string | null; vorId?: string | null },
  ) => req<Page>(`/pages/${id}/reihenfolge`, { method: "PUT", body: JSON.stringify(ziel) }),
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
  //
  // Through XMLHttpRequest instead of fetch, for exactly one reason: fetch
  // reports no progress while sending. With a file of twenty megabytes an upload
  // without a bar looks like a hang, and one clicks a second time.
  uploadAttachment: (id: string, file: File, fortschritt?: (anteil: number) => void) =>
    new Promise<Attachment>((fertig, fehler) => {
      const body = new FormData();
      body.append("file", file);
      const x = new XMLHttpRequest();
      x.open("POST", `/api/pages/${id}/attachments`);
      x.withCredentials = true;
      x.upload.onprogress = (e) => {
        // lengthComputable is false as long as the browser does not know the
        // total size; every fraction is then guessed, and guessed is worse than
        // indefinite.
        if (e.lengthComputable && e.total > 0) fortschritt?.(e.loaded / e.total);
      };
      x.onload = () => {
        if (x.status >= 200 && x.status < 300) {
          try {
            fertig(JSON.parse(x.responseText) as Attachment);
          } catch {
            fehler(new Error("Antwort war kein JSON"));
          }
          return;
        }
        let text = x.statusText;
        try {
          const j = JSON.parse(x.responseText);
          if (j && j.error) text = j.error;
        } catch {
          /* dann eben der Statustext */
        }
        const e = new Error(text) as Error & { status?: number };
        e.status = x.status;
        fehler(e);
      };
      x.onerror = () => fehler(new Error("Verbindung abgebrochen"));
      x.onabort = () => fehler(new Error("Abgebrochen"));
      x.send(body);
    }),
  // Used directly as an img/iframe src, so the browser fetches it with the
  // session cookie and the backend still checks access.
  attachmentUrl: (id: string, attId: string) => `/api/pages/${id}/attachments/${attId}`,
  deleteAttachment: (id: string, attId: string) =>
    req<void>(`/pages/${id}/attachments/${attId}`, { method: "DELETE" }),

  // Einfuhr: eine oder mehrere Markdown-Dateien, oder ein ZIP mit Struktur.
  // Wie beim Anhang an req vorbei, aus demselben Grund, FormData setzt der
  // Browser samt Grenzmarke selbst.
  importieren: async (
    dateien: File[],
    // neueAblage excludes the other two: the archive then brings a space of its
    // own instead of mixing into an existing one.
    ziel: { parentId?: string; spaceId?: string; neueAblage?: string },
    vorschau = false,
  ) => {
    const body = new FormData();
    for (const d of dateien) body.append("file", d);
    if (ziel.parentId) body.append("parentId", ziel.parentId);
    if (ziel.spaceId) body.append("spaceId", ziel.spaceId);
    if (ziel.neueAblage) body.append("neueAblage", ziel.neueAblage);
    // Derselbe Aufruf mit demselben Inhalt, nur ohne Folgen, der Server
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
  // The order of the sidebar, as a complete list. It is kept per account: a
  // space open to the whole instance stands in everybody's sidebar, and whoever
  // drags it must not rearrange it for the others.
  spacesOrdnen: (ids: string[]) =>
    req<void>("/spaces/reihenfolge", { method: "PUT", body: JSON.stringify({ ids }) }),
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
