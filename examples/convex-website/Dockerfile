FROM node:22-bookworm AS build

WORKDIR /app

RUN corepack enable
ENV PNPM_CONFIG_DANGEROUSLY_ALLOW_ALL_BUILDS=true

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY . .

ARG VITE_CONVEX_URL

ENV VITE_CONVEX_URL=${VITE_CONVEX_URL}

RUN pnpm run build:web

FROM nginx:1.27-alpine AS runtime

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=10s --timeout=3s --retries=6 CMD wget -q --spider http://127.0.0.1/ || exit 1
