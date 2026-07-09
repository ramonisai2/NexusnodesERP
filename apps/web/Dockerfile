# syntax=docker/dockerfile:1
FROM node:20-bookworm AS build
WORKDIR /web
COPY apps/web/package.json ./
RUN npm install --no-fund --no-audit
COPY apps/web/ ./
# Skip tsc project build in image; Vite bundles production assets.
RUN npx vite build

FROM nginx:1.27-alpine
COPY infra/docker/nginx-web.conf /etc/nginx/conf.d/default.conf
COPY --from=build /web/dist /usr/share/nginx/html
EXPOSE 80
HEALTHCHECK --interval=10s --timeout=3s --retries=10 \
  CMD wget -qO- http://127.0.0.1/ >/dev/null || exit 1
