# syntax=docker/dockerfile:1
#
# One image, one process: the Go server serves the API under /api and the
# built web app for everything else (see internal/adapters/web/static.go)
# — no nginx/supervisord needed. Build from the repo root: `docker build -t <image> .`
#
# The web app built here is the Expo app (mobile/), exported as a plain SPA
# (app.json's web.output: "single" — one index.html, Expo Router handles
# navigation client-side, same as the old Vite/React Router app did) rather
# than the standalone frontend/ (Vite/React) app it replaces. frontend/
# still exists and still runs standalone (`npm run dev`) but is no longer
# what gets deployed — mobile/ has since gained feature parity (accounts,
# categories, recurring items, sharing, bank sync) except for a few things
# not yet ported: CSV import, the reconciliation tab, and
# splitting/commenting on an operation. Don't ship
# this over frontend/ in production until those are either ported or
# confirmed not needed there.
# EXPO_PUBLIC_API_URL=/api makes the exported build call the API on
# whatever origin it's served from — this same server, on `/api` — instead
# of mobile's own native dev fallback (an absolute http://localhost:8000).
FROM node:22-slim AS web-builder
WORKDIR /src
COPY mobile/package.json mobile/package-lock.json ./
# Match the npm major used locally to write package-lock.json: node:22 ships
# npm 10, which misreads nested entries in an npm 11 lockfile and fails `npm ci`.
RUN npm install -g npm@11 && npm ci
COPY mobile/ .
ENV EXPO_PUBLIC_API_URL=/api
RUN npm run build

# --- Python deps build (bank_sync / woob) ----------------------------------------
# Builds a standalone venv from the root pyproject.toml/uv.lock. Kept as its
# own stage so it only rebuilds when Python deps change, not on every Go edit.
FROM ghcr.io/astral-sh/uv:python3.14-bookworm-slim AS python-builder
WORKDIR /app
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev

# --- Backend build --------------------------------------------------------------
FROM golang:1.27-bookworm AS backend-builder
WORKDIR /src

# Build stamp for the /version page (internal/core.CommitHash/Branch/BuildDate)
# — passed as --build-arg by the Makefile's docker-build/build-and-push-docker
# targets, derived from git. Defaults keep a plain `docker build` working.
ARG COMMIT_HASH=unknown
ARG BRANCH=unknown
ARG BUILD_DATE=unknown

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
# Overlay the built frontend onto the embed source directory (see embed.go)
# — go:embed needs these files present at compile time, and it must live
# under this module's source tree, not just be copied into the final image.
RUN rm -f internal/adapters/web/dist/placeholder.txt
COPY --from=web-builder /src/dist/ internal/adapters/web/dist/
RUN CGO_ENABLED=0 go build -ldflags "\
    -X bailleul.dev/budget-server/internal/core.CommitHash=${COMMIT_HASH} \
    -X bailleul.dev/budget-server/internal/core.Branch=${BRANCH} \
    -X bailleul.dev/budget-server/internal/core.BuildDate=${BUILD_DATE}" \
    -o /out/server ./cmd/server

# --- Runtime ----------------------------------------------------------------------
# Migrations are embedded into the binary too (backend/migrations/migrations.go),
# applied automatically on startup.
#
# No longer distroless: the Go server shells out to the bank_sync Python CLI
# (internal/adapters/banksync) to talk to woob, which needs a real Python +
# gnupg (module signature verification) + ca-certificates (HTTPS to
# updates.woob.tech) at runtime, not just at build time.
FROM python:3.14-slim-bookworm
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    gnupg ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=python-builder /app/.venv /app/.venv
COPY bank_sync/ /app/bank_sync/

# woob's own state (downloaded module + GPG keyring) lives under XDG_* on
# /data - mount a volume there so it survives restarts and a sync doesn't
# re-download/re-verify the module every time. The per-user bank session
# itself (cookies/2FA) lives encrypted in Postgres, not here - the CLI is
# invoked as a port (session in on stdin, session out on stdout, see
# adapters/banksync), so there's no on-disk session directory to configure.
ENV PATH="/app/.venv/bin:${PATH}" \
    XDG_CONFIG_HOME="/data/woob/config" \
    XDG_DATA_HOME="/data/woob/data" \
    BANK_SYNC_PYTHON_BIN="/app/.venv/bin/python" \
    BANK_SYNC_PROJECT_DIR="/app"
# The server never needs root: it binds an unprivileged port and only writes
# under /data (woob's state, gpg's keyring). Run as a fixed, unprivileged uid
# so a compromise of the server (or of a bank_sync dependency) doesn't hand
# over the container. The uid is fixed (not just "a non-root user") because a
# Kubernetes securityContext, and the init step that fixes ownership of the
# mounted volume, refer to it by number.
ENV HOME="/data/home" \
    PYTHONDONTWRITEBYTECODE=1
RUN groupadd --system --gid 10001 lagune \
    && useradd --system --uid 10001 --gid 10001 --home-dir /data/home --shell /usr/sbin/nologin lagune \
    && mkdir -p /data/woob/config /data/woob/data /data/home \
    && chown -R 10001:10001 /data

COPY --from=backend-builder /out/server /server

USER 10001:10001
EXPOSE 8000
CMD ["/server"]
