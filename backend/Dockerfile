# Go 1.25, because minio-go demands at least that, see go.mod.
FROM golang:1.25-alpine AS build
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod ./
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /nexora ./

FROM alpine:3.19
# Two tools the service invokes as separate processes.
#
# poppler-utils supplies pdftotext for the full text out of PDF attachments.
# Purely in Go it would work without it, but it fails on many real PDF files:
# font encodings, columns, embedded images.
#
# postgresql16-client supplies pg_dump for the backup. The dump comes out of the
# tool and not out of SQL output of our own: what gets backed up has to be
# restorable, and pg_dump produces exactly the form psql reads back in. Three
# megabytes in the image for that.
#
# MIND the major version: pg_dump refuses to back up a NEWER server. If 16
# stands here and the database runs on 17, this line has to change with it.
RUN apk add --no-cache poppler-utils postgresql16-client \
 && adduser -D -u 10001 nexora \
 && mkdir -p /data/attachments \
 && chown -R nexora:nexora /data
# What is in this image, the image says itself. Whoever passes it on
# distributes GPL programs (poppler-utils, busybox) and owes the corresponding
# source for them; the label says where it lies. See THIRD-PARTY.md in the
# source tree.
LABEL org.opencontainers.image.title="Nexora Backend" \
      org.opencontainers.image.source="https://github.com/Dschonas04/Nexora" \
      org.opencontainers.image.licenses="BUSL-1.1" \
      org.nexora.fremde-bestandteile="Enthaelt GPL-2.0-Programme aus Alpine, darunter poppler-utils. Nexora ruft sie als eigene Prozesse auf und bleibt unter BUSL-1.1." \
      org.nexora.gpl-quellen="https://gitlab.alpinelinux.org/alpine/aports"

COPY --from=build /nexora /usr/local/bin/nexora
# Declared so a fresh named volume inherits nexora ownership (uid 10001) instead
# of defaulting to root — otherwise the non-root process can't write uploads.
VOLUME ["/data/attachments"]
USER nexora
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/nexora"]
