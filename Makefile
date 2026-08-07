# SPDX-License-Identifier: Apache-2.0
SHELL := /bin/bash
BIN := $(CURDIR)/bin
GOLANGCI_LINT_VERSION := v2.12.2
COVERAGE_THRESHOLD := 80

.PHONY: verify fmt-check vet lint build test covergate buf-lint contracts \
        web-verify generate up down demo demo-live migrate tools help

## verify: everything the Definition of Done requires. Must exit 0.
verify: fmt-check vet lint build test covergate buf-lint contracts web-verify
	@echo "verify: OK"

fmt-check:
	@files=$$(gofmt -l . 2>/dev/null | grep -v -E '^(bin|web|reference)/' || true); \
	if [ -n "$$files" ]; then echo "gofmt needed on:"; echo "$$files"; exit 1; fi
	@echo "fmt: OK"

vet:
	go vet ./...

lint: $(BIN)/golangci-lint
	$(BIN)/golangci-lint run

$(BIN)/golangci-lint:
	@mkdir -p $(BIN)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
		| sh -s -- -b $(BIN) $(GOLANGCI_LINT_VERSION)

build:
	go build ./...

# -coverpkg=./internal/... makes untested internal packages count against the
# number (honest denominator); covergate strips generated code only.
test:
	go test ./... -coverprofile=coverage.out -covermode=atomic -coverpkg=./internal/...

covergate:
	go run ./scripts/covergate -profile coverage.out -threshold $(COVERAGE_THRESHOLD)

buf-lint:
	@if [ -d proto ]; then go tool buf lint; else echo "buf-lint: no proto/ yet, skipping"; fi

contracts:
	./scripts/contract-checks.sh

web-verify:
	@if [ -f web/vite.config.ts ]; then \
		cd web && npm ci --no-audit --no-fund && npx tsc --noEmit && npm run build; \
	else echo "web-verify: no web/ yet, skipping"; fi

## generate: regenerate protobuf (Go + TS) and sqlc code. Output is committed.
generate:
	go tool buf generate
	go tool sqlc generate

# Host-side targets read .env the same way docker compose does, so
# `cp .env.example .env` is the only setup step a reviewer needs.
define with_env
	set -a; if [ -f .env ]; then . ./.env; fi; set +a;
endef

migrate:
	@$(with_env) go run ./cmd/migrate up

up:
	docker compose up --build -d --wait
	@echo "All services up. Public API: http://localhost:8080  Dashboard: http://localhost:8081"

down:
	docker compose down -v

## demo: deterministic pipeline demo from committed fixtures (no network).
demo:
	@$(with_env) go run ./cmd/demo

## demo-live: same demo but ingesting live Open-Meteo data (free, no key).
demo-live:
	@$(with_env) CLIMATE_SOURCE=openmeteo go run ./cmd/demo

help:
	@grep -E '^##' Makefile | sed 's/^## //'
