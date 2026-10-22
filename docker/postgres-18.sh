#!/bin/sh
# Carries the database of a compose installation from PostgreSQL 16 to 18.
#
# Run it in the directory of the installation, after pulling the new compose
# files, with the same files the installation uses (or with COMPOSE_FILE set in
# .env, then without arguments):
#
#     sh postgres-18.sh   (in docker/) -f docker-compose.yml -f docker-compose.db.yml
#
# What it does: stops the stack, starts the old volume once more under
# PostgreSQL 16 to dump everything, keeps a copy of the old volume as
# <volume>-pg16, starts a fresh PostgreSQL 18 in the original volume, loads the
# dump into it and brings the stack back up. The dump stays next to this script
# as nexora-pg16-<date>.sql. Nothing of the old data is deleted.
set -eu
# The dump holds every page and every password hash: readable by the owner only.
umask 077

DC="docker compose $*"

DB=$($DC ps -a -q db)
if [ -z "$DB" ]; then
    echo "Kein db-Container gefunden. Mit denselben -f-Dateien aufrufen wie die Installation." >&2
    exit 1
fi
VOL=$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/var/lib/postgresql/data"}}{{.Name}}{{end}}{{end}}' "$DB")
if [ -z "$VOL" ]; then
    echo "Das Datenverzeichnis ist kein benanntes Volume; bitte von Hand umziehen (docs/operations.md)." >&2
    exit 1
fi

STAND=$(docker run --rm -v "$VOL":/v alpine:3.24 cat /v/PG_VERSION 2>/dev/null || true)
case "$STAND" in
    18) echo "Volume $VOL läuft schon auf PostgreSQL 18, nichts zu tun."; exit 0 ;;
    16) ;;
    *) echo "Unerwarteter Stand in $VOL: '$STAND'" >&2; exit 1 ;;
esac

BENUTZER=$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$DB" | sed -n 's/^POSTGRES_USER=//p')
BENUTZER=${BENUTZER:-nexora}
DUMP="nexora-pg16-$(date +%Y%m%d-%H%M%S).sql"

echo "== Stack anhalten"
$DC stop

echo "== Alles aus PostgreSQL 16 sichern nach $DUMP"
docker run -d --name nexora-pg16-umzug -e PGDATA=/var/lib/postgresql/data \
    -v "$VOL":/var/lib/postgresql/data postgres:16-alpine >/dev/null
trap 'docker rm -f nexora-pg16-umzug >/dev/null 2>&1 || true' EXIT
i=0
until docker exec nexora-pg16-umzug pg_isready -q -U "$BENUTZER"; do
    i=$((i + 1)); [ $i -lt 60 ] || { echo "PostgreSQL 16 startet nicht" >&2; exit 1; }
    sleep 1
done
docker exec nexora-pg16-umzug pg_dumpall -U "$BENUTZER" > "$DUMP"
docker rm -f nexora-pg16-umzug >/dev/null
[ -s "$DUMP" ] || { echo "Die Sicherung ist leer, Abbruch" >&2; exit 1; }

echo "== Altes Volume aufheben als $VOL-pg16"
docker volume create "$VOL-pg16" >/dev/null
docker run --rm -v "$VOL":/alt -v "$VOL-pg16":/neu alpine:3.24 sh -c 'cp -a /alt/. /neu/ && rm -rf /alt/* /alt/.[!.]* 2>/dev/null; true'

echo "== PostgreSQL 18 anlegen und die Sicherung einlesen"
$DC up -d db
i=0
# Over TCP: during the first initialisation the image runs a temporary server
# on the socket only, and loading into that one would be lost on its restart.
until $DC exec -T db pg_isready -q -h 127.0.0.1 -U "$BENUTZER" 2>/dev/null; do
    i=$((i + 1)); [ $i -lt 90 ] || { echo "PostgreSQL 18 startet nicht" >&2; exit 1; }
    sleep 1
done
# The dump also creates the role and the database that the new container just
# made itself; those two "already exists" messages are expected.
$DC exec -T db psql -q -U "$BENUTZER" -d postgres < "$DUMP" 2>&1 >/dev/null | grep -v "already exists" || true

echo "== Stack starten"
$DC up -d
echo "Fertig. Sicherung: $DUMP (enthält alle Inhalte und Passwort-Hashes), altes Volume: $VOL-pg16."
echo "Beides löschen, sobald alles läuft."
