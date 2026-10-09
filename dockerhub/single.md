# Nexora (single container)

**Self-hosted wiki with nested pages, backlinks and a knowledge graph, in one container.**

PostgreSQL, the Nexora service and the web interface together in one image, one
volume, nothing to configure. For trying Nexora or for a small team on a small
machine.

```bash
docker run -d --name nexora -p 3000:80 -v nexora_data:/data \
  --restart unless-stopped dschohnas/nexora-single
```

Open `http://localhost:3000`. The first account that registers becomes the
administrator. Port 443 serves the same with a self-signed certificate
(`-p 3443:443`).

Or with Compose:
[docker-compose.single.yml](https://github.com/Dschonas04/Nexora/blob/main/docker-compose.single.yml)

## What you get

Nested pages in a block editor, spaces with roles, `[[wiki links]]` with
backlinks, a knowledge graph, version history, comments, attachments, trash,
PostgreSQL full text search, import from Obsidian, Notion and Confluence,
Markdown export. Interface in English, German and French.

## Good to know

- **Standard scope only.** This image runs the free part of Nexora and reads no
  licence key. For Pro or Business features take the stack with separate
  containers.
- **Not built for load.** Database, service and web server share one container.
  For more than a handful of people writing at once, take the stack; backup and
  restore (Settings, System) move your data across.
- **Everything lives in `/data`**: the database, attachments, the session secret
  and database password generated on the first start, and the certificate.
  Back up that volume.
- `JWT_SECRET` may be set from outside; otherwise it is generated and kept.

## Tags

`latest` newest release · `2.2` stays on a minor line · `2.2.1` pinned exactly.
Architectures `linux/amd64` and `linux/arm64`. The same images are on
`ghcr.io/dschonas04/nexora-single`.

## Links

- Source and documentation: https://github.com/Dschonas04/Nexora
- Live demo (resets nightly): https://nexora.jonasgroll.de
- Licence: Business Source License 1.1, becomes Apache 2.0 on 2030-08-19.
  Production and commercial use of the Standard scope are permitted.
