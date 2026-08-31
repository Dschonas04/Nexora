# Configuration Reference

Every setting Nexora reads, what it does, its default, and the environment
variable that overrides it.

## How settings are resolved

```
environment variable   →   config.conf   →   built-in default
```

The environment wins so a container can override a single value without a
rebuilt image, and so a secret never has to touch disk.

The file is looked for in this order, first hit wins:

1. `$NEXORA_CONFIG`
2. `./config.conf`
3. `/etc/nexora/config.conf`

**A missing file is not an error.** Every setting has a default that produces a
working server, which is what lets the binary start with no configuration at
all. The startup log says which file was read, or that none was found.

### File format

```ini
schluessel = wert          # one entry per line; # and ; start a comment
[Abschnitt]                # sections are read and discarded, they only group
"  wert mit Rand  "        # quotes preserve leading and trailing spaces
```

Broken lines do not bring anything down: a line without `=` is reported with its
line number and skipped, and a value that should be a number and is not keeps
the default with a note in the log. A typo must not cause an outage.

### Yes/no values

Any of `1 ja true an yes on` is true, any of `0 nein false aus no off` is false.
Anything else keeps the default and is logged.

### Editing it from the browser

**Settings → Wartung** shows and writes exactly the file that was read. It checks
the draft before writing, keeps a timestamped backup of the previous version,
masks credentials on the way to the browser and restores them on the way back —
so saving never overwrites a secret with asterisks.

Because the file is only read at startup, that page also carries a restart
button: the process ends and whatever supervises it brings it back.

For the page to be able to save, the file has to belong to the group the service
runs under in the container:

```bash
sudo chgrp 10001 config.conf && sudo chmod 660 config.conf
```

Without this it is shown read-only.

---

## Server

| Key | Environment | Default | What it does |
|---|---|---|---|
| `port` | `PORT` | `8080` | The port the API listens on. In the compose setup this is the container-internal port; what you publish is `PORT` in `.env`, which maps to the *frontend* container |
| `daten_verzeichnis` | `NEXORA_DATA_DIR` | `/data/attachments` | The data directory |
| `anhang_verzeichnis` | `NEXORA_ANHANG_PFAD` | *(empty)* | Where attachment bytes go, if not the data directory. Attachments are the only part that grows without bound, so they often belong on a disk of their own. Empty means the same directory as before, so an upgrade does not move an existing installation's files |
| `oeffentliche_url` | `NEXORA_PUBLIC_URL` | *(empty)* | The address the browser actually uses. Needed to build the OIDC callback and to name the right host in a public share link. Required as soon as `oidc_aktiv` is on |

## Database

| Key | Environment | Default | What it does |
|---|---|---|---|
| `datenbank_url` | `DATABASE_URL` | `postgres://nexora:nexora@localhost:5432/nexora?sslmode=disable` | The connection string. The default password is warned about at every boot |

**PostgreSQL fixes the password at first launch.** It is written into the data
directory at initialisation, so changing it afterwards locks the backend out
unless you also change it inside the running database
(`ALTER USER nexora WITH PASSWORD ...`) or discard the volume.

## Sessions

| Key | Environment | Default | What it does |
|---|---|---|---|
| `jwt_geheimnis` | `JWT_SECRET` | `change-me-in-production` | Signs the session token. On the default, every session is forgeable — warned about at every boot. `openssl rand -hex 32` |
| `sitzung_stunden` | `NEXORA_SESSION_HOURS` | `12` | How long a session lasts. One in use renews itself once half its life is gone |
| `sitzung_tage` | `NEXORA_SESSION_DAYS` | — | **Deprecated.** The old key in days. Still read and converted (× 24), so an existing file does not silently drop from seven days to twelve hours because the unit changed |

## Licence

| Key | Environment | Default | What it does |
|---|---|---|---|
| `lizenz` | `NEXORA_LIZENZ` | *(empty)* | The licence key. A key imported through the admin pages takes precedence over this one, or an import in the browser would revert on the next restart. A missing or invalid key is never fatal |

Issuing keys needs `NEXORA_SIGNIERSCHLUESSEL`, the Ed25519 private key. Without
it the issuing endpoint answers 501 and the section does not appear in the
interface. See [`backend/premium/README.md`](../backend/premium/README.md).

## Registration

| Key | Environment | Default | What it does |
|---|---|---|---|
| `registrierung_offen` | `NEXORA_REGISTRIERUNG_OFFEN` | `ja` | Whether anyone may create an account. **The very first account created becomes the administrator** — turning this off before that account exists locks everybody out |
| `erlaubte_domaenen` | `NEXORA_ERLAUBTE_DOMAENEN` | *(empty)* | Comma-separated list of email domains that may register. Empty means all |

Both are also editable at runtime from the settings page, and the stored value
then wins over the file — it was set later and on purpose.

## Search

| Key | Environment | Default | What it does |
|---|---|---|---|
| `such_woerterbuch` | `NEXORA_SUCH_WOERTERBUCH` | `german` | The PostgreSQL text search configuration. `german` reaches across word forms in German text and costs a little precision in English; `simple` does neither. Changing it needs a reindex: `POST /api/system/suchindex` |

## Attachments

| Key | Environment | Default | What it does |
|---|---|---|---|
| `max_anhang_mb` | `NEXORA_MAX_ANHANG_MB` | `25` | Largest upload. nginx caps the body at 25 MiB too (`client_max_body_size`), so raising this alone is not enough |

## Trash

| Key | Environment | Default | What it does |
|---|---|---|---|
| `papierkorb_tage` | `NEXORA_PAPIERKORB_TAGE` | `30` | After how many days a page in the trash disappears for good. `0` disables the sweep and pages stay until somebody empties it. The hourly sweep removes the attachment bytes too |

## Object storage (S3)

| Key | Environment | Default | What it does |
|---|---|---|---|
| `s3_aktiv` | `NEXORA_S3_AKTIV` | `nein` | Store attachments in a bucket instead of on disk |
| `s3_endpunkt` | `NEXORA_S3_ENDPUNKT` | *(empty)* | Host and port of the store, **as seen from this container**. If MinIO runs in another compose project, that is the host's address with the published port, not the container name |
| `s3_bucket` | `NEXORA_S3_BUCKET` | `nexora` | The bucket |
| `s3_zugriffsschluessel` | `NEXORA_S3_ZUGRIFFSSCHLUESSEL` | *(empty)* | Access key |
| `s3_geheimnis` | `NEXORA_S3_GEHEIMNIS` | *(empty)* | Secret key |
| `s3_region` | `NEXORA_S3_REGION` | `us-east-1` | Region. Self-hosted stores generally do not care |
| `s3_tls` | `NEXORA_S3_TLS` | `nein` | HTTPS to the store. Off means keys and files travel in the clear — warned about |
| `s3_pfadstil` | `NEXORA_S3_PFADSTIL` | `ja` | Path-style addressing (`endpoint/bucket/key`). What MinIO, Garage and Ceph want; AWS wants it off |
| `s3_rueckfall` | `NEXORA_S3_RUECKFALL` | `nein` | Whether a store that does not answer at startup may be replaced by the local disk. **Off by default on purpose**: the instance would come up, uploads would work, and weeks later half the attachments would be in a directory nobody backs up |

The settings page can bind a store interactively and test it
(`POST /api/system/ablage/test`). Credentials are deliberately *not* kept in the
settings table — a secret access key does not belong in a row that a database
dump carries off.

## Redis (optional cache)

| Key | Environment | Default | What it does |
|---|---|---|---|
| `redis_adresse` | `NEXORA_REDIS_ADRESSE` | *(empty)* | `host:port`. Empty means no Redis, and everything works without it |
| `redis_passwort` | `NEXORA_REDIS_PASSWORT` | *(empty)* | |
| `redis_datenbank` | `NEXORA_REDIS_DATENBANK` | `0` | |
| `redis_vorsilbe` | `NEXORA_REDIS_VORSILBE` | `nexora` | Prefix in front of every key, so two instances can share one Redis |

Redis is a cache, never the source of truth. A failure to connect is logged, not
fatal.

## Metrics for Prometheus

| Key | Env | Default | Meaning |
|---|---|---|---|
| `metriken_token` | `NEXORA_METRIKEN_TOKEN` | *(empty)* | Bearer token for `GET /metrics`. Empty means the endpoint does not exist |

Empty is the default and the endpoint then answers **404**, not 401: the figures
say how many people work here and when, and that they exist is not something a
caller who may not fetch them needs to learn. Set a token and Prometheus can
scrape it:

```yaml
  - job_name: 'nexora'
    metrics_path: /metrics
    authorization:
      credentials: 'the same token'
    static_configs:
      - targets: ['nexora-host:3000']
```

The endpoint sits outside `/api`, since a scraper brings no session cookie, and
it does not count itself — otherwise the request rate would never be zero even
when nobody is working. A ready-made Grafana dashboard is in
[`grafana/nexora.json`](../grafana/nexora.json).

The in-app system view covers the last minute and answers *what is happening
now*. This covers *what happened at three in the morning*, which is the question
one actually has when somebody complains about yesterday.

## LDAP / Active Directory · paid extra `ldap`

| Key | Environment | Default | What it does |
|---|---|---|---|
| `ldap_aktiv` | `NEXORA_LDAP_AKTIV` | `nein` | |
| `ldap_server` | `NEXORA_LDAP_SERVER` | *(empty)* | `ldap://host:389` or `ldaps://host:636` |
| `ldap_starttls` | `NEXORA_LDAP_STARTTLS` | `ja` | Upgrade a plain connection to TLS. Off, without `ldaps://`, means credentials cross the network in the clear — warned about |
| `ldap_tls_pruefen` | `NEXORA_LDAP_TLS_PRUEFEN` | `ja` | Verify the server certificate. Off is warned about |
| `ldap_bind_dn` | `NEXORA_LDAP_BIND_DN` | *(empty)* | The account used to search for users |
| `ldap_bind_passwort` | `NEXORA_LDAP_BIND_PASSWORT` | *(empty)* | |
| `ldap_basis_dn` | `NEXORA_LDAP_BASIS_DN` | *(empty)* | Where the search starts |
| `ldap_benutzer_filter` | `NEXORA_LDAP_BENUTZER_FILTER` | `(&(objectClass=person)(\|(uid=%s)(sAMAccountName=%s)(mail=%s)))` | `%s` is what was typed at sign-in |
| `ldap_feld_name` | `NEXORA_LDAP_FELD_NAME` | `cn` | Attribute holding the display name |
| `ldap_feld_email` | `NEXORA_LDAP_FELD_EMAIL` | `mail` | Attribute holding the address. Accounts are linked by **verified email** |
| `ldap_gruppe_admin` | `NEXORA_LDAP_GRUPPE_ADMIN` | *(empty)* | Membership in this group grants the admin role |

## OIDC / Keycloak · paid extra `sso`

| Key | Environment | Default | What it does |
|---|---|---|---|
| `oidc_aktiv` | `NEXORA_OIDC_AKTIV` | `nein` | Requires `oeffentliche_url`, or the callback cannot be built — warned about |
| `oidc_aussteller` | `NEXORA_OIDC_AUSSTELLER` | *(empty)* | Issuer URL. Anything publishing a discovery document works |
| `oidc_client_id` | `NEXORA_OIDC_CLIENT_ID` | *(empty)* | |
| `oidc_geheimnis` | `NEXORA_OIDC_GEHEIMNIS` | *(empty)* | Client secret |
| `oidc_bereiche` | `NEXORA_OIDC_BEREICHE` | `openid email profile` | Scopes |
| `oidc_feld_name` | `NEXORA_OIDC_FELD_NAME` | `name` | Claim holding the display name |
| `oidc_feld_email` | `NEXORA_OIDC_FELD_EMAIL` | `email` | Claim holding the address |
| `oidc_gruppe_admin` | `NEXORA_OIDC_GRUPPE_ADMIN` | *(empty)* | Membership in this group grants the admin role |
| `oidc_knopf_text` | `NEXORA_OIDC_KNOPF_TEXT` | `Mit SSO anmelden` | Label of the button on the sign-in page |

The callback to register with the provider is
`<oeffentliche_url>/api/auth/oidc/zurueck`.

Neither OIDC nor LDAP ever takes over an account that has its own password. Both
link by verified email address.

---

## Settings that live in the database, not the file

These are changed at runtime from **Settings**, are stored in the
`einstellungen` table, and override what the file says — they were set later and
on purpose:

`registrierung_offen` · `erlaubte_domaenen` · `max_anhang_mb` ·
`sitzung_stunden` · `papierkorb_tage` · `such_woerterbuch` ·
`design_grundton` (`weiss` | `grau` | `dunkel`) · `design_akzent` (a colour)

The database URL, the port and the JWT secret deliberately do **not** live here:
they are needed before the database is open.

---

## `.env` — what Docker Compose reads

Compose reads `.env` for the values it needs before the backend starts.

| Variable | Purpose |
|---|---|
| `COMPOSE_FILE` | Which compose files make up the stack. Default brings PostgreSQL along |
| `PORT` | Host port for the interface (3000) |
| `PORT_TLS` | Host port for HTTPS (3443) |
| `POSTGRES_PASSWORD` | Used by both the database and the backend's `DATABASE_URL` |
| `DATABASE_URL` | Set this instead when you run your own database |
| `JWT_SECRET` | Overrides `jwt_geheimnis` |
| `NEXORA_LIZENZ` | Overrides `lizenz` |
| `NEXORA_ANHANG_ORT` | **What gets mounted** onto the attachment path — a named volume by default, or a host directory / share |
| `NEXORA_ANHANG_PFAD` | The path **inside** the container. Moving attachments changes the *Ort*, not this |
| `NEXORA_TLS_NAME` | Name in the self-signed certificate. Without it the container's hostname is used, which produces a second browser warning |
| `NEXORA_TLS_IP` | An IP for the certificate's SAN |
| `NEXORA_S3_*` | Wire up an existing object store without the MinIO side file |

A directory given as `NEXORA_ANHANG_ORT` has to belong to **uid/gid 10001**, the
account the service runs under in the container. Changing the setting does not
move files that are already there — carry them over first, then restart.

---

## Startup warnings

Named at every boot, none of them fatal:

```
ACHTUNG: jwt_geheimnis steht auf der Vorgabe, jede Sitzung ist fälschbar
ACHTUNG: datenbank_url benutzt das Vorgabepasswort
ACHTUNG: oidc_aktiv ohne oeffentliche_url, die Rücksprungadresse lässt sich nicht bilden
ACHTUNG: s3_aktiv ohne s3_endpunkt, Anhänge landen weiter auf der Platte
ACHTUNG: s3_rueckfall=ja, bei einer Störung des Objektspeichers landen neue Anhänge doch auf der Platte
ACHTUNG: S3 ohne TLS, Zugangsschlüssel und Dateien gehen unverschlüsselt über das Netz
ACHTUNG: ldap_aktiv ohne ldap_server, die Anmeldung fällt auf Passwörter zurück
ACHTUNG: LDAP ohne TLS, Zugangsdaten gehen im Klartext über das Netz
ACHTUNG: ldap_tls_pruefen=nein, das Serverzertifikat wird nicht geprüft
```

A home-lab install on default settings should still run. It should just be
impossible to miss that it did.
