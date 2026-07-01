FROM golang:1.22-alpine AS build
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod ./
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /nexora ./

FROM alpine:3.19
RUN adduser -D -u 10001 nexora
COPY --from=build /nexora /usr/local/bin/nexora
USER nexora
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/nexora"]
