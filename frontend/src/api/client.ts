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
}

export interface Space {
  id: string;
  ownerId: string;
  name: string;
  createdAt: string;
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
   * the component -- never fed to dangerouslySetInnerHTML.
   */
  ausschnitt: string;
  /** false means the page was reached through a share or as an admin. */
  eigen: boolean;
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

  listPages: () => req<PageMeta[]>("/pages"),
  listShared: () => req<PageMeta[]>("/pages/shared"),
  listTrash: () => req<PageMeta[]>("/pages/trash"),
  createPage: (parentId?: string | null, spaceId?: string | null) =>
    req<Page>("/pages", {
      method: "POST",
      body: JSON.stringify({ parentId: parentId ?? null, spaceId: spaceId ?? null }),
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

  // Spaces
  listSpaces: () => req<Space[]>("/spaces"),
  createSpace: (name: string) =>
    req<Space>("/spaces", { method: "POST", body: JSON.stringify({ name }) }),
  renameSpace: (id: string, name: string) =>
    req<void>(`/spaces/${id}`, { method: "PUT", body: JSON.stringify({ name }) }),
  deleteSpace: (id: string) => req<void>(`/spaces/${id}`, { method: "DELETE" }),

  // Knowledge graph
  graph: () => req<Graph>("/graph"),

  listTags: () => req<Tag[]>("/tags"),
  createTag: (name: string, color: string) =>
    req<Tag>("/tags", { method: "POST", body: JSON.stringify({ name, color }) }),
  deleteTag: (id: string) => req<void>(`/tags/${id}`, { method: "DELETE" }),
  attachTag: (pageId: string, tagId: string) =>
    req<void>(`/pages/${pageId}/tags`, { method: "POST", body: JSON.stringify({ tagId }) }),
  detachTag: (pageId: string, tagId: string) =>
    req<void>(`/pages/${pageId}/tags/${tagId}`, { method: "DELETE" }),

  // encodeURIComponent matters here: a query may contain &, # or a slash.
  search: (q: string) => req<SearchHit[]>(`/search?q=${encodeURIComponent(q)}`),
  getPublicPage: (token: string) => req<PublicPage>(`/public/${token}`),
};
