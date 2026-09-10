# A container that runs once and is then done.
#
# It issues the certificates for the internal traffic and ends. All other
# services wait for it to end, see docker-compose.yml. An image of our own and
# no reaching into a foreign one: this is where the key the whole compound
# trusts comes into being, and whatever issues that one should be something you
# built yourself.
FROM alpine:3.19
RUN apk add --no-cache openssl
COPY erzeuge.sh /usr/local/bin/erzeuge.sh
RUN chmod +x /usr/local/bin/erzeuge.sh

LABEL org.opencontainers.image.title="Nexora PKI" \
      org.opencontainers.image.source="https://github.com/Dschonas04/Nexora" \
      org.opencontainers.image.licenses="BUSL-1.1" \
      org.nexora.gpl-quellen="https://gitlab.alpinelinux.org/alpine/aports"

ENTRYPOINT ["/usr/local/bin/erzeuge.sh"]
