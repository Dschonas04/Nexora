FROM node:22-alpine AS build
WORKDIR /app
COPY package.json ./
RUN npm install
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
# openssl fuer das selbst unterschriebene Zertifikat nach aussen, gettext fuer
# envsubst: die Vorlage traegt zwei Platzhalter, siehe tls-start.sh.
RUN apk add --no-cache openssl gettext
# Als Vorlage und nicht als fertige Konfiguration: beim Start wird eingesetzt,
# wie der Dienst dahinter anzusprechen ist.
COPY nginx.conf /etc/nginx/vorlage.conf
COPY tls-start.sh /usr/local/bin/tls-start.sh
RUN chmod +x /usr/local/bin/tls-start.sh
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80 443
CMD ["/usr/local/bin/tls-start.sh"]

# Was in diesem Abbild steckt, sagt das Abbild selbst. Poppler liegt hier nicht,
# das ist das Backend; die Alpine-Grundlage bringt aber busybox mit, und das ist
# GPL-2.0. Wer das Abbild weitergibt, verteilt es mit und schuldet dafür den
# zugehörigen Quelltext. Siehe THIRD-PARTY.md im Quelltextbestand.
LABEL org.opencontainers.image.title="Nexora Frontend" \
      org.opencontainers.image.source="https://github.com/Dschonas04/Nexora" \
      org.opencontainers.image.licenses="BUSL-1.1" \
      org.nexora.fremde-bestandteile="Das ausgelieferte Buendel enthaelt MPL-2.0-Code (BlockNote). Quelltext: https://github.com/TypeCellOS/BlockNote" \
      org.nexora.gpl-quellen="https://gitlab.alpinelinux.org/alpine/aports"
