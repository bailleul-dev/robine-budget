SHELL := /bin/bash
.DEFAULT_GOAL := help

.PHONY: help setup env db-up db-down db-logs db-psql db-reset \
	backend-install-tools backend-run backend-build backend-test backend-vet backend-lint \
	backend-migrate backend-migrate-down backend-migration \
	frontend-install frontend-run frontend-build frontend-typecheck \
	mobile-install mobile-run ota-keys ota-publish ota-rollback \
	ios-setup ios-prebuild ios-archive ios-upload ios-beta ios-release \
	ios-screenshots ios-screenshots-upload \
	docker-build docker-run docker-push build-and-push-docker \
	dev test lint clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

## --- First-time setup -------------------------------------------------

env: backend/.env ## Create backend/.env from .env.example if missing

backend/.env:
	cp .env.example backend/.env

setup: env db-up mobile-install ## Full first-time setup (db + backend tooling + mobile)

## --- Database (Postgres via Docker) ------------------------------------

db-up: ## Start Postgres in the background
	docker compose up -d

db-down: ## Stop and remove the Postgres container
	docker compose down

db-logs: ## Tail Postgres logs
	docker compose logs -f postgres

db-psql: ## Open a psql shell on the dev database
	docker compose exec postgres psql -U budget -d budget

db-reset: ## Drop and recreate the dev database, then re-run migrations
	docker compose down -v
	docker compose up -d
	sleep 2
	$(MAKE) backend-migrate

## --- Backend (Go, backend/) ------------------------------------------------
# The Go server reads backend/.env itself (see internal/core/config.go), so
# `make env`/`make setup` is still all you need — no separate env file.

GOBIN := $(shell go env GOPATH)/bin

$(GOBIN)/air:
	go install github.com/air-verse/air@latest

$(GOBIN)/golangci-lint:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0

backend-install-tools: $(GOBIN)/air $(GOBIN)/golangci-lint ## Install air (hot-reload) and golangci-lint if not already on GOPATH/bin

backend-run: backend-install-tools ## Run the Go backend dev server with hot reload (air)
	$(GOBIN)/air

backend-build: ## Build the Go backend binary
	cd backend && go build -ldflags "\
		-X bailleul.dev/budget-server/internal/core.CommitHash=$(shell git rev-parse --short HEAD 2>/dev/null || echo dev) \
		-X bailleul.dev/budget-server/internal/core.Branch=$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo dev) \
		-X bailleul.dev/budget-server/internal/core.BuildDate=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)" \
		-o tmp/server ./cmd/server

backend-test: ## Run Go backend tests
	cd backend && \
	go test $$(go list ./... | grep -v '/migrations$$') -coverprofile=coverage.out -cover && \
	go tool cover -func=coverage.out | grep total

backend-vet: ## Static-analyze the Go backend
	cd backend && go vet ./...

backend-lint: $(GOBIN)/golangci-lint ## Lint the Go backend (golangci-lint, config in backend/.golangci.yml)
	cd backend && $(GOBIN)/golangci-lint run ./...

# golang-migrate wants a plain "postgres://" DSN with an explicit sslmode —
# convert it once here the same way internal/core/config.go does for the
# server itself.
GO_DATABASE_URL := $(shell . ./backend/.env 2>/dev/null; echo "$$DATABASE_URL" | sed -e 's,postgresql+asyncpg://,postgres://,' -e 's,postgresql://,postgres://,')
ifeq (,$(findstring sslmode,$(GO_DATABASE_URL)))
GO_DATABASE_URL := $(GO_DATABASE_URL)?sslmode=disable
endif

$(GOBIN)/migrate:
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

backend-migrate: $(GOBIN)/migrate ## Apply Go backend database migrations
	cd backend && $(GOBIN)/migrate -path migrations -database "$(GO_DATABASE_URL)" up

backend-migrate-down: $(GOBIN)/migrate ## Roll back the last Go backend migration
	cd backend && $(GOBIN)/migrate -path migrations -database "$(GO_DATABASE_URL)" down 1

backend-migration: $(GOBIN)/migrate ## Create a new Go migration pair (usage: make backend-migration name=add_foo)
	cd backend && $(GOBIN)/migrate create -ext sql -dir migrations -seq $(name)

## --- Frontend (React, Vite) [DEPRECATED — superseded by mobile/, kept for reference] ---

frontend-install: ## Install frontend dependencies (deprecated, see mobile-install)
	cd frontend && npm install

frontend-run: ## Run the frontend dev server (deprecated, see mobile-run)
	cd frontend && npm run dev

frontend-build: ## Build the frontend for production (deprecated)
	cd frontend && npm run build

frontend-typecheck: ## Type-check the frontend (deprecated)
	cd frontend && npx tsc --noEmit

## --- Mobile (Expo, mobile/) ------------------------------------------------

mobile-install: ## Install mobile app dependencies
	cd mobile && npm install

mobile-run: ## Run the Expo dev server
	cd mobile && npm run start

## --- Mobile over-the-air updates (self-hosted, never EAS) -----------------
# Served by the Go backend itself (GET /api/updates/manifest, see
# backend/internal/adapters/web/ota_updates_handlers.go) from OTA_UPDATES_DIR
# or an S3 bucket (OTA_S3_*, Garage in production).
#
# Defaults target production as deployed by ../silo01 (roles/lagune: bucket
# lagune_ota_bucket, served through Garage's public S3 endpoint
# garage_s3_domain), so with the publisher key exported it's just:
#
#   export OTA_S3_ACCESS_KEY_ID=GK… OTA_S3_SECRET_ACCESS_KEY=…
#   make ota-publish
#   make ota-rollback
#
# (read the key on the host with `k3s kubectl -n garage exec garage-0 --
# /garage key info --show-secret lagune-ota-publisher`). Override any of the
# three below for another target, e.g. OTA_DEST=/tmp/ota for a local test.
#
# OTA_DEST is where the server reads from: an s3://bucket[/prefix], a local
# path, or an rsync/ssh target. Only JS/asset changes can ship this way —
# anything touching native code needs a version bump in mobile/app.json and
# a new store build.

OTA_API_URL ?= https://lagune.bailleul.dev/api
OTA_DEST ?= s3://lagune-ota
OTA_S3_ENDPOINT ?= https://s3.bailleul.dev

ota-keys: ## Generate the OTA code signing key pair (once; keep mobile/.ota-keys/private-key.pem secret)
	@test ! -e mobile/.ota-keys/private-key.pem || (echo "mobile/.ota-keys/private-key.pem already exists — refusing to overwrite it (every shipped build trusts it)"; exit 1)
	cd mobile && npx expo-updates codesigning:generate \
		--key-output-directory .ota-keys \
		--certificate-output-directory code-signing \
		--certificate-validity-duration-years 10 \
		--certificate-common-name "Lagune"
	@echo "→ Commit mobile/code-signing/certificate.pem. Copy mobile/.ota-keys/private-key.pem to the server and set OTA_CODE_SIGNING_KEY_PATH; back it up somewhere safe."

ota-publish: ## Publish the current JS as an OTA update to production (needs OTA_S3_ACCESS_KEY_ID/SECRET)
	cd mobile && OTA_API_URL="$(OTA_API_URL)" OTA_DEST="$(OTA_DEST)" OTA_S3_ENDPOINT="$(OTA_S3_ENDPOINT)" node scripts/ota-publish.mjs

ota-rollback: ## Roll installed apps back to their embedded JS (needs OTA_S3_ACCESS_KEY_ID/SECRET)
	cd mobile && OTA_DEST="$(OTA_DEST)" OTA_S3_ENDPOINT="$(OTA_S3_ENDPOINT)" node scripts/ota-rollback.mjs

## --- iOS release (fastlane, local build + upload, never EAS) ------------------
# See mobile/fastlane/Fastfile. Defaults: team 986VZQX8W4, API
# https://lagune.bailleul.dev/api, build number from the UTC clock.
# Override with APPLE_TEAM_ID=… EXPO_PUBLIC_API_URL=… IOS_BUILD_NUMBER=…,
# and ASC_KEY_PATH/ASC_KEY_ID/ASC_ISSUER_ID (or mobile/fastlane/.env) to
# sign and upload with an App Store Connect API key instead of an Apple ID.

FASTLANE := cd mobile && bundle exec fastlane ios

ios-setup: ## Install fastlane (bundle install in mobile/)
	cd mobile && bundle install

ios-prebuild: ## Regenerate mobile/ios/ from app config (expo prebuild --clean)
	$(FASTLANE) prebuild

ios-archive: ## Build a signed Release .ipa of the iOS app
	$(FASTLANE) build

ios-upload: ## Upload the last .ipa to App Store Connect (TestFlight)
	$(FASTLANE) upload

ios-beta: ## Build + upload a TestFlight beta in one go
	$(FASTLANE) beta

ios-release: ios-beta ## Alias of ios-beta

ios-screenshots: ## Capture App Store screenshots on the simulator (demo data, Maestro)
	$(FASTLANE) screenshots

ios-screenshots-upload: ## Upload mobile/fastlane/screenshots/ to the App Store listing
	$(FASTLANE) upload_screenshots

## --- Docker --------------------------------------------------------------
# One image, one process: the Go server serves the API and the built React
# app together (see internal/adapters/web/static.go) — no nginx needed.

# Override on the command line, e.g.: make build-and-push-docker DOCKER_IMAGE=registry.bailleul.dev/budget:v1.0.0
DOCKER_REGISTRY ?= registry.bailleul.dev
DOCKER_IMAGE ?= $(DOCKER_REGISTRY)/lagune
DOCKER_TAG ?= latest
DOCKER_PLATFORM ?= linux/amd64

# Build stamp baked into the image for the /version page — see the
# Dockerfile's backend-builder stage and internal/core.CommitHash/Branch/BuildDate.
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
DOCKER_BUILD_ARGS := --build-arg COMMIT_HASH=$(GIT_COMMIT) --build-arg BRANCH=$(GIT_BRANCH) --build-arg BUILD_DATE=$(BUILD_DATE)

docker-build: ## Build the combined image (explicitly linux/amd64)
	docker build --platform $(DOCKER_PLATFORM) $(DOCKER_BUILD_ARGS) -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

docker-run: ## Run the built image locally on http://localhost:8080
	docker run --rm -p 8080:8000 --env-file backend/.env \
		-e DATABASE_URL=postgres://budget:budget@host.docker.internal:5432/budget \
		-v budget_woob_data:/data \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

docker-push: docker-build ## Build and push the image to the container registry
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)

build-and-push-docker: ## Build and push the linux/amd64 Docker image to registry.bailleul.dev via buildx
	docker buildx build --platform $(DOCKER_PLATFORM) $(DOCKER_BUILD_ARGS) -t $(DOCKER_IMAGE):$(DOCKER_TAG) --push .


## --- Combined ------------------------------------------------------------

dev: ## Run the Go backend (hot reload via air) and the Expo mobile app, together (Ctrl+C stops all)
	@trap 'kill 0' EXIT INT TERM; \
	$(MAKE) backend-run & \
	$(MAKE) mobile-run & \
	wait

test: backend-test ## Run all tests

lint: backend-lint ## Lint everything

clean: ## Remove local build artifacts and dependency caches
	rm -rf backend/tmp
	rm -rf frontend/node_modules frontend/dist

run-bank-sync-api:
	cd bank_sync && uv run fastapi dev