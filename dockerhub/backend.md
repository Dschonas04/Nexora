# Nexora: Go API service

**Self-hosted wiki with nested pages, backlinks and a knowledge graph.**

The Go service: pages, search, versions, attachments, accounts. Talks to PostgreSQL, serves the API behind `nexora-frontend`.

This image is one of three that run together: `nexora-backend`,
`nexora-frontend` and `nexora-pki`, plus PostgreSQL. It is not meant to run
alone.

## Quick start

The whole stack from one file:

```bash
curl -O https://raw.githubusercontent.com/Dschonas04/Nexora/main/docker-compose.stack.yml
# set POSTGRES_PASSWORD and JWT_SECRET first (openssl rand -hex 32)
NEXORA_REGISTRY=dschohnas docker compose -f docker-compose.stack.yml up -d
```

Only want to try it? [`dschohnas/nexora-single`](https://hub.docker.com/r/dschohnas/nexora-single)
is everything in one container: `docker run -d -p 3000:80 -v nexora_data:/data dschohnas/nexora-single`.

## Tags

`latest` newest release · `2.2` stays on a minor line · `2.2.1` pinned exactly.
Architectures `linux/amd64` and `linux/arm64`. The same images are on
`ghcr.io/dschonas04/nexora-backend`.

## Links

- Source and documentation: https://github.com/Dschonas04/Nexora
- Configuration: https://github.com/Dschonas04/Nexora/blob/main/docs/configuration.md
- Live demo (resets nightly): https://nexora.jonasgroll.de
- Licence: Business Source License 1.1, becomes Apache 2.0 on 2030-08-19.
