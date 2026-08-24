FROM node:22-alpine AS build
WORKDIR /app
COPY package.json ./
RUN npm install
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
# openssl für das selbst ausgestellte Zertifikat beim ersten Start.
RUN apk add --no-cache openssl
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY tls-start.sh /usr/local/bin/tls-start.sh
RUN chmod +x /usr/local/bin/tls-start.sh
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80 443
CMD ["/usr/local/bin/tls-start.sh"]
