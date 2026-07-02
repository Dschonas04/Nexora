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

export interface PageMeta {
  id: string;
  parentId: string | null;
  spaceId: string | null;
  title: string;
  icon: string;
  shared: boolean;
  updatedAt: string;
}

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

export interface PublicPage {
  title: string;
  content: unknown;
  icon: string;
  updatedAt: string;
}

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
}

export interface GraphEdge {
  source: string;
  target: string;
  kind: string;
}

export interface Graph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

async function req<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(`/api${path}`, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...opts,
  });
  if (!res.ok) {
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
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

export interface PagePatch {
  title?: string;
  content?: unknown;
  icon?: string;
  parentId?: string | null;
  spaceId?: string | null;
}

export const api = {
  me: () => req<User>("/auth/me"),
  login: (email: string, password: string) =>
    req<User>("/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }),
  register: (email: string, name: string, password: string) =>
    req<User>("/auth/register", { method: "POST", body: JSON.stringify({ email, name, password }) }),
  logout: () => req<void>("/auth/logout", { method: "POST" }),

  listPages: () => req<PageMeta[]>("/pages"),
  listShared: () => req<PageMeta[]>("/pages/shared"),
  listTrash: () => req<PageMeta[]>("/pages/trash"),
  createPage: (parentId?: string | null, spaceId?: string | null) =>
    req<Page>("/pages", {
      method: "POST",
      body: JSON.stringify({ parentId: parentId ?? null, spaceId: spaceId ?? null }),
    }),
  getPage: (id: string) => req<Page>(`/pages/${id}`),
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

  search: (q: string) => req<PageMeta[]>(`/search?q=${encodeURIComponent(q)}`),
  getPublicPage: (token: string) => req<PublicPage>(`/public/${token}`),
};
