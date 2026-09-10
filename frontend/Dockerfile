FROM node:22-alpine AS build
WORKDIR /app
COPY package.json ./
RUN npm install
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
# openssl for the self-signed certificate facing outwards, gettext for envsubst:
# the template carries two placeholders, see tls-start.sh.
RUN apk add --no-cache openssl gettext
# As a template and not as a finished configuration: how the service behind it
# is to be addressed is substituted at start-up.
COPY nginx.conf /etc/nginx/vorlage.conf
COPY tls-start.sh /usr/local/bin/tls-start.sh
RUN chmod +x /usr/local/bin/tls-start.sh
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80 443
CMD ["/usr/local/bin/tls-start.sh"]

# What is in this image, the image says itself. Poppler does not lie here, that
# is the backend; the Alpine base does bring busybox along, though, and that is
# GPL-2.0. Whoever passes the image on distributes it with it and owes the
# corresponding source for it. See THIRD-PARTY.md in the source tree.
LABEL org.opencontainers.image.title="Nexora Frontend" \
      org.opencontainers.image.source="https://github.com/Dschonas04/Nexora" \
      org.opencontainers.image.licenses="BUSL-1.1" \
      org.nexora.fremde-bestandteile="Das ausgelieferte Buendel enthaelt MPL-2.0-Code (BlockNote). Quelltext: https://github.com/TypeCellOS/BlockNote" \
      org.nexora.gpl-quellen="https://gitlab.alpinelinux.org/alpine/aports"
