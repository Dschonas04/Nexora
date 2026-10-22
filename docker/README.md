# Nexora with Docker

Everything for running Nexora on one machine with Docker Compose.

## Install

```bash
./install.sh            # Nexora, its PostgreSQL and the internal PKI
./install.sh redis      # the same plus a Redis cache
./install.sh single     # everything in one container (Standard scope only)
```

On the first run the script writes `.env` with random values for
`POSTGRES_PASSWORD` and `JWT_SECRET`, pulls the published images and starts
them, and waits until the API answers. Then open `https://<host>:3443` (or
`http://<host>:3000`); the first account you register becomes the
administrator.

Keep the `.env`: PostgreSQL fixes its password at the first start, and without
the file the database stays locked.

**Update:** run `./install.sh` again. It keeps the `.env`, pulls and restarts.
A fixed version instead of the newest: `NEXORA_VERSION=3.1.0` in `.env`;
Docker Hub instead of GitHub: `NEXORA_REGISTRY=dschohnas`.

Needs Docker with the Compose plugin; amd64 and arm64 (Raspberry Pi 4/5) both
work.

## The files

| File | What it is |
|---|---|
| `install.sh` | Writes `.env`, pulls, starts, checks |
| `docker-compose.stack.yml` | The published images: database, PKI, backend, frontend. Also works pasted into Portainer, Coolify or Dokploy |
| `docker-compose.single.yml` | `nexora-single`: database, service and web server in one container |
| `docker-compose.yml` | PKI, backend and frontend **built from this checkout** |
| `docker-compose.db.yml` | Side file: a PostgreSQL of its own, for `docker-compose.yml` |
| `docker-compose.image.yml` | Side file: the published images instead of building, for `docker-compose.yml` |
| `docker-compose.redis.yml` | Side file: Redis as a cache |
| `docker-compose.minio.yml` | Side file: MinIO for the attachments |
| `.env.example` | Every variable, documented; `install.sh` starts from it |
| `postgres-18.sh` | Moves a PostgreSQL 16 volume to 18 |

`COMPOSE_FILE` in `.env` says which files apply, so every `docker compose`
command in this folder uses the same set:

```bash
COMPOSE_FILE=docker-compose.stack.yml                                  # install.sh's default
COMPOSE_FILE=docker-compose.yml:docker-compose.db.yml                  # built from source
COMPOSE_FILE=docker-compose.yml:docker-compose.db.yml:docker-compose.minio.yml
```

## Building from source

```bash
cp .env.example .env
cp ../config.example.conf config.conf
# in .env: COMPOSE_FILE=docker-compose.yml:docker-compose.db.yml,
#          POSTGRES_PASSWORD and JWT_SECRET (openssl rand -hex 32)
docker compose up -d --build
```

`config.conf` belongs next to these files; the backend reads it and the
maintenance page edits it. For that it has to be writable by the service:
`sudo chgrp 10001 config.conf && sudo chmod 660 config.conf`.

## Coming from before this folder

The compose files used to stand at the top of the repository. Move `.env` and
`config.conf` into this folder and run compose from here. The project keeps its
name (`name: nexora` in the files), so the containers and volumes are the same
ones as before -- nothing is created twice.

More: [operations](../docs/operations.md), [configuration](../docs/configuration.md).
