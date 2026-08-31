# Go 1.25, weil minio-go das mindestens verlangt, siehe go.mod.
FROM golang:1.25-alpine AS build
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod ./
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /nexora ./

FROM alpine:3.19
# poppler-utils supplies pdftotext for the full text in PDF attachments.
# Extraction code purely in Go would manage without this dependency but fails on
# many real PDFs: font encodings, columns, embedded images.
RUN apk add --no-cache poppler-utils \
 && adduser -D -u 10001 nexora \
 && mkdir -p /data/attachments \
 && chown -R nexora:nexora /data
# Was in diesem Abbild steckt, sagt das Abbild selbst. Wer es weitergibt,
# verteilt GPL-Programme (poppler-utils, busybox) und schuldet dafür den
# zugehörigen Quelltext; die Kennzeichnung nennt, wo er liegt. Siehe
# THIRD-PARTY.md im Quelltextbestand.
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
