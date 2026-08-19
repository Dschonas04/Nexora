# Go 1.25, weil minio-go das mindestens verlangt -- siehe go.mod.
FROM golang:1.25-alpine AS build
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod ./
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /nexora ./

FROM alpine:3.19
RUN adduser -D -u 10001 nexora \
 && mkdir -p /data/attachments \
 && chown -R nexora:nexora /data
COPY --from=build /nexora /usr/local/bin/nexora
# Declared so a fresh named volume inherits nexora ownership (uid 10001) instead
# of defaulting to root — otherwise the non-root process can't write uploads.
VOLUME ["/data/attachments"]
USER nexora
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/nexora"]
