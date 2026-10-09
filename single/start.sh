#!/bin/sh
# Starts the three parts of the single container and watches them: PostgreSQL,
# the Nexora service and nginx. If one of them stops, the container stops, and
# Docker's restart policy brings it back as a whole. Half a Nexora that looks
# alive is worse than one that is visibly down.
set -eu

DATA=/data
PG="$DATA/postgres"
mkdir -p "$DATA/attachments" "$DATA/tls" "$DATA/log" /run/postgresql
chown nexora:nexora "$DATA/attachments"
chown postgres:postgres /run/postgresql "$DATA/log"

# The certificate nginx generates on the first start belongs in the volume,
# or the browser warns about a new one after every update.
if [ ! -L /etc/nginx/tls ]; then
    rm -rf /etc/nginx/tls
    ln -s "$DATA/tls" /etc/nginx/tls
fi

# The session secret: given from outside, or generated once and kept. A fixed
# value in the image would be the same on every installation in the world.
if [ -z "${JWT_SECRET:-}" ]; then
    if [ ! -s "$DATA/jwt_secret" ]; then
        (umask 077; openssl rand -hex 32 > "$DATA/jwt_secret")
        echo "Sitzungsschlüssel erzeugt ($DATA/jwt_secret)"
    fi
    JWT_SECRET=$(cat "$DATA/jwt_secret")
fi
export JWT_SECRET

# PostgreSQL listens on 127.0.0.1 only. The service signs in with a password of
# its own, generated once and kept in the volume; the superuser can only be
# reached as the system user postgres over the socket. Without that, any
# process in the container could act as superuser, and a hole in the service
# would be a hole in the whole database.
if [ ! -s "$DATA/db_password" ]; then
    (umask 077; openssl rand -hex 24 > "$DATA/db_password")
fi
DB_PASSWORD=$(cat "$DATA/db_password")
PGOPTS="-c listen_addresses=127.0.0.1 -c shared_buffers=64MB -c max_connections=30 -c password_encryption=scram-sha-256"
if [ ! -s "$PG/PG_VERSION" ]; then
    echo "Datenbank wird angelegt"
    mkdir -p "$PG"
    chown postgres:postgres "$PG"
    chmod 700 "$PG"
    su-exec postgres initdb -D "$PG" -U postgres -E UTF8 --auth-local=peer --auth-host=scram-sha-256 >/dev/null
    NEU=1
fi
# Written on every start, so that an installation from 2.2.0, which trusted
# every local connection, is tightened on its next start.
cat > "$PG/pg_hba.conf" <<HBA
local   all   postgres                 peer
local   all   all                      scram-sha-256
host    all   all   127.0.0.1/32       scram-sha-256
HBA
chown postgres:postgres "$PG/pg_hba.conf"
chmod 600 "$PG/pg_hba.conf"
rm -f "$PG/postmaster.pid"
su-exec postgres pg_ctl -D "$PG" -o "$PGOPTS" -l "$DATA/log/postgres.log" -w start >/dev/null
if [ -n "${NEU:-}" ]; then
    su-exec postgres psql -q -v ON_ERROR_STOP=1 \
        -c "CREATE ROLE nexora LOGIN" -c "CREATE DATABASE nexora OWNER nexora"
    su-exec postgres psql -q -v ON_ERROR_STOP=1 -d nexora -c "CREATE EXTENSION IF NOT EXISTS pgcrypto"
fi
# Over stdin, so the password shows up in no process list.
printf "ALTER ROLE nexora WITH LOGIN NOSUPERUSER PASSWORD '%s';\n" "$DB_PASSWORD" \
    | su-exec postgres psql -q -v ON_ERROR_STOP=1 >/dev/null
echo "Datenbank läuft"

stop() {
    echo "Nexora wird beendet"
    [ -n "${WEB:-}" ] && kill "$WEB" 2>/dev/null || true
    [ -n "${SERVICE:-}" ] && kill "$SERVICE" 2>/dev/null || true
    wait 2>/dev/null || true
    su-exec postgres pg_ctl -D "$PG" -m fast -w stop >/dev/null 2>&1 || true
    exit "${1:-0}"
}
trap stop TERM INT

su-exec nexora env \
    DATABASE_URL="postgres://nexora:$DB_PASSWORD@127.0.0.1:5432/nexora?sslmode=disable" \
    PORT=8080 \
    NEXORA_ATTACHMENT_PATH="$DATA/attachments" \
    /usr/local/bin/nexora &
SERVICE=$!

/usr/local/bin/tls-start.sh &
WEB=$!

# Watch all three. busybox sh knows no "wait -n", so it looks every few seconds.
while :; do
    if ! kill -0 "$SERVICE" 2>/dev/null; then echo "Der Dienst ist beendet" >&2; break; fi
    if ! kill -0 "$WEB" 2>/dev/null; then echo "nginx ist beendet" >&2; break; fi
    if ! su-exec postgres pg_ctl -D "$PG" status >/dev/null 2>&1; then echo "PostgreSQL ist beendet" >&2; break; fi
    sleep 5 &
    wait $! || true
done
stop 1
