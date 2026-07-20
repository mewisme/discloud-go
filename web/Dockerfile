# syntax=docker/dockerfile:1

FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN corepack enable && corepack use pnpm@latest
RUN --mount=type=cache,id=discloud-pnpm,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

FROM node:22-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN corepack enable && corepack use pnpm@latest
# Rewrites are resolved at build time, so the API origin must be known here.
ARG API_URL=http://api:8080
ENV API_URL=${API_URL} NEXT_TELEMETRY_DISABLED=1
RUN --mount=type=cache,id=discloud-next,target=/app/.next/cache \
    pnpm run build

FROM node:22-alpine AS run
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
COPY --from=build /app/public ./public
USER node
EXPOSE 3000
ENV PORT=3000 HOSTNAME=0.0.0.0
CMD ["node", "server.js"]
