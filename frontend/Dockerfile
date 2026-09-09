# Frontend React servi par nginx.
#
# L'origine de l'API est figee au build : Vite substitue import.meta.env au
# moment de la compilation, elle ne peut donc pas se lire au demarrage du
# conteneur. Changer d'API demande de rebatir l'image.

FROM node:22-alpine AS build
WORKDIR /src

# Les dependances d'abord : leur couche ne se reconstruit que si les
# manifestes changent.
COPY package.json package-lock.json ./
RUN npm ci

COPY tsconfig.json vite.config.ts index.html ./
COPY src ./src

ARG VITE_API_URL=""
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost/ >/dev/null || exit 1
