# Ein Container, der einmal läuft und dann fertig ist.
#
# Er stellt die Zertifikate für den Innenverkehr aus und beendet sich. Alle
# anderen Dienste warten auf sein Ende, siehe docker-compose.yml. Ein eigenes
# Abbild und kein Griff in ein fremdes: hier entsteht der Schlüssel, dem der
# ganze Verbund glaubt, und was den ausstellt, sollte man selbst gebaut haben.
FROM alpine:3.19
RUN apk add --no-cache openssl
COPY erzeuge.sh /usr/local/bin/erzeuge.sh
RUN chmod +x /usr/local/bin/erzeuge.sh

LABEL org.opencontainers.image.title="Nexora PKI" \
      org.opencontainers.image.source="https://github.com/Dschonas04/Nexora" \
      org.opencontainers.image.licenses="BUSL-1.1" \
      org.nexora.gpl-quellen="https://gitlab.alpinelinux.org/alpine/aports"

ENTRYPOINT ["/usr/local/bin/erzeuge.sh"]
