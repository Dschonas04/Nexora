#!/usr/bin/env node
// Nexora MCP server — exposes the Nexora knowledge base to an MCP client
// (e.g. Claude Code) over stdio. Zero external dependencies: uses Node 22's
// built-in fetch and crypto, speaks newline-delimited JSON-RPC 2.0.
//
// Configuration via environment:
//   NEXORA_URL       base URL of the Nexora backend (default http://10.0.2.43:3000)
//   NEXORA_EMAIL     login email       (default demo@nexora.local)
//   NEXORA_PASSWORD  login password    (default secret1)
//
// The server authenticates lazily and re-authenticates once on a 401 so the
// httpOnly nexora_token cookie stays fresh across long sessions.

import { randomUUID } from "node:crypto";

const BASE = (process.env.NEXORA_URL || "http://10.0.2.43:3000").replace(/\/+$/, "");
const EMAIL = process.env.NEXORA_EMAIL || "demo@nexora.local";
// No credential default in source (this repo is public). Supply via the MCP
// client config, e.g. NEXORA_PASSWORD in Claude Code's user-scoped settings.
const PASSWORD = process.env.NEXORA_PASSWORD || "";

let cookie = null; // "nexora_token=..."

// ---------------------------------------------------------------- HTTP layer

async function login() {
  if (!PASSWORD) throw new Error("NEXORA_PASSWORD not set in the environment");
  const res = await fetch(`${BASE}/api/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email: EMAIL, password: PASSWORD }),
  });
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(`login failed (${res.status}): ${body || res.statusText}`);
  }
  const setCookies =
    typeof res.headers.getSetCookie === "function"
      ? res.headers.getSetCookie()
      : [res.headers.get("set-cookie")].filter(Boolean);
  const tok = setCookies
    .map((c) => /(?:^|;\s*)(nexora_token=[^;]+)/.exec(c))
    .find(Boolean);
  if (!tok) throw new Error("login succeeded but no nexora_token cookie returned");
  cookie = tok[1];
}

// Authenticated JSON request against /api. Re-logs in once on a 401.
async function apiRaw(method, path, body, _retried = false) {
  if (!cookie) await login();
  const opts = { method, headers: { Cookie: cookie } };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(`${BASE}/api${path}`, opts);
  if (res.status === 401 && !_retried) {
    cookie = null;
    return apiRaw(method, path, body, true);
  }
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const j = await res.json();
      if (j && j.error) msg = j.error;
    } catch {
      /* ignore */
    }
    const err = new Error(`${method} ${path} -> ${res.status}: ${msg}`);
    err.status = res.status;
    throw err;
  }
  const text = await res.text();
  return text ? JSON.parse(text) : undefined;
}

// --------------------------------------------------- BlockNote <-> markdown

// Convert lightweight markdown into BlockNote block JSON. Supports headings
// (#, ##, ###), bullet/numbered list items and paragraphs — enough for
// readable notes, and [[wiki-links]] survive as plain text so the graph and
// backlinks pick them up.
function markdownToBlocks(md) {
  const baseProps = { textColor: "default", backgroundColor: "default", textAlignment: "left" };
  const mk = (type, text, extraProps = {}) => ({
    id: randomUUID(),
    type,
    props: { ...baseProps, ...extraProps },
    content: text ? [{ type: "text", text, styles: {} }] : [],
    children: [],
  });
  const blocks = [];
  for (const rawLine of String(md ?? "").split("\n")) {
    const line = rawLine.replace(/\s+$/, "");
    let m;
    if ((m = /^(#{1,3})\s+(.*)$/.exec(line))) {
      blocks.push(mk("heading", m[2], { level: m[1].length }));
    } else if ((m = /^\s*[-*]\s+(.*)$/.exec(line))) {
      blocks.push(mk("bulletListItem", m[1]));
    } else if ((m = /^\s*\d+\.\s+(.*)$/.exec(line))) {
      blocks.push(mk("numberedListItem", m[1]));
    } else if (line.trim() === "") {
      // collapse blank lines; skip
    } else {
      blocks.push(mk("paragraph", line));
    }
  }
  if (blocks.length === 0) blocks.push(mk("paragraph", ""));
  return blocks;
}

// Extract readable text from a single BlockNote inline-content value, which
// may be an array of inline nodes or a bare string.
function inlineText(content) {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content
    .map((n) => {
      if (typeof n === "string") return n;
      if (n && typeof n.text === "string") return n.text;
      if (n && n.type === "link") return inlineText(n.content);
      return "";
    })
    .join("");
}

// Render BlockNote block JSON back to markdown for reading.
function blocksToMarkdown(content) {
  const blocks = Array.isArray(content) ? content : [];
  const out = [];
  const walk = (list, depth) => {
    for (const b of list) {
      if (!b || typeof b !== "object") continue;
      const text = inlineText(b.content);
      const indent = "  ".repeat(depth);
      switch (b.type) {
        case "heading":
          out.push("#".repeat(b.props?.level || 1) + " " + text);
          break;
        case "bulletListItem":
          out.push(indent + "- " + text);
          break;
        case "numberedListItem":
          out.push(indent + "1. " + text);
          break;
        case "checkListItem":
          out.push(indent + "- [" + (b.props?.checked ? "x" : " ") + "] " + text);
          break;
        default:
          out.push(indent + text);
      }
      if (Array.isArray(b.children) && b.children.length) walk(b.children, depth + 1);
    }
  };
  walk(blocks, 0);
  return out.join("\n").trim();
}

// ------------------------------------------------------------------- tools

const TOOLS = [
  {
    name: "nexora_search",
    description:
      "Full-text search across all Nexora pages. Returns matching pages with id and title.",
    inputSchema: {
      type: "object",
      properties: { query: { type: "string", description: "Search terms" } },
      required: ["query"],
    },
    handler: async ({ query }) => {
      const rows = await apiRaw("GET", `/search?q=${encodeURIComponent(query)}`);
      return list((rows || []).map((p) => `${p.title || "Untitled"}  [${p.id}]`), "No matches.");
    },
  },
  {
    name: "nexora_list_pages",
    description:
      "List all pages the account can access, with id, title, parent and space, as a tree overview.",
    inputSchema: { type: "object", properties: {} },
    handler: async () => {
      const [pages, spaces] = await Promise.all([
        apiRaw("GET", "/pages"),
        apiRaw("GET", "/spaces").catch(() => []),
      ]);
      const spaceName = Object.fromEntries((spaces || []).map((s) => [s.id, s.name]));
      const lines = (pages || []).map((p) => {
        const sp = p.spaceId ? ` {${spaceName[p.spaceId] || p.spaceId}}` : "";
        const par = p.parentId ? ` (child of ${p.parentId})` : "";
        return `${p.title || "Untitled"}  [${p.id}]${sp}${par}`;
      });
      return list(lines, "No pages.");
    },
  },
  {
    name: "nexora_list_spaces",
    description: "List all spaces (top-level groups) with id and name.",
    inputSchema: { type: "object", properties: {} },
    handler: async () => {
      const spaces = await apiRaw("GET", "/spaces");
      return list((spaces || []).map((s) => `${s.name}  [${s.id}]`), "No spaces.");
    },
  },
  {
    name: "nexora_get_page",
    description:
      "Read a page by id. Returns title, tags and the body rendered as markdown.",
    inputSchema: {
      type: "object",
      properties: { id: { type: "string", description: "Page id (UUID)" } },
      required: ["id"],
    },
    handler: async ({ id }) => {
      const p = await apiRaw("GET", `/pages/${id}`);
      const tags = (p.tags || []).map((t) => t.name).join(", ");
      const header = [
        `# ${p.title || "Untitled"}`,
        `id: ${p.id}${p.spaceId ? `  space: ${p.spaceId}` : ""}${p.parentId ? `  parent: ${p.parentId}` : ""}`,
        tags ? `tags: ${tags}` : null,
        "",
      ]
        .filter((x) => x !== null)
        .join("\n");
      return text(header + "\n" + blocksToMarkdown(p.content));
    },
  },
  {
    name: "nexora_create_page",
    description:
      "Create a new page. Body is markdown (headings #/##/###, bullet/numbered lists, paragraphs). " +
      "Use [[Page Title]] anywhere in the text to create a wiki-link to another page.",
    inputSchema: {
      type: "object",
      properties: {
        title: { type: "string" },
        body: { type: "string", description: "Markdown content" },
        parentId: { type: "string", description: "Optional parent page id" },
        spaceId: { type: "string", description: "Optional space id" },
      },
      required: ["title"],
    },
    handler: async ({ title, body, parentId, spaceId }) => {
      const created = await apiRaw("POST", "/pages", {
        parentId: parentId ?? null,
        spaceId: spaceId ?? null,
      });
      await apiRaw("PUT", `/pages/${created.id}`, {
        title,
        content: markdownToBlocks(body || ""),
      });
      return text(`Created page "${title}"  [${created.id}]`);
    },
  },
  {
    name: "nexora_update_page",
    description:
      "Update a page's title and/or body by id. Body (markdown) replaces the whole content. " +
      "Omit a field to leave it unchanged.",
    inputSchema: {
      type: "object",
      properties: {
        id: { type: "string" },
        title: { type: "string" },
        body: { type: "string", description: "New markdown content (replaces existing)" },
      },
      required: ["id"],
    },
    handler: async ({ id, title, body }) => {
      const patch = {};
      if (title !== undefined) patch.title = title;
      if (body !== undefined) patch.content = markdownToBlocks(body);
      if (Object.keys(patch).length === 0) return text("Nothing to update (provide title and/or body).");
      await apiRaw("PUT", `/pages/${id}`, patch);
      return text(`Updated page [${id}]`);
    },
  },
  {
    name: "nexora_backlinks",
    description: "List pages that link to the given page via [[wiki-link]].",
    inputSchema: {
      type: "object",
      properties: { id: { type: "string" } },
      required: ["id"],
    },
    handler: async ({ id }) => {
      const rows = await apiRaw("GET", `/pages/${id}/backlinks`);
      return list((rows || []).map((p) => `${p.title || "Untitled"}  [${p.id}]`), "No backlinks.");
    },
  },
];

// small helpers to shape tool results
function text(s) {
  return { content: [{ type: "text", text: s }] };
}
function list(lines, empty) {
  return text(lines.length ? lines.join("\n") : empty);
}

const TOOL_MAP = Object.fromEntries(TOOLS.map((t) => [t.name, t]));

// ------------------------------------------------------------- JSON-RPC I/O

const PROTOCOL_VERSION = "2024-11-05";

function send(msg) {
  process.stdout.write(JSON.stringify(msg) + "\n");
}
function reply(id, result) {
  send({ jsonrpc: "2.0", id, result });
}
function replyError(id, code, message) {
  send({ jsonrpc: "2.0", id, error: { code, message } });
}

async function handle(msg) {
  const { id, method, params } = msg;
  // Notifications (no id) need no response.
  if (id === undefined || id === null) return;

  switch (method) {
    case "initialize":
      reply(id, {
        protocolVersion: params?.protocolVersion || PROTOCOL_VERSION,
        capabilities: { tools: {} },
        serverInfo: { name: "nexora", version: "1.0.0" },
      });
      return;
    case "ping":
      reply(id, {});
      return;
    case "tools/list":
      reply(id, {
        tools: TOOLS.map(({ name, description, inputSchema }) => ({
          name,
          description,
          inputSchema,
        })),
      });
      return;
    case "tools/call": {
      const tool = TOOL_MAP[params?.name];
      if (!tool) {
        replyError(id, -32602, `unknown tool: ${params?.name}`);
        return;
      }
      try {
        const result = await tool.handler(params.arguments || {});
        reply(id, result);
      } catch (e) {
        // Report tool failures as tool results (isError) so the model sees them.
        reply(id, { content: [{ type: "text", text: `Error: ${e.message}` }], isError: true });
      }
      return;
    }
    default:
      replyError(id, -32601, `method not found: ${method}`);
  }
}

let buf = "";
let inFlight = 0;
let stdinEnded = false;
const maybeExit = () => {
  if (stdinEnded && inFlight === 0) process.exit(0);
};

process.stdin.setEncoding("utf8");
process.stdin.on("data", (chunk) => {
  buf += chunk;
  let nl;
  while ((nl = buf.indexOf("\n")) >= 0) {
    const line = buf.slice(0, nl).trim();
    buf = buf.slice(nl + 1);
    if (!line) continue;
    let msg;
    try {
      msg = JSON.parse(line);
    } catch {
      continue; // ignore malformed lines
    }
    inFlight++;
    handle(msg)
      .catch((e) => {
        if (msg && msg.id != null) replyError(msg.id, -32603, e.message);
      })
      .finally(() => {
        inFlight--;
        maybeExit();
      });
  }
});
// Don't exit until in-flight requests have flushed their responses, otherwise
// piped input (stdin closes immediately) would truncate pending replies.
process.stdin.on("end", () => {
  stdinEnded = true;
  maybeExit();
});
