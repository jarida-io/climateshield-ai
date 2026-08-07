# CLAUDE.md — ClimateShield build rules

Re-read this file whenever you lose context. It encodes a signed funding
agreement, not preferences.

## What this is

Go walking skeleton for a climate-health early warning system (Kenya, UNICEF
Innovation Fund). Thin vertical slice: climate data → risk prediction → alert
via **mock** channel → hash-chained immunization ledger → public read-only API.
Publicly audited; demoed live. Optimize for correctness, type safety, test
coverage, one-command run — never feature count.

## Stack (fixed — do not substitute)

Go 1.24+ single module, `cmd/` binaries · chi + net/http · ConnectRPC +
Protobuf + buf · PostgreSQL 16 + PostGIS (`imresamu/postgis:16-3.4`, multi-arch;
official `postgis/postgis` is amd64-only) · pgx/v5 + sqlc (no ORM) ·
golang-migrate (plain SQL, embedded, run via `cmd/migrate`) · River jobs ·
caarlos0/env config · log/slog JSON through the PII **redaction helper** ·
prometheus/client_golang · testing + testify/require + testcontainers-go ·
web: TypeScript strict + React + Vite + MapLibre GL JS, types generated from
proto (protoc-gen-es) — no hand-written response interfaces.

Tools are pinned: buf/sqlc/protoc-gen-* via `go.mod` `tool` directives
(`go tool <name>`); golangci-lint v2.12.2 into `./bin` via Makefile.

## FORBIDDEN dependencies (license/funding breaches — stop and ask)

TimescaleDB · Redis 7.4+ (use Valkey) · Mapbox GL v2+ (use MapLibre) ·
Terraform/Vault (OpenTofu/OpenBao) · Redpanda · Arbitrum Nitro · RapidPro ·
W&B server · Codecov · hardcoded Infura/Alchemy · Fiber · GORM.
Every dependency must be open source and free; build/test/run needs **zero
credentials**.

## Hard rules

1. **No false claims in output — non-negotiable.** The old prototype printed
   "SMS sent…" while sending nothing. If the mock channel is active, output
   says `[mock] would send N alerts`. No output may imply an action not taken.
   No fabricated benchmarks/accuracy numbers anywhere, including README.
2. **No PII on any public surface.** No child data, no per-child hashes, on
   the public API or any public ledger. Aggregates only, k≥10 suppression on
   people-derived counts.
3. **Never log PII.** All logging through `internal/platform/logging` redaction.
4. **Coverage gate: ≥80%** across `./internal/...` excluding generated code
   (`internal/gen`, `internal/store/db`) — enforced by `scripts/covergate.go`
   in `make verify` and CI. Exclusions are policy, documented in README.
5. **No test may touch the network.** Fixtures + httptest + testcontainers only.
6. **Apache-2.0** with SPDX header (`SPDX-License-Identifier: Apache-2.0`) on
   every first-party .go/.ts/.tsx/.proto/.sql file (`scripts/contract-checks.sh`).
7. **Contract tests — do not delete or rename:**
   `TestContract_PIILeak` (internal/publicapi/pii_contract_test.go) and the
   k-anonymity test. CI runs them by name and fails if absent.
8. **Append-only:** `immunization_events` has a DB trigger blocking
   UPDATE/DELETE. UPDATE is never allowed; DELETE only via the guarded
   erasure path (`store.WithErasure`, used by ForgetChild). Never add other
   mutating queries.
9. **`sealed.child_keys`** may be referenced only from
   `internal/store/queries/ledger.sql` (grep-enforced).
10. Risk thresholds live in `internal/predict/rules.go` ONLY (cholera ≥60/≥30mm,
    malaria ≥40/≥20mm, pneumonia ≤16/≤19°C, meningitis ≥39/≥36°C). Published in
    the funding proposal — do not invent new ones.

## Conventions

- Conventional commits, one commit per service, message explains *why*.
- Test-first on pure logic (thresholds, Merkle, canonical serialization,
  GSM-7 templates, suppression).
- Thin `cmd/*/main.go` (~15 lines) delegating to `internal/<pkg>/service.Run`.
- `make verify` must pass before claiming done. Acceptance:
  `cp .env.example .env && make up && make demo` with zero credentials.
