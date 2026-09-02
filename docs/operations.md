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

### From the settings page

**Settings → Wartung → Sicherung** streams the whole instance as one ZIP:
`pg_dump` plus every attachment, read through the storage interface so it works
the same on disk and against an object store. It shows the sizes first, so
nobody presses the button on a multi-gigabyte holding and then wonders whether
it has hung.

Check for the file `FERTIG` inside before trusting an archive — a backup that
broke off mid-stream is still a valid ZIP, and half a backup would otherwise look
exactly like a whole one. `LIESMICH.md` beside it carries the restore commands.

### Restoring one

**Settings → Wartung → Sicherung einspielen**, pick the ZIP, confirm. Before
anything is overwritten the current state is dumped into the data directory as
`vor-wiederherstellung-<stamp>.sql` — that is the way back if the wrong archive
was picked, and the restore refuses to run at all if that dump cannot be written.

An archive without the `FERTIG` marker is rejected. The service restarts when it
is done, and you will be signed out: the sessions now come from the archive.

Attachments in the archive are written back through the storage interface, so it
lands on disk or in the object store depending on how the target is configured —
a backup taken from a disk instance restores into an S3 one and the other way
round. Attachments already in storage that the archive does not mention are left
alone.

There is no token path for restoring, unlike for backing up. Replacing the whole
holding should not be something a script can trigger.

### On a schedule

A button in a browser is an action, not a schedule. For a timer, a script needs a
way in, and it has no cookie — so **Settings → Wartung → Regelmäßig sichern**
generates a token for it and hands you the finished script, with the token and
the address already in it:

```sh
curl -fsS --max-time 3600 -H "Authorization: Bearer $WORT" \
     -o "$NAME" https://your-nexora/api/system/sicherung
unzip -l "$NAME" | grep -q '/FERTIG$' || { echo "unvollständig" >&2; exit 1; }
```

The `FERTIG` check is the part not to skip: a backup that broke off mid-stream is
still a valid ZIP, so without it a truncated archive lands in the rotation
looking exactly like a good one.

The token is separate from the metrics one because it is worth far more: it hands
out the entire holding without a sign-in. Every fetch with it is written to the
audit trail with its address. Remove it under the same heading when the script
goes away — and check first that none is still running, or it will back up into
nothing and say 401 to a log nobody reads.

**Attachments come along**, whether they sit on disk or in an object store: they
are read through the storage interface, so the same script covers both. That is
the gap a `pg_dump` timer alone leaves.


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
| Set a password for a forgotten one | Admin view → the account's row → *Passwort zurücksetzen* |
| Change your own password | The bar at the bottom left → *Passwort* (every account, not just admins) |
| Watch your own machines: reachable? which version? | Settings → System → Eigene Rechner |
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

### When it gets slow

Measured figures are in the [README](../README.md#capacity). The short version:
throughput saturates at about 20 concurrent requests and holds around 4,600
operations per second on a 12-core host, with no errors up to 3,200 concurrent.
It queues under load rather than failing.

If it does get slow, turn the knobs in this order.

**The connection pool, first.** `DATABASE_URL` carries no `pool_max_conns`, so
pgx defaults to one connection per core. Raising it to 50 measured 37 % more
throughput at 100 concurrent requests:

```
DATABASE_URL=postgres://nexora:...@db:5432/nexora?sslmode=disable&pool_max_conns=50
```

Keep it under PostgreSQL's `max_connections` (100 by default), and leave room
for the handful of connections that psql and backups take.

**Memory, much later.** `shared_buffers` sits at the container default of 128 MB.
That sounds small and usually is not: a Nexora database of a few thousand pages
is a few tens of megabytes, and it fits many times over. Check before changing
anything:

```bash
docker exec nexora-db-1 psql -U nexora -d nexora -tAc \
  "SELECT pg_size_pretty(pg_database_size(current_database()))"
docker exec nexora-db-1 psql -U nexora -d nexora -tAc \
  "SELECT round(100.0*sum(blks_hit)/(sum(blks_hit)+sum(blks_read)),2) FROM pg_stat_database WHERE datname=current_database()"
```

While the hit ratio stays above 99 %, PostgreSQL is answering from memory and
more of it buys nothing. When the database approaches a gigabyte, or the ratio
falls below about 95 %, add memory in `docker-compose.db.yml`:

```yaml
command:
  - postgres
  - -c
  - shared_buffers=1GB          # ~25 % of what the database may use
  - -c
  - effective_cache_size=3GB    # what the OS caches on top; a hint, not a reservation
  - -c
  - work_mem=32MB               # per sort, and there can be several per query
```

`shared_buffers` is reserved on start, `effective_cache_size` only informs the
planner. `work_mem` is the one to be careful with: it applies per sorting step,
so a large value times many concurrent queries is how a database gets itself
killed by the kernel.

**Attachments are not in the database.** They are files, and a slow object store
shows up as slow uploads while everything else stays fast. Settings → Verbund
carries the response time of each surrounding service.

### The system view

Nexora has **no access to the Docker socket**, on purpose: whoever holds that
socket is all-powerful on the host, and handing it in so a page can list
containers would be a bad bargain. What the system view reports is what can be
established without it — which services this one talks to, whether they answer,
how fast, which version they run, and how much they hold. Those are the
questions people actually ask when something is stuck.

Below it stands the second list, **Eigene Rechner**: machines you enter
yourself, the ones around this service. The same restraint applies there and for
the same reason — Nexora knocks, on the level where an answer needs no
credentials (a TCP connection that comes up, an HTTP response that arrives), and
holds no key to any of them. A wiki that may be reachable from outside is the
wrong place to keep a general key to your network. Redirects are not followed —
a 301 is already an answer, and where it points is a different question — and on
`https://` the certificate is not checked, since otherwise every self-signed
appliance in the house would show up as silent while running.

The version comes out of the knock itself. Whoever accepts a connection usually
says who they are in the first breath: an SSH service names its version before
anybody has been asked for a password, a web server names it in the `Server`
header. On a TLS target the certificate is read too, and how many days it still
has — under thirty the cell turns red, because an expired certificate is the
most common reason a service at home suddenly stops answering, and the only one
you could have seen coming. Whoever stays silent leaves the column empty;
nothing is guessed, and nothing outside is asked.

---

### What is encrypted, and where

| Hop | How |
|---|---|
| Browser → interface | The container's own certificate, self-signed on the first start (`PORT_TLS`, 3443). A real one goes into the `nexora_tls` volume under the same names |
| Interface → service | HTTPS, certificate verified against the stack's own authority |
| Service → database | `sslmode=verify-full`, the authority named in the connection string |
| Service → object store | HTTPS, verified |
| Service → cache | TLS, and the cache's plain port is closed (`--port 0`) rather than merely unused |

The certificates for the last four come from the `pki` container, which runs
once at start-up and then exits. It is idempotent: what is already in the volume
stays, so the authority does not change under you on every boot. Ten years is
its lifetime — long, deliberately: a certificate that expires in a home
installation nobody watches produces an outage nobody can explain.

Own certificates instead of the generated ones go into the `nexora_pki` volume
under the names each service expects; the generator only fills in what is
missing.

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
      terminates TLS — and, if pages are written on together, one that passes
      WebSocket upgrades through to `/api/echtzeit/`
- [ ] `oeffentliche_url` set to the address the browser actually uses
- [ ] `s3_tls` on if an object store is in use — on by default in the bundled
      stack, where the store speaks TLS anyway
- [ ] The `pki` container ran and exited cleanly. Everything between the
      containers hangs on it; there is nothing to configure, but there is
      something to look at once
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

### An upload is refused as an executable

A file whose first four bytes are the ELF magic is refused with 415, however it
is named — a Linux binary or shared object. The extension plays no part in the
decision: it is a claim by whoever uploads, while those four bytes are what the
kernel reads when something is started. Scripts are **not** refused: `#!/bin/sh`
is text, and a wiki that may no longer keep a documented backup script has lost
one of its purposes. The same check runs on import from an archive, where such a
file is skipped and named in the warnings rather than stopping the import.

### Uploads fail

The attachment directory does not belong to uid/gid 10001, or the file is larger
than `max_anhang_mb`, or larger than nginx's `client_max_body_size` (512 MiB in
the bundled configuration) — raising the setting alone is not enough. What the
real limit is can be measured under **Settings → Anhänge**.

### Writing together does not start, or reconnects for ever

The session runs over a WebSocket on `/api/echtzeit/{id}`. Anything in front has
to pass the upgrade through:

```nginx
location /api/echtzeit/ {
    proxy_pass http://nexora-frontend;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 86400s;
}
```

Without it the proxy answers 400 and the browser retries with a growing pause.
The bundled nginx already carries this; a second proxy in front of it, the usual
case in a home network, does not. The page keeps working meanwhile — it says
**Nicht verbunden** in the top bar, and the text is saved as before, whole.

Other reasons the session may not start at all: the licence does not include
`echtzeit`, the switch under **Settings → Zusammenarbeit** is off, or nobody
else has edit rights on the page — a page that only its owner may write on opens
no session, on purpose.

### Somebody forgot their password

Another administrator sets a new one in the **Admin view**, in that account's
row. Every session of the account is ended along with it, so a device somebody
else is holding loses access at the same moment. Hand the new password over on a
different channel than email and let the person set their own afterwards, at the
bottom left of the sidebar.

Two cases this does not cover. An account that signs in through SSO or the
directory has no password here, and setting one would take its sign-in away, so
it is refused — the password of such an account belongs in the provider. And an
instance whose **only** administrator is locked out has no way in through the
interface; there the account has to be repaired in the database, which is what
the backup and a fresh `password_hash` are for.

### The machine list says "still" although the machine is up

It reports what it can establish, and that is one thing only: does something
answer on that address and port. A machine that is up but has nothing listening
on the port you entered is silent by that measure, and rightly so. Check the
port first — SSH is 22, a web service is whatever it publishes. The list refuses
an address without a port on purpose rather than guessing one, because a guessed
port produces exactly this misleading answer.

If the Fassung column stays empty, that service simply does not introduce
itself: PostgreSQL, a database or a plain forwarder say nothing on connect, and
a web server may be configured to keep its `Server` header to itself. That is
their right, and nothing is invented to fill the gap.

### After a rebuild nothing talks to anything any more

Check the `pki` container first: `docker compose logs pki`. It has to have run
and exited cleanly, and every other service waits for exactly that. If the
volume was removed (`docker compose down -v`) a new authority is generated, and
then everything fits together again — but only after every container has been
restarted, because the old ones still hold the old certificates.

`x509: certificate signed by unknown authority` in the service's log means it
does not know the authority: `tls_wurzel` is unset or points at a file that is
not there. `certificate is valid for X, not Y` means a service was renamed —
the name is in the certificate, so `docker compose down` and up again, having
removed that service's directory from the volume so it gets a fresh one.

If the interface answers 502 and its log says `SSL_do_handshake() failed`, the
service behind it is speaking plain HTTP while the interface expects TLS. That
is the case when the service runs without `tls_zertifikat`; then
`NEXORA_DIENST_SCHEMA=http` and `NEXORA_DIENST_PORT=8080` belong in the `.env`.

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

If nothing there stands out, the knobs and the order to turn them are under
[When it gets slow](#when-it-gets-slow). The connection pool is the first one
and the memory setting is almost never the right one.

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
