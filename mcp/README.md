# Nexora MCP Server

A zero-dependency [Model Context Protocol](https://modelcontextprotocol.io) server
that exposes the Nexora knowledge base to MCP clients (e.g. Claude Code, Claude
Desktop). It speaks newline-delimited JSON-RPC 2.0 over stdio and uses only the
Node.js standard library (built-in `fetch` + `crypto`), so **Node 18+ is the only
requirement** — no `npm install`.

## Tools

| Tool | Purpose |
|------|---------|
| `nexora_search` | Full-text search across pages |
| `nexora_list_pages` | Tree overview of all accessible pages (id, title, parent, space) |
| `nexora_list_spaces` | List spaces (top-level groups) |
| `nexora_get_page` | Read a page by id (body rendered as markdown) |
| `nexora_create_page` | Create a page from markdown (`[[wiki-links]]` supported) |
| `nexora_update_page` | Replace a page's title and/or body |
| `nexora_backlinks` | Pages linking here via `[[wiki-link]]` |

Markdown support is intentionally light: `#`/`##`/`###` headings, bullet and
numbered lists, and paragraphs. `[[Page Title]]` anywhere in the text becomes a
wiki-link that the graph and backlinks pick up.

## Configuration (environment)

| Variable | Default | Notes |
|----------|---------|-------|
| `NEXORA_URL` | `http://10.0.2.43:3000` | Backend base URL |
| `NEXORA_EMAIL` | `demo@nexora.local` | Login email |
| `NEXORA_PASSWORD` | *(none)* | **Required.** Never hardcoded — supply via client config |

The server authenticates lazily and re-logs in once on a `401`, so the httpOnly
`nexora_token` cookie stays fresh across long sessions.

## Register with Claude Code

```bash
claude mcp add nexora --scope user \
  -e NEXORA_URL=http://10.0.2.43:3000 \
  -e NEXORA_EMAIL=you@example.com \
  -e NEXORA_PASSWORD=your-password \
  -- node /path/to/Nexora/mcp/nexora-mcp.mjs
```

User scope keeps the password out of any committed file. Verify with
`claude mcp list` (should report `nexora: ✔ Connected`).

## Manual smoke test

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{}}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | NEXORA_PASSWORD=... node nexora-mcp.mjs
```
