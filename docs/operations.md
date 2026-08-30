# Deployment and Operations

Installing, upgrading, backing up and repairing an instance.

## Installation

```bash
git clone https://github.com/Dschonas04/Nexora.git && cd Nexora
cp .env.example .env
cp config.beispiel.conf config.conf

# the two values nobody may leave alone
openssl rand -hex 32          # → JWT_SECRET in .env
openssl rand -base64 24       # → POSTGRES_PASSWORD in .env

docker compose up -d --build
```

Then open `http://localhost:3000` — or `https://localhost:3443`, with a
certificate warning that no one can avoid, because nobody issues a trusted
certificate for an address in a private network.

**The first account created becomes the administrator.** Create it before doing
anything else.

`config.conf` is deliberately not tracked: it holds credentials and is edited
from the maintenance page, so a tracked copy would be overwritten on every
rollout. For that page to be able to save it:

```bash
sudo chgrp 10001 config.conf && sudo chmod 660 config.conf
```

### Choosing the stack

`COMPOSE_FILE` in `.env` decides which side files come along:

```bash
COMPOSE_FILE=docker-compose.yml                              # your own PostgreSQL, set DATABASE_URL
COMPOSE_FILE=docker-compose.yml:docker-compose.db.yml        # the default: bundled PostgreSQL 16
…:docker-compose.minio.yml                                   # plus a bundled MinIO
…:docker-compose.redis.yml                                   # plus Redis as a cache
```

The database and the object store are outside the main file on purpose. Most
installations already run both, maintained and backed up; chaining them to
Nexora would mean operating a second copy of each.

### Two things to know before the first start

- **PostgreSQL fixes its password at initialisation.** It is written into the
  data directory, so changing `POSTGRES_PASSWORD` afterwards locks the backend
  out unless you also run `ALTER USER nexora WITH PASSWORD ...` inside the
  running database, or discard the volume.
- **Do not turn `registrierung_offen` off before the first account exists**, or
  nobody can create it and the instance has no administrator.

---

## Where the data is

| What | Where | Backed up by |
|---|---|---|
| Pages, versions, comments, sessions, audit trail, settings, licence | PostgreSQL | `pg_dump` |
| Attachment bytes | `nexora_files` volume, a host directory, or a bucket | A file-level copy, or the object store's own backup |
| TLS certificate | `nexora_tls` volume | Only worth keeping if it is a real certificate |
| `config.conf` | Bind mount from the repository directory | Ordinary file backup |

### Moving attachments

```bash
NEXORA_ANHANG_ORT=/srv/nexora/anhaenge     # .env — what gets mounted
anhang_verzeichnis = /data/attachments     # config.conf — the path inside
```

The directory has to belong to **uid/gid 10001**, the account the service runs
under in the container. **Changing the setting does not move the files that are
already there** — carry them over first, then restart.

Or move them off disk entirely by setting `s3_aktiv` and the `s3_*` settings, or
by binding a store from the settings page and testing it there before saving.

---

## Backup

```bash
# database — the bulk of everything
docker compose exec -T db pg_dump -U nexora nexora | gzip > nexora-$(date +%F).sql.gz

# attachments, while they are on disk
docker run --rm -v nexora_files:/d -v "$PWD":/out alpine \
  tar czf /out/nexora-anhaenge-$(date +%F).tar.gz -C /d .

# the configuration
cp config.conf config.conf.$(date +%F)
```

Restoring the database into a fresh volume:

```bash
gunzip -c nexora-2026-08-30.sql.gz | docker compose exec -T db psql -U nexora nexora
```

The schema is applied on every start, so an empty database plus a running
backend is a working installation — the dump is content, not structure.

**A `pg_dump` alone is not a complete backup** while attachments are on disk.
This is the strongest practical argument for an object store: the bytes become
somebody else's backup problem, one that is already solved.

---

## Upgrading

```bash
git pull
docker compose up -d --build
```

The schema migration runs at startup and is idempotent. There is no separate
migration step and no maintenance window beyond the restart.

Take a database dump first anyway. Not because the migration is risky — it only
adds — but because the dump is the thing you will want if something else turns
out to be wrong.

---

## Day-to-day administration

Everything below is under **Settings** in the interface, and every one of them
is an admin-only API route as well ([API reference](api.md#administration)).

| Task | Where |
|---|---|
| Accounts, roles, deleting an account | Admin view |
| Edit `config.conf`, with a checked draft and a timestamped backup | Settings → Wartung |
| Restart the process (the only way a config change takes effect) | Settings → Wartung |
| Empty the whole instance's trash | Settings → Wartung |
| Import a licence key | Settings → Lizenz |
| Bind and test an object store | Settings → Ablage |
| Rebuild the search index | Settings, or `POST /api/system/suchindex` |
| Fill in attachment text for older uploads | `POST /api/system/anhangindex` |
| See which surrounding services answer, and how fast | Settings → Verbund |
| See who tried to sign in, from where, and why it failed | Settings → Anmeldungen (free) |
| Read the audit trail | Protokoll (paid: `pruefspur`) |

### Sign-in attempts

**Settings → Anmeldungen** lists every attempt, successful or not, with the
address it came from, the way in (password form, directory, identity provider),
the browser, and on a failure the reason. It reads the audit trail but is not
behind the `pruefspur` extra: an unlicensed instance still has to be able to see
that somebody is knocking.

Two things there answer most questions faster than the list itself. The summary
compares 24 hours against 7 days, so a spike stands out without counting rows.
The **Herkunft** table groups the week by address, most failures first, and shows
how many distinct accounts each address tried — the shape of a password spray,
which the single entries scrolling past do not show.

Nothing here is ever deleted, and passwords are recorded nowhere, not even on a
failed attempt.

### The system view

Nexora has **no access to the Docker socket**, on purpose: whoever holds that
socket is all-powerful on the host, and handing it in so a page can list
containers would be a bad bargain. What the system view reports is what can be
established without it — which services this one talks to, whether they answer,
how fast, which version they run, and how much they hold. Those are the
questions people actually ask when something is stuck.

---

## Security checklist

Before an instance is reachable by anyone but you:

- [ ] `JWT_SECRET` at least 32 random characters — `openssl rand -hex 32`
- [ ] `POSTGRES_PASSWORD` at least 24 random characters
- [ ] `.env` is not committed
- [ ] Read the startup log: dangerous defaults are named there, every boot
- [ ] `registrierung_offen` off once the accounts that should exist do — but not
      before the first one
- [ ] `erlaubte_domaenen` set if registration stays open
- [ ] A real TLS certificate in the `nexora_tls` volume, or a reverse proxy that
      terminates TLS
- [ ] `oeffentliche_url` set to the address the browser actually uses
- [ ] `s3_tls` on if an object store is in use
- [ ] LDAP with StartTLS or `ldaps://`, and certificate verification left on
- [ ] After going live, look at **Settings → Anmeldungen** once. The Herkunft
      table sums the week up by address; one address with many failures against
      many different accounts is somebody working through a list, not a
      colleague who forgot a password

`chimw.RealIP` trusts `X-Forwarded-For`. That is correct behind a proxy you
control and wrong when the container faces the internet directly — there, a
client can forge the IP that lands in the audit trail.

---

## Troubleshooting

### The backend will not start

| Log line | Cause |
|---|---|
| `db connect: ...` | Wrong `DATABASE_URL`, or the password was changed after the volume was initialised. Fix with `ALTER USER nexora WITH PASSWORD ...` inside the database |
| `Objektspeicher nicht erreichbar` | A configured object store does not answer. **This stops the boot on purpose** — otherwise new attachments would quietly land on disk while the old ones stay in the bucket. Fix the store, or set `s3_rueckfall = ja` if the disk is an acceptable stopgap |
| `db migrate: ...` | The database user lacks rights, most often to `CREATE EXTENSION pgcrypto` |

An invalid licence key never stops the boot; it is logged and the free feature
set applies.

### Uploads fail

The attachment directory does not belong to uid/gid 10001, or the file is larger
than `max_anhang_mb`, or larger than nginx's `client_max_body_size` (25 MiB) —
raising the setting alone is not enough.

### Search finds nothing for older pages

Pages written before the search index existed have an empty `content_text`. The
backend fills these in at startup, and `POST /api/system/suchindex` rebuilds the
lot. The same call is needed after changing `such_woerterbuch`.

### Attachment search finds nothing

Files uploaded before the attachment index existed have no extracted text:
`POST /api/system/anhangindex`. Note that PDF extraction needs `pdftotext`
(`poppler-utils`), which the official image ships.

### A page shows broken images to a visitor with a share link

That path is `/api/public/{token}/dateien/{attId}`, and it is part of the
`freigeben` extra. Without the licence the images do not load, because the
ordinary attachment route requires a session.

### Sign-in through the provider comes back to the wrong address

`oeffentliche_url` is unset or wrong. The callback cannot be derived from a
request that has passed through a proxy — a boot with `oidc_aktiv` and no public
URL warns about exactly this.

### Everything is slow

Check the system view first: it names which of the surrounding services answers
slowly. If Redis is configured and down, everything still works and is merely
slower — that is by design and not the fault you are looking for.

### The maintenance page says the config is read-only

The file does not belong to gid 10001:

```bash
sudo chgrp 10001 config.conf && sudo chmod 660 config.conf
docker compose restart backend
```

---

## Useful commands

```bash
docker compose logs -f backend            # the startup log names every warning
docker compose restart backend            # after a config change
docker compose exec db psql -U nexora nexora
curl -fsS http://localhost:3000/healthz   # pings the database too
```
