export interface User {
  id: string;
  email: string;
  name: string;
  createdAt: string;
}

export interface Tag {
  id: string;
  name: string;
  color: string;
}

export interface PageMeta {
  id: string;
  parentId: string | null;
  title: string;
  icon: string;
  updatedAt: string;
}

export interface Page {
  id: string;
  ownerId: string;
  parentId: string | null;
  title: string;
  content: unknown;
  icon: string;
  isPublic: boolean;
  publicToken: string | null;
  tags: Tag[];
  isFavorite: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface PublicPage {
  title: string;
  content: unknown;
  icon: string;
  updatedAt: string;
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
}

export const api = {
  me: () => req<User>("/auth/me"),
  login: (email: string, password: string) =>
    req<User>("/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }),
  register: (email: string, name: string, password: string) =>
    req<User>("/auth/register", { method: "POST", body: JSON.stringify({ email, name, password }) }),
  logout: () => req<void>("/auth/logout", { method: "POST" }),

  listPages: () => req<PageMeta[]>("/pages"),
  createPage: (parentId?: string | null) =>
    req<Page>("/pages", { method: "POST", body: JSON.stringify({ parentId: parentId ?? null }) }),
  getPage: (id: string) => req<Page>(`/pages/${id}`),
  updatePage: (id: string, patch: PagePatch) =>
    req<Page>(`/pages/${id}`, { method: "PUT", body: JSON.stringify(patch) }),
  deletePage: (id: string) => req<void>(`/pages/${id}`, { method: "DELETE" }),

  addFavorite: (id: string) => req<void>(`/pages/${id}/favorite`, { method: "POST" }),
  removeFavorite: (id: string) => req<void>(`/pages/${id}/favorite`, { method: "DELETE" }),
  listFavorites: () => req<PageMeta[]>("/favorites"),

  sharePage: (id: string) =>
    req<{ isPublic: boolean; publicToken: string }>(`/pages/${id}/share`, { method: "POST" }),
  unsharePage: (id: string) => req<{ isPublic: boolean }>(`/pages/${id}/share`, { method: "DELETE" }),

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
