# SPDX-License-Identifier: Apache-2.0
SHELL := /bin/bash
BIN := $(CURDIR)/bin
GOLANGCI_LINT_VERSION := v2.12.2
COVERAGE_THRESHOLD := 80

.PHONY: verify fmt-check vet lint build test covergate buf-lint contracts \
        web-verify generate contract climatology climatology-digest \
        up up-ai down demo demo-live demo-ai migrate tools help

AI_COMPOSE := -f docker-compose.yml -f deploy/docker-compose.ai.yml

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

## contract: recompile the RootAnchor Solidity contract with the pinned solc
## image (one-off, developer only). Artifacts and BUILD.txt are committed;
## `make up`, tests and CI never compile Solidity.
contract:
	./scripts/build-contract.sh

## climatology: rebuild the reference climatology from the Open-Meteo archive
## (developer only; free and keyless, but the ONLY outbound request this repo
## makes). Prints the SHA-256 of what it wrote — compare it with
## reference_sha256 on GET /v1/model. Nothing in `make up`, `make demo`, the
## tests or CI runs this.
climatology:
	go run ./cmd/buildclimatology

## climatology-digest: print the SHA-256 of the committed reference
## climatology. Makes no network request.
climatology-digest:
	go run ./cmd/buildclimatology -digest

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

## up-ai: same stack, but briefings are written by a small open-weights model
## (Ollama + qwen2.5:1.5b, Apache-2.0) instead of the deterministic template.
## The FIRST run downloads an image and the model and therefore needs internet;
## later runs are offline. Still zero credentials. Every draft still goes
## through the grounding check, and a refused draft still falls back to the
## labelled template. See deploy/docker-compose.ai.yml.
up-ai:
	docker compose $(AI_COMPOSE) --profile ai up --build -d --wait
	@echo "All services up with the ai profile. Public API: http://localhost:8080  Dashboard: http://localhost:8081"

down:
	docker compose $(AI_COMPOSE) --profile ai down -v

## demo: deterministic pipeline demo from committed fixtures (no network).
demo:
	@$(with_env) go run ./cmd/demo

## demo-live: same demo but ingesting live Open-Meteo data (free, no key).
demo-live:
	@$(with_env) CLIMATE_SOURCE=openmeteo go run ./cmd/demo

## demo-ai: the same demo against a stack started with `make up-ai`. The
## briefing step then prints text written by the local model — or, if the
## grounding check refused it, the template plus the reasons it was refused.
demo-ai: demo
	@echo
	@echo "The briefing above states which generator and model wrote it."
	@echo "Full response, including the fact sheet every number must come from:"
	@echo "  curl -s 'http://localhost:8080/v1/briefings?area=Kisumu&lang=en' | jq ."

help:
	@grep -E '^##' Makefile | sed 's/^## //'
