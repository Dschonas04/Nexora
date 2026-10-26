// Typed client for the backend API, plus the wire types it returns. These
// mirror the Go structs in backend/internal/models: changing a JSON tag there
// means changing the matching field here.

// User is the public view of an account; the password hash never leaves the
// backend, so there is no field for it.
export interface User {
  id: string;
  email: string;
  name: string;
  // The sign-in name. May be empty: accounts from SSO and from older versions
  // have none, and signing in then goes on through the address.
  username: string;
  role: string;
  createdAt: string;
  /**
   * When the profile picture was last set; missing when there is none.
   * Answers both at once: WHETHER a picture is there, and WHICH -- the
   * timestamp hangs on the address so a new one appears at once.
   */
  imageVersion?: string;
  /** Whether a second factor is set up on this account. */
  twoFactor?: boolean;
}

/**
 * The answer to a sign-in. Either the account -- then the session already sits
 * in the cookie -- or the prompt for the second step together with a ticket.
 * Both in one type, because the server decides it in one place and the
 * interface has to make the distinction anyway.
 */
export type Anmeldung = User | { secondStep: true; ticket: string };

export const brauchtZweitenSchritt = (
  a: Anmeldung,
): a is { secondStep: true; ticket: string } => "secondStep" in a;

/** Der Stand des zweiten Faktors am eigenen Konto. */
export interface ZweitfaktorStand {
  active: boolean;
  since?: string;
  /** Recovery codes not yet used up. */
  remaining: number;
  /** Ob die Instanz ihn von jedem Konto verlangt. */
  required: boolean;
}

export interface Tag {
  id: string;
  name: string;
  color: string;
  /** How many pages carry this tag. Zero means it is orphaned. */
  count: number;
}

/** The configuration file as the maintenance page sees it. */
export interface KonfigDatei {
  path: string;
  /** Inhalt mit unkenntlich gemachten Zugangsdaten. */
  content: string;
  found: boolean;
  writable: boolean;
  notes: string[];
  /** All keys this version evaluates. */
  key: string[];
  /** Keys whose values are hidden. */
  secrets: string[];
}

export interface Space {
  id: string;
  ownerId: string;
  name: string;
  createdAt: string;
  /** True when the space belongs to someone else and is visible through a right. */
  foreign: boolean;
  /**
   * Visibility of the space to the remaining logged in accounts of the
   * instance. "nein" means: only the owner and those explicitly entitled. This
   * has nothing to do with the share link of a single page; public here means
   * the instance, not the internet.
   */
  publicAccess: "nein" | "lesen" | "schreiben";
  /** Whether this account may administer the space (visibility, permissions). */
  canManage: boolean;
  /** Colour in the graph as #rrggbb. Empty/missing: the interface hands out one. */
  color?: string;
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
  /** The viewer gets an inbox message when somebody else changes the page. */
  watching: boolean;
  canEdit: boolean;
  isOwner: boolean;
  /**
   * More than one account may write on this page, and the service permits
   * editing together. Only then does the browser open the wire for it.
   */
  collaborative: boolean;
  /** Measure: "normal", "breit" or "voll". Belongs to the page, not to the reader. */
  width: Seitenbreite;
  createdAt: string;
  updatedAt: string;
}

// PublicPage is the reduced shape behind a public link: no ids, no owner,
// nothing that would give away workspace structure.
export interface PublicPage {
  title: string;
  content: unknown;
  icon: string;
  width: Seitenbreite;
  updatedAt: string;
}

/** The three measures. A fixed list: behind the names sit values in the
    stylesheet, a free pixel count would be a number nobody checks any more. */
// The empty value means: no choice of one's own, the instance default from
// /design applies. It is the initial state of every page.
export type Seitenbreite = "" | "normal" | "breit" | "voll";

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
  valid: boolean;
  holder: string;
  /** The tier that was sold, if the key names one. */
  tier?: string;
  expiresAt: string;
  reason: string;
  allExtras: string[];
  unlocked: string[];
  /** What each tier contains, so the interface keeps no second table. */
  tiers?: { name: string; features: string[] }[];
  /** True only at the issuer: that is where the private signing key lies. */
  canIssue?: boolean;
}

// Sitzung is a stored sign-in. "diese" marks the one currently being worked
// with; without it one easily ends one's own.
export interface Sitzung {
  id: string;
  createdAt: string;
  lastSeenAt: string;
  expiresAt: string;
  ip: string;
  browser: string;
  current: boolean;
}

// Spureintrag is one row of the audit trail. Names and titles are frozen copies
// taken when the action happened, so an entry stays readable after the account
// or page it refers to is gone.
export interface Spureintrag {
  id: number;
  timestamp: string;
  actorId: string;
  actorName: string;
  actorEmail: string;
  action: string;
  objectType: string;
  objectId: string;
  objectTitle: string;
  details: Record<string, unknown>;
  ip: string;
}

// Kommentar is one entry on a page. A deleted one keeps its place in the thread
// with an empty text, so replies hanging off it do not lose their context.
/** An account as it appears in the @ dropdown. The name and nothing else: a
    mention is the name in the text, and the address would be nobody's business. */
export interface Person {
  name: string;
}

/** A saved template. The built-in ones live in vorlagen.ts. */
export interface Vorlage {
  id: string;
  title: string;
  icon: string;
  content: unknown;
  forAll: boolean;
  own: boolean;
  ownerName: string;
  createdAt: string;
}

/** A personal API key. key is only there in the answer that made it. */
export interface APISchluessel {
  id: string;
  name: string;
  hint: string;
  scope: "lesen" | "schreiben";
  createdAt: string;
  lastUsedAt: string | null;
  expiresAt: string | null;
  key?: string;
}

export interface Webhook {
  id: string;
  name: string;
  url: string;
  events: string[];
  active: boolean;
  /** Only in the answer that made the hook. */
  secret?: string;
  secretHint: string;
  createdAt: string;
  lastAt: string | null;
  lastStatus: number;
  lastError: string;
}

export interface Kommentar {
  id: string;
  parentId: string | null;
  authorId: string;
  authorName: string;
  text: string;
  resolved: boolean;
  createdAt: string;
  updatedAt: string | null;
  deleted: boolean;
  /** Whether the signed-in account may edit or delete this one. */
  canEdit: boolean;
}

// Einstellung is one runtime setting, with the description the backend supplies.
// The page renders whatever it gets rather than keeping its own list, which
// would drift from the server's.
export interface Einstellung {
  key: string;
  value: string;
  type: "janein" | "zahl" | "text" | "liste";
  title: string;
  description: string;
  warning?: string;
  default: string;
  /** Environment variable that overrides this key at start; absent when none does. */
  fromEnv?: string;
  /** true while the value still comes from config.conf, untouched here. */
  fromFile: boolean;
  updatedBy?: string;
  updatedAt?: string;
}

// Dienst is one part of the compound: the services the backend talks to. The
// Docker compound itself is NOT here; seeing it would mean giving the container
// the control channel, and the application is not worth that.
export interface Dienst {
  name: string;
  role: string;
  address: string;
  state: string;
  version?: string;
  response?: string;
  note?: string;
  required: boolean;
}

// SystemZustand is the read-only half of the settings page.
export interface SystemZustand {
  services?: Dienst[];
  license: {
    valid: boolean;
    holder: string;
    expiresAt: string;
    reason: string;
    unlocked: string[];
    all: number;
  };
  counts: Record<string, number>;
  database: {
    size: string;
    version: string;
    tables: { name: string; rows: number; bytes: string }[];
  };
  attachmentBytes: number;
  security: {
    admins: { name: string; email: string }[];
    lastSignIn: string;
    lastFailedAttempt: string;
    failedAttempts24h: number;
    registrationOpen: boolean;
    allowedDomains: string[] | null;
    sessionHours: number;
  };
  fileOnly: {
    port: string;
    dataDirectory: string;
    publicUrl: string;
    ldapEnabled: boolean;
    ldapServer: string;
    oidcEnabled: boolean;
    oidcIssuer: string;
  };
  warnings: string[];
}

// Anmeldeversuch is one attempt to sign in, successful or not. Every way in
// writes one: the password form, the directory, the identity provider.
export interface Anmeldeversuch {
  timestamp: string;
  success: boolean;
  login: string;
  name: string;
  accountId: string;
  ip: string;
  method: string;
  reason: string;
  browser: string;
}

// Herkunft is one address with its attempts summed up over the last week.
export interface Herkunft {
  ip: string;
  attempts: number;
  failed: number;
  accounts: number;
  last: string;
}

export interface Anmeldungen {
  attempts: Anmeldeversuch[];
  sources: Herkunft[];
  days: number;
  summary: {
    successes24h: number;
    failures24h: number;
    successes7d: number;
    failures7d: number;
    addresses24h: number;
    lastSignIn: string | null;
    lastFailedAttempt: string | null;
  };
}

// LDAPEinrichtung is how the directory is set up in config.conf. The service
// account's password is not part of it, only whether one is set.
export interface LDAPEinrichtung {
  active: boolean;
  licensed: boolean;
  server: string;
  startTLS: boolean;
  tlsVerify: boolean;
  bindDN: string;
  bindPasswordSet: boolean;
  baseDN: string;
  userFilter: string;
  fieldName: string;
  fieldEmail: string;
  adminGroup: string;
  encrypted: boolean;
}

export interface LDAPBefund {
  dn: string;
  name: string;
  email: string;
  admin: boolean;
  groups: string[];
  passwordChecked: boolean;
}

export interface LDAPTestErgebnis {
  ok: boolean;
  errors?: string;
  note?: string;
  result?: LDAPBefund;
}

// Puls is the live state: the last minute in per-second slots, plus the
// connection pool and the process. The current second is missing from it, it
// has only partly passed.
// MitschriftZustand is the administration's view of writing together: which
// pages are open right now and who sits at them. Only the moment, nothing
// stored: a room ends as soon as the last person leaves.
// One machine of the overview: what was entered and what was just measured in
// one piece. Zustand is "antwortet", "still" or "unbekannt".
export interface Rechner {
  id: string;
  name: string;
  target: string;
  comment?: string;
  state: string;
  response?: string;
  note?: string;
  // What the machine itself said when knocked on.
  version?: string;
  certificate?: string;
  daysUntilExpiry?: number;
}

export interface RechnerEingabe {
  name: string;
  target: string;
  comment?: string;
}

export interface RechnerListe {
  hosts: Rechner[];
  checkedAt?: string;
}

export interface MitschriftZustand {
  enabled: boolean;
  licensed: boolean;
  limit: number;
  rooms: { page: string; title: string; count: number; who: string[] }[];
}

export interface PulsSekunde {
  secondsAgo: number;
  requests: number;
  errors: number;
  rejected: number;
  avgMs: number;
  maxMs: number;
}

export interface Puls {
  measuredAt: string;
  requests?: {
    inFlight: number;
    total: number;
    uptimeSec: number;
    minute: PulsSekunde[];
    perSecond: number;
    avgMs: number;
    peakMs: number;
    errors: number;
    rejected: number;
  };
  pool: {
    limit: number;
    open: number;
    inUse: number;
    idle: number;
    acquires: number;
    waitCount: number;
    avgWaitMs: number;
    connecting: number;
  };
  process: {
    goroutines: number;
    memoryMB: number;
    systemMB: number;
    gcRuns: number;
    cores: number;
  };
  database: {
    size: string;
    hitRatio: number | null;
    connections: number;
  };
}

// SicherungUmfang says in advance what would go into a backup. In advance,
// because with a body of several gigabytes nobody should press the button and
// then guess whether it is stuck or merely taking a while.
export interface SicherungUmfang {
  tokenSet: boolean;
  token: string;
  script: string;
  databaseBytes: number;
  attachments: number;
  attachmentBytes: number;
  storage: string;
  estimatedBytes: number;
  ready: boolean;
  errors: string;
}

// Gruppe is a group of accounts. Groups belong to the installation, not to an
// account: a department is not a private matter.
export interface Gruppe {
  id: string;
  name: string;
  description: string;
  createdAt: string;
  members: number;
}

export interface Mitglied {
  id: string;
  name: string;
  email: string;
  role: string;
  /** Whether this account is currently in the group. */
  isMember: boolean;
}

// SpaceRecht grants a group or a single account access to a space.
// recht is a ladder: lesen < schreiben < verwalten.
export interface SpaceRecht {
  groupId: string | null;
  groupName: string;
  userId: string | null;
  userName: string;
  permission: "lesen" | "schreiben" | "verwalten";
  grantedAt: string;
}

// EinfuhrBericht says what became of an import. The warnings are the most
// important part: what was skipped has to be visible, otherwise one takes half
// an import for a whole one.
export interface EinfuhrBericht {
  pages: number;
  attachments: number;
  roots: string[];
  warnings: string[];
  /** Set when the import created a space of its own. */
  space?: { id: string; name: string };
}

// EinfuhrAst is a node of the preview, the tree as it would come into being.
// PapierkorbSeite additionally carries the day on which it disappears by itself.
// null means: the instance deletes nothing by itself.
export interface PapierkorbSeite extends PageMeta {
  expiresAt: string | null;
}

export interface EinfuhrAst {
  title: string;
  source: string;
  children?: EinfuhrAst[];
}

export interface EinfuhrVorschau {
  pages: number;
  files: number;
  tree: EinfuhrAst[];
  warnings: string[];
  /** Name of the space that would come into being. */
  space?: string;
}

// SuchFilter narrows a search. All fields are optional; empty means "no
// restriction".
// Nachricht is one entry in the inbox.
export interface Nachricht {
  id: string;
  type: "kommentar" | "antwort" | "erwaehnung" | "freigabe" | "aenderung" | "aufgabe";
  pageId: string | null;
  commentId: string | null;
  /** Who triggered it. Null when the account was deleted. */
  actorId: string | null;
  actorName: string;
  /** Timestamp of this account's profile picture; missing when it has none. */
  actorImage?: string;
  pageTitle: string;
  text: string;
  readAt: string | null;
  createdAt: string;
}

export interface SuchFilter {
  /** Id of a space, or "ohne" for pages without a space. */
  space?: string;
  tag?: string;
  /** Only pages changed within the last n days. */
  days?: number;
  /** "ich" restricts to one's own pages. */
  who?: string;
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
  snippet: string;
  /** false means the page was reached through a share or as an admin. */
  own: boolean;
  /**
   * Name of the attachment the hit came from, empty when the page text itself
   * matched. Without it a snippet from a PDF would read like page content that
   * is nowhere to be seen on the page.
   */
  source: string;
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
  baseVersion?: string;
}

// One entry per endpoint. Deliberately a plain object of thin functions rather
// than a generated client, so the API surface stays readable at a glance.
export const api = {
  me: () => req<User>("/auth/me"),
  // kennung ist Adresse oder Benutzername; welches von beidem, entscheidet der
  // Server am @.
  login: (kennung: string, password: string) =>
    req<Anmeldung>("/auth/login", { method: "POST", body: JSON.stringify({ login: kennung, password }) }),
  register: (email: string, name: string, password: string, benutzername = "") =>
    req<User>("/auth/register", {
      method: "POST",
      body: JSON.stringify({ email, name, password, username: benutzername }),
    }),
  logout: () => req<void>("/auth/logout", { method: "POST" }),

  // What the login page may offer. Public, because nobody is signed in here
  // yet.
  ssoZustand: () =>
    req<{
      oidc: boolean;
      oidcButtonText: string;
      ldap: boolean;
      password: boolean;
      provider: string;
    }>("/auth/sso"),
  ldapAnmelden: (benutzer: string, passwort: string) =>
    req<Anmeldung>("/auth/ldap", { method: "POST", body: JSON.stringify({ user: benutzer, password: passwort }) }),

  // The second factor.
  //
  // The flow has three calls because it has three states: start creates a
  // secret that is not yet in force, an switches it on with the first correct
  // code and returns the recovery codes while doing so, aus takes it back.
  // Without separating start and an, an aborted setup -- a look at the QR code,
  // then a closed tab -- locked the account out.
  zweitfaktorStand: () => req<ZweitfaktorStand>("/auth/two-factor"),
  zweitfaktorStart: () =>
    req<{ secret: string; uri: string; qr: string }>("/auth/two-factor/start", { method: "POST" }),
  zweitfaktorAn: (code: string) =>
    req<{ recoveryCodes: string[] }>("/auth/two-factor/enable", {
      method: "POST",
      body: JSON.stringify({ code }),
    }),
  zweitfaktorAus: (passwort: string) =>
    req<{ ok: boolean }>("/auth/two-factor/disable", {
      method: "POST",
      body: JSON.stringify({ password: passwort }),
    }),
  zweitfaktorCodesNeu: (passwort: string) =>
    req<{ recoveryCodes: string[] }>("/auth/two-factor/codes", {
      method: "POST",
      body: JSON.stringify({ password: passwort }),
    }),
  // The second step of signing in. Accepted is the current code from the app or
  // one of the recovery codes.
  zweitfaktorPruefen: (ticket: string, code: string) =>
    req<User>("/auth/two-factor/verify", {
      method: "POST",
      body: JSON.stringify({ ticket, code }),
    }),

  // Word attachments: read as editor blocks and write back as .docx.
  wordLesen: (seiteId: string, anhangId: string) =>
    req<{ title: string; blocks: unknown[] }>(`/pages/${seiteId}/attachments/${anhangId}/word`),
  wordSchreiben: (seiteId: string, anhangId: string, titel: string, bloecke: unknown) =>
    req<{ ok: boolean; bytes: number }>(`/pages/${seiteId}/attachments/${anhangId}/word`, {
      method: "PUT",
      body: JSON.stringify({ title: titel, blocks: bloecke }),
    }),

  // Write a PDF marked up in the browser over the old one. Raw and not as JSON:
  // the file is already bytes, and sending it through Base64 would make it a
  // third larger without anybody getting anything out of it.
  pdfErsetzen: async (seiteId: string, anhangId: string, daten: Uint8Array) => {
    const res = await fetch(`/api/pages/${seiteId}/attachments/${anhangId}/pdf`, {
      method: "PUT",
      credentials: "include",
      headers: { "Content-Type": "application/pdf" },
      body: new Blob([daten as BlobPart], { type: "application/pdf" }),
    });
    if (!res.ok) {
      let msg = res.statusText;
      try {
        const j = await res.json();
        if (j && j.error) msg = j.error;
      } catch {
        /* ignore */
      }
      throw new Error(msg);
    }
    return (await res.json()) as { ok: boolean; bytes: number };
  },

  // One's own profile. The name as JSON, the picture raw: it is already bytes,
  // and sending it through Base64 would make it a third larger.
  profilAendern: (name: string) =>
    req<{ ok: boolean; name: string }>("/profile", {
      method: "PUT",
      body: JSON.stringify({ name }),
    }),
  profilbildSetzen: async (bild: Blob) => {
    const res = await fetch("/api/profile/image", {
      method: "PUT",
      credentials: "include",
      headers: { "Content-Type": bild.type || "image/jpeg" },
      body: bild,
    });
    if (!res.ok) {
      let msg = res.statusText;
      try {
        const j = await res.json();
        if (j && j.error) msg = j.error;
      } catch {
        /* ignore */
      }
      throw new Error(msg);
    }
    return (await res.json()) as { ok: boolean; imageVersion: string };
  },
  profilbildWeg: () => req<{ ok: boolean }>("/profile/image", { method: "DELETE" }),

  sitzungen: () => req<Sitzung[]>("/sessions"),
  sitzungBeenden: (id: string) => req<void>(`/sessions/${id}`, { method: "DELETE" }),
  sitzungenBeenden: () => req<{ ended: number }>("/sessions", { method: "DELETE" }),

  // Read once after sign-in. Hiding locked features is a courtesy to the
  // reader, not a protection: the backend refuses the same calls with 402
  // regardless of what the interface shows.
  lizenz: () => req<Lizenz>("/license"),
  lizenzEinlesen: (schluessel: string) =>
    req<Lizenz>("/system/license", { method: "PUT", body: JSON.stringify({ key: schluessel }) }),
  lizenzAusstellen: (p: {
    holder: string;
    tier: string;
    features?: string[];
    expires?: string;
  }) => req<{ key: string }>("/system/license/issue", {
    method: "POST",
    body: JSON.stringify(p),
  }),

  pruefspur: (p: { action?: string; actor?: string; object?: string; limit?: number } = {}) => {
    const q = new URLSearchParams();
    if (p.action) q.set("action", p.action);
    if (p.actor) q.set("actor", p.actor);
    if (p.object) q.set("object", p.object);
    if (p.limit) q.set("limit", String(p.limit));
    return req<Spureintrag[]>(`/audit-log?${q.toString()}`);
  },
  pruefspurAktionen: () => req<{ action: string; count: number }[]>("/audit-log/actions"),

  design: () => req<{ baseColor: string; accent: string; pageWidth: string; language?: string }>("/design"),
  // The interface language on the account. Empty resets to "the browser decides".
  spracheSpeichern: (sprache: string) =>
    req<unknown>("/design/language", { method: "PUT", body: JSON.stringify({ language: sprache }) }),
  /** Base tone and accent of one's own account. Empty resets to the default. */
  benachrichtigung: () => req<Benachrichtigung>("/account/notifications"),
  benachrichtigungSpeichern: (email: boolean) =>
    req<Benachrichtigung>("/account/notifications", { method: "PUT", body: JSON.stringify({ email }) }),
  benachrichtigungTesten: () =>
    req<{ ok: boolean; to: string }>("/account/notifications/test", { method: "POST" }),
  aussehenSpeichern: (grundton: string, akzent: string) =>
    req<{ baseColor: string; accent: string }>("/design", {
      method: "PUT",
      body: JSON.stringify({ baseColor: grundton, accent: akzent }),
    }),

  einstellungen: () => req<Einstellung[]>("/settings"),
  einstellungSetzen: (schluessel: string, wert: string) =>
    req<{ value: string }>("/settings", {
      method: "PUT",
      body: JSON.stringify({ key: schluessel, value: wert }),
    }),
  einstellungZuruecksetzen: (schluessel: string) =>
    req<{ value: string }>(`/settings?key=${encodeURIComponent(schluessel)}`, {
      method: "DELETE",
    }),
  systemZustand: () => req<SystemZustand>("/system"),
  puls: () => req<Puls>("/system/pulse"),
  mitschriftZustand: () => req<MitschriftZustand>("/system/collab"),
  mitschreibende: (id: string) =>
    req<{ count: number; possible: boolean }>(`/pages/${id}/collaborators`),
  sicherungUmfang: () => req<SicherungUmfang>("/system/backup/scope"),
  // Restoring. No req: the body is multipart and can be gigabytes in size,
  // that does not belong through JSON.stringify.
  wiederherstellen: async (datei: File) => {
    const rumpf = new FormData();
    rumpf.append("file", datei);
    rumpf.append("confirmation", "restore");
    const antwort = await fetch("/api/system/restore", {
      method: "POST",
      credentials: "include",
      body: rumpf,
    });
    const d = await antwort.json().catch(() => ({}));
    if (!antwort.ok) throw new Error(d.error || `Fehler ${antwort.status}`);
    return d as {
      ok: boolean;
      attachments: number;
      failed: number;
      rollback: string;
      note: string;
    };
  },
  sicherungTokenNeu: () =>
    req<SicherungUmfang>("/system/backup/token", { method: "POST" }),
  sicherungTokenWeg: () =>
    req<SicherungUmfang>("/system/backup/token", { method: "DELETE" }),
  // No req: the answer is an archive and not JSON, and it can be gigabytes in
  // size. The browser downloads it itself, the cookie goes along.
  sicherungAdresse: "/api/system/backup",
  ldapEinrichtung: () => req<LDAPEinrichtung>("/system/ldap"),
  ldapTesten: (benutzer: string, passwort: string) =>
    req<LDAPTestErgebnis>("/system/ldap/test", {
      method: "POST",
      body: JSON.stringify({ user: benutzer, password: passwort }),
    }),
  // Sends off that many megabytes and reports whether they arrive. Not through
  // req: the body is not JSON, and a severed connection is a result here and
  // not an error.
  grenzprobe: async (mb: number): Promise<boolean> => {
    try {
      const r = await fetch("/api/system/size-probe", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/octet-stream" },
        body: new Blob([new Uint8Array(Math.round(mb * 1024 * 1024))]),
      });
      return r.ok;
    } catch {
      // nginx does not always answer a body that is too large with a 413, it
      // cuts the connection instead. For the measurement that is the same
      // thing.
      return false;
    }
  },
  anmeldungen: (p: { only?: string; ip?: string; days?: number; limit?: number } = {}) => {
    const q = new URLSearchParams();
    if (p.only) q.set("only", p.only);
    if (p.ip) q.set("ip", p.ip);
    if (p.days !== undefined) q.set("days", String(p.days));
    if (p.limit) q.set("limit", String(p.limit));
    return req<Anmeldungen>(`/system/sign-ins?${q.toString()}`);
  },
  suchindexNeu: () => req<{ withoutSearchText: number }>("/system/search-index", { method: "POST" }),
  anhangindexNachziehen: () =>
    req<{ checked: number; read: number; withoutText: number }>("/system/attachment-index", {
      method: "POST",
    }),
  ablageZustand: () => req<{ storage: string }>("/system/storage"),
  ablageTesten: (p: {
    endpoint: string;
    bucket: string;
    accessKey: string;
    secretKey: string;
    region: string;
    tls: boolean;
    pathStyle: boolean;
  }) =>
    req<{ ok: boolean; storage?: string; step?: string; reason?: string; remark?: string }>(
      "/system/storage/test",
      { method: "POST", body: JSON.stringify(p) },
    ),

  kommentare: (pageId: string) => req<Kommentar[]>(`/pages/${pageId}/comments`),
  // Whom one can address with an @ in a comment on this page: exactly the
  // accounts allowed to read it. The others would get no notification anyway.
  erwaehnbare: (pageId: string) => req<Person[]>(`/pages/${pageId}/mentionable`),
  kommentarAnlegen: (pageId: string, text: string, elternId?: string) =>
    req<Kommentar>(`/pages/${pageId}/comments`, {
      method: "POST",
      body: JSON.stringify({ text, parentId: elternId ?? null }),
    }),
  kommentarAendern: (id: string, text: string) =>
    req<void>(`/comments/${id}`, { method: "PUT", body: JSON.stringify({ text }) }),
  kommentarLoeschen: (id: string) => req<void>(`/comments/${id}`, { method: "DELETE" }),
  kommentarErledigt: (id: string) =>
    req<{ resolved: boolean }>(`/comments/${id}/resolved`, { method: "POST" }),

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

  gruppen: () => req<Gruppe[]>("/groups"),
  gruppeAnlegen: (name: string, beschreibung: string) =>
    req<Gruppe>("/groups", { method: "POST", body: JSON.stringify({ name, description: beschreibung }) }),
  gruppeLoeschen: (id: string) => req<void>(`/groups/${id}`, { method: "DELETE" }),
  gruppenMitglieder: (id: string) => req<Mitglied[]>(`/groups/${id}/members`),
  mitgliedSetzen: (gruppeId: string, userId: string, drin: boolean) =>
    req<{ isMember: boolean }>(`/groups/${gruppeId}/members`, {
      method: "PUT",
      body: JSON.stringify({ userId, isMember: drin }),
    }),

  // The colour of a space in the graph. Empty resets it to the series.
  spaceFarbe: (id: string, farbe: string) =>
    req<{ color: string }>(`/spaces/${id}/color`, {
      method: "PUT",
      body: JSON.stringify({ color: farbe }),
    }),

  spaceRechte: (spaceId: string) => req<SpaceRecht[]>(`/spaces/${spaceId}/permissions`),
  spaceRechtSetzen: (
    spaceId: string,
    ziel: { groupId?: string; userId?: string },
    recht: string,
  ) =>
    req<{ permission: string }>(`/spaces/${spaceId}/permissions`, {
      method: "PUT",
      body: JSON.stringify({ groupId: ziel.groupId ?? "", userId: ziel.userId ?? "", permission: recht }),
    }),
  // The measure of a page. Whoever may write may set it: it is a property of
  // the text, not of the sharing.
  seiteBreite: (id: string, breite: Seitenbreite) =>
    req<{ width: Seitenbreite }>(`/pages/${id}/width`, {
      method: "PUT",
      body: JSON.stringify({ width: breite }),
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
    ziel: { parentId?: string | null; spaceId?: string | null; beforeId?: string | null },
  ) => req<Page>(`/pages/${id}/order`, { method: "PUT", body: JSON.stringify(ziel) }),
  deletePage: (id: string) => req<void>(`/pages/${id}`, { method: "DELETE" }),
  restorePage: (id: string) => req<void>(`/pages/${id}/restore`, { method: "POST" }),
  purgePage: (id: string) => req<void>(`/pages/${id}/purge`, { method: "DELETE" }),

  addFavorite: (id: string) => req<void>(`/pages/${id}/favorite`, { method: "POST" }),
  beobachten: (id: string) => req<void>(`/pages/${id}/watch`, { method: "POST" }),
  nichtBeobachten: (id: string) => req<void>(`/pages/${id}/watch`, { method: "DELETE" }),

  vorlagen: () => req<Vorlage[]>("/templates"),
  vorlageAnlegen: (p: { title: string; icon?: string; content: unknown; forAll?: boolean }) =>
    req<Vorlage>("/templates", { method: "POST", body: JSON.stringify(p) }),
  vorlageLoeschen: (id: string) => req<void>(`/templates/${id}`, { method: "DELETE" }),


  apiSchluessel: () => req<APISchluessel[]>("/api-keys"),
  apiSchluesselAnlegen: (name: string, scope: "lesen" | "schreiben", days: number) =>
    req<APISchluessel>("/api-keys", { method: "POST", body: JSON.stringify({ name, scope, days }) }),
  apiSchluesselWiderrufen: (id: string) => req<void>(`/api-keys/${id}`, { method: "DELETE" }),

  webhooks: () => req<{ hooks: Webhook[]; events: string[] }>("/system/webhooks"),
  webhookAnlegen: (p: { name: string; url: string; events: string[] }) =>
    req<Webhook>("/system/webhooks", { method: "POST", body: JSON.stringify(p) }),
  webhookAendern: (id: string, p: { name?: string; url?: string; events?: string[]; active?: boolean }) =>
    req<void>(`/system/webhooks/${id}`, { method: "PUT", body: JSON.stringify(p) }),
  webhookLoeschen: (id: string) => req<void>(`/system/webhooks/${id}`, { method: "DELETE" }),
  webhookTesten: (id: string) =>
    req<{ status: number; error: string }>(`/system/webhooks/${id}/test`, { method: "POST" }),
  removeFavorite: (id: string) => req<void>(`/pages/${id}/favorite`, { method: "DELETE" }),
  listFavorites: () => req<PageMeta[]>("/favorites"),

  sharePage: (id: string) =>
    req<{ isPublic: boolean; publicToken: string }>(`/pages/${id}/share`, { method: "POST" }),
  unsharePage: (id: string) => req<{ isPublic: boolean }>(`/pages/${id}/share`, { method: "DELETE" }),

  // Version history
  listVersions: (id: string) => req<PageVersion[]>(`/pages/${id}/versions`),
  getVersion: (id: string, versionId: string) =>
    req<PageVersion & { content: unknown }>(`/pages/${id}/versions/${versionId}`),
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
          /* then the status text it is */
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

  // Import: one or several Markdown files, or a ZIP with structure. Past req
  // as with an attachment, for the same reason; FormData is set by the browser
  // itself, boundary included.
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
    if (ziel.neueAblage) body.append("newSpace", ziel.neueAblage);
    // The same call with the same content, only without consequences; the
    // server computes the same plan and creates nothing.
    if (vorschau) body.append("preview", "1");
    const res = await fetch(`/api/import`, { method: "POST", credentials: "include", body });
    if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || res.statusText);
    return (await res.json()) as EinfuhrBericht & EinfuhrVorschau;
  },

  // Postfach
  postfach: (nurUngelesen = false) =>
    req<Nachricht[]>(`/postfach${nurUngelesen ? "?ungelesen=1" : ""}`),
  postfachAnzahl: () => req<{ unread: number }>("/inbox/count"),
  postfachGelesen: (id?: string) =>
    req<{ ok: boolean }>(id ? `/inbox/${id}/read` : "/inbox/read", { method: "POST" }),
  postfachLeeren: () => req<{ deleted: number }>("/inbox", { method: "DELETE" }),

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
  createUser: (email: string, name: string, password: string, role: string, benutzername = "") =>
    req<User>("/users", {
      method: "POST",
      body: JSON.stringify({ email, name, password, role, username: benutzername }),
    }),
  deleteUser: (id: string) => req<void>(`/users/${id}`, { method: "DELETE" }),
  // Remove another account's second factor, for the lost phone. The account
  // signs in with the password alone again afterwards.
  zweitfaktorZuruecksetzen: (id: string) =>
    req<{ ok: boolean }>(`/users/${id}/two-factor`, { method: "DELETE" }),
  setUserRole: (id: string, role: string) =>
    req<{ role: string }>(`/users/${id}/role`, { method: "PUT", body: JSON.stringify({ role }) }),
  benutzernameSetzen: (id: string, benutzername: string) =>
    req<{ username: string }>(`/users/${id}/username`, {
      method: "PUT",
      body: JSON.stringify({ username: benutzername }),
    }),

  // One's own machines: the list together with what can currently be seen of
  // them.
  rechner: () => req<RechnerListe>("/system/hosts"),
  rechnerAnlegen: (r: RechnerEingabe) =>
    req<Rechner>("/system/hosts", { method: "POST", body: JSON.stringify(r) }),
  rechnerAendern: (id: string, r: RechnerEingabe) =>
    req<Rechner>(`/system/hosts/${id}`, { method: "PUT", body: JSON.stringify(r) }),
  rechnerLoeschen: (id: string) =>
    req<{ ok: boolean }>(`/system/hosts/${id}`, { method: "DELETE" }),

  // One's own password. The answer says how many other sessions were ended in
  // the process -- that is the number somebody wants to see who is changing it
  // because another device was involved.
  passwortWechseln: (alt: string, neu: string) =>
    req<{ ok: boolean; ended: number }>("/auth/password", {
      method: "POST",
      body: JSON.stringify({ old: alt, new: neu }),
    }),

  // Reset by an administrator, for a forgotten password.
  passwortSetzen: (id: string, neu: string) =>
    req<{ ok: boolean; ended: number }>(`/users/${id}/password`, {
      method: "PUT",
      body: JSON.stringify({ new: neu }),
    }),

  // Wartung: Konfigurationsdatei, Neustart, Papierkorb der Instanz
  konfigLesen: () => req<KonfigDatei>("/system/config"),
  konfigPruefen: (inhalt: string) =>
    req<{ notes: string[] }>("/system/config", {
      method: "PUT",
      body: JSON.stringify({ content: inhalt, dryRun: true }),
    }),
  konfigSchreiben: (inhalt: string) =>
    req<{ notes: string[]; backup: string; restartRequired: boolean }>("/system/config", {
      method: "PUT",
      body: JSON.stringify({ content: inhalt }),
    }),
  neustarten: () =>
    req<{ ok: boolean }>("/system/restart", {
      method: "POST",
      body: JSON.stringify({ confirmation: "neustart" }),
    }),
  papierkorbLeeren: () =>
    req<{ deleted: number }>("/system/trash", { method: "POST" }),

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
    req<void>("/spaces/order", { method: "PUT", body: JSON.stringify({ ids }) }),
  spaceOeffentlich: (id: string, oeffentlich: "nein" | "lesen" | "schreiben") =>
    req<{ publicAccess: string }>(`/spaces/${id}/public-access`, {
      method: "PUT",
      body: JSON.stringify({ publicAccess: oeffentlich }),
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
    if (filter?.days) p.set("days", String(filter.days));
    if (filter?.who) p.set("who", filter.who === "ich" ? "me" : filter.who);
    return req<SearchHit[]>(`/search?${p.toString()}`);
  },
  getPublicPage: (token: string) => req<PublicPage>(`/public/${token}`),
};

/** Inbox by e-mail: the account's choice and whether the instance can send. */
export type Benachrichtigung = { email: boolean; available: boolean; address: string };
