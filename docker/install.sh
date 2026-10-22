#!/bin/sh
# Installs Nexora with Docker, from the published images.
#
#     ./install.sh            Nexora, its database and the internal PKI
#     ./install.sh single     everything in one container
#     ./install.sh redis      the stack plus a Redis cache
#
# On the first run it writes a .env with random passwords for the database
# and the session signing; on every later run it keeps that .env and only
# pulls and restarts -- which is also how an update is done.
#
# Version and registry come from the .env (NEXORA_VERSION, NEXORA_REGISTRY);
# without them it is the newest release from ghcr.io.
set -eu

cd "$(dirname "$0")"

say() { printf '%s\n' "$*"; }
fail() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

command -v docker >/dev/null 2>&1 || fail "Docker is not installed (https://docs.docker.com/engine/install/)."
docker compose version >/dev/null 2>&1 || fail "the Compose plugin is missing (docker compose)."

variante=${1:-stack}
case "$variante" in
  stack) dateien=docker-compose.stack.yml ;;
  redis) dateien=docker-compose.stack.yml:docker-compose.redis.yml ;;
  single) dateien=docker-compose.single.yml ;;
  *) fail "unknown variant '$variante' (stack, redis or single)." ;;
esac

# 32 random bytes as hex, from openssl where there is one.
zufall() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  else
    od -An -tx1 -N32 /dev/urandom | tr -d ' \n'
  fi
}

# Sets KEY=value in .env, replacing the line if it is there.
setze() {
  if grep -q "^$1=" .env; then
    # | as the delimiter: values here are hex, file names and colons.
    sed -i.alt "s|^$1=.*|$1=$2|" .env && rm -f .env.alt
  else
    printf '%s=%s\n' "$1" "$2" >> .env
  fi
}

if [ -f .env ]; then
  say "Keeping the existing .env."
  # A variant given on the command line switches the files, nothing else.
  [ $# -gt 0 ] && setze COMPOSE_FILE "$dateien"
else
  cp .env.example .env
  chmod 600 .env
  setze COMPOSE_FILE "$dateien"
  setze POSTGRES_PASSWORD "$(zufall)"
  setze JWT_SECRET "$(zufall)"
  say "Wrote .env with random passwords (keep it: without it the database stays locked)."
fi

# The configuration file of the build-from-source variant. The image stack
# does not mount it, but a later switch should find it in place.
[ -f config.conf ] || cp ../config.example.conf config.conf

say "Pulling the images…"
docker compose pull --quiet
say "Starting…"
docker compose up -d

port=$(grep -E '^PORT=' .env | tail -1 | cut -d= -f2)
port=${port:-3000}
tls=$(grep -E '^PORT_TLS=' .env | tail -1 | cut -d= -f2)
tls=${tls:-3443}
[ "$variante" = single ] && port=3000 && tls=3443

# Not "the container started" but the view from outside: the interface answers
# and the API asks for a sign-in.
i=0
while [ $i -lt 60 ]; do
  i=$((i + 1))
  api=$(curl -sk -o /dev/null -w '%{http_code}' --max-time 5 "https://127.0.0.1:$tls/api/auth/me" 2>/dev/null || true)
  if [ "$api" = 401 ]; then
    say ""
    say "Nexora is running:  https://<this host>:$tls"
    [ "$variante" != single ] && say "                    http://<this host>:$port"
    say "The first account you register becomes the administrator."
    exit 0
  fi
  sleep 3
done
say "Nexora did not answer within three minutes. What the services say:" >&2
docker compose ps >&2
docker compose logs --tail 30 >&2
exit 1
