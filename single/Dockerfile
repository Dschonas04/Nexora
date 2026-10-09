# Nexora in one container: the database, the service and the web server
# together, for a quick start, a small team or a machine where a whole stack is
# too much. Built from the repository root:
#
#     docker build -f single/Dockerfile -t nexora-single .
#
# It runs the free Standard scope only (built with the tag nur_kern), so no key
# is needed and none is read. Whoever wants Pro or Business, or more than a
# handful of people at once, takes the stack with separate containers; see
# docs/operations.md.
#
# Everything that has to survive lies under /data: the database, the
# attachments, the generated session secret and the self-signed certificate.

FROM golang:1.27.1-alpine AS backend
WORKDIR /app
RUN apk add --no-cache git
COPY backend/ ./
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -trimpath -tags nur_kern -o /nexora ./

FROM node:26-alpine AS frontend
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# The frontend's base, because its start script and nginx template are reused
# unchanged: they already know how to talk to a service over plain http.
FROM nginx:1.31-alpine
# postgresql16 brings the server and pg_dump for the backup, -contrib brings
# pgcrypto. poppler-utils is what the backend calls for text out of PDFs.
# su-exec starts each part under its own user.
RUN apk add --no-cache postgresql16 postgresql16-contrib poppler-utils openssl gettext su-exec \
 && adduser -D -u 10001 nexora \
 && mkdir -p /data
COPY frontend/nginx.conf /etc/nginx/vorlage.conf
COPY frontend/tls-start.sh /usr/local/bin/tls-start.sh
COPY single/start.sh /usr/local/bin/nexora-single
COPY --from=frontend /app/dist /usr/share/nginx/html
COPY --from=backend /nexora /usr/local/bin/nexora
RUN chmod +x /usr/local/bin/tls-start.sh /usr/local/bin/nexora-single

# The web server reaches the service inside the container, unencrypted, at a
# fixed address: there is no network between them to protect.
ENV NEXORA_SERVICE_SCHEME=http \
    NEXORA_SERVICE_HOST=127.0.0.1 \
    NEXORA_SERVICE_PORT=8080 \
    NEXORA_RESOLVER=127.0.0.1

LABEL org.opencontainers.image.title="Nexora (single container)" \
      org.opencontainers.image.description="Nexora Standard with its database in one container" \
      org.opencontainers.image.source="https://github.com/Dschonas04/Nexora" \
      org.opencontainers.image.licenses="BUSL-1.1" \
      org.nexora.fremde-bestandteile="Enthaelt PostgreSQL (PostgreSQL License), GPL-2.0-Programme aus Alpine (poppler-utils, busybox) und MPL-2.0-Code (BlockNote). Nexora ruft sie als eigene Prozesse auf und bleibt unter BUSL-1.1." \
      org.nexora.gpl-quellen="https://gitlab.alpinelinux.org/alpine/aports"

VOLUME ["/data"]
EXPOSE 80 443
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
CMD ["/usr/local/bin/nexora-single"]
