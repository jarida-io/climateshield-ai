<!-- SPDX-License-Identifier: Apache-2.0 -->

# ClimateShield AI

**Climate-responsive early warning for child immunization in Kenya.**

Climate data comes in, a deterministic model scores outbreak risk per county,
guardians of under-vaccinated children are selected for alerting, immunization
events are committed to a tamper-evident ledger, and county-level aggregates
are published on a free, read-only public API.

This repository is a **walking skeleton**: a thin but complete vertical slice.
Every service is real, tested and runnable; no service is feature-complete.
See [NOTES.md](NOTES.md) for a blunt account of what is implemented, what is
stubbed, and what is thin.

- **No SMS is ever sent.** The default messaging channel is a mock that writes
  what it *would* send to a file and says so.
- **No personal data reaches any public surface.** Aggregates only, with k≥10
  suppression on any count derived from people.
- **No credentials required.** `git clone && cp .env.example .env && make up`
  works on a clean machine, offline.

Licensed under [Apache 2.0](LICENSE).

---

## Quick start

```bash
cp .env.example .env
make up
make demo
```

`make up` builds and starts nine containers and waits for every health check.
`make demo` seeds a fictional population, runs the pipeline end to end, and
prints what happened. Then:

- Public API — <http://localhost:8080/v1/risk/current>
- Dashboard — <http://localhost:8081>
- Registry API (internal) — <http://localhost:8082>

Tear everything down with `make down` (this deletes the database volume).

### What `make demo` prints

The demo ingests the committed fixture scenario (a Kisumu long-rains window),
so its output is identical on every machine:

```
--- Outbreak risk (latest per county x disease) ---
scored from observations ingested via: fixture (committed demo scenario, not live weather) [5 counties]
  ⚠ Eldoret  pneumonia   MEDIUM (mean_max_temp_c_14d = 17.2, rules v1.0.0)
  ⚠ Kisumu   cholera     HIGH   (peak_rainfall_mm_14d = 74.0, rules v1.0.0)
  ⚠ Kisumu   malaria     HIGH   (peak_rainfall_mm_14d = 74.0, rules v1.0.0)
  ⚠ Mombasa  cholera     MEDIUM (peak_rainfall_mm_14d = 41.0, rules v1.0.0)
  ⚠ Mombasa  malaria     HIGH   (peak_rainfall_mm_14d = 41.0, rules v1.0.0)
5 elevated (HIGH/MEDIUM) county-disease pairs

--- Alerts ---
  skipped_consent      4
  would_send           35
[mock] would send 35 alerts
(mock channel active: NO SMS was sent; see var/outbox.jsonl for the rendered messages)
```

`make demo-live` runs the same flow against live Open-Meteo forecasts instead.
Real weather means real results: the risk levels you see will differ, and the
output labels the source it actually scored from.

---

## Architecture

```
Open-Meteo ──▶ ingestor ──▶ climate_observations
                              │  (River: risk_predict)
                              ▼
                           predictor ──▶ risk_scores
                              │  (River: alert_dispatch, HIGH/MEDIUM only)
                              ▼
   consent_log ──────────▶ notifier ──▶ Channel ──▶ [mock] outbox.jsonl
   registry (children, KEPI schedule)                smpp (wired, untested)
                              │                      africastalking (stub)
                              ▼
registry ──▶ immunization_events ──▶ ledger ──▶ HMAC leaves ──▶ daily Merkle root ──▶ LocalAnchor

                    risk_scores + aggregate counts
                              ▼
                          publicapi ──▶ JSON / CSV / GeoJSON + Connect ──▶ web dashboard
```

Six services, one Go module, one Postgres database. Jobs move between services
through [River](https://riverqueue.com) (Postgres-backed), so no message broker
is needed.

| Service | Port | Responsibility |
|---|---|---|
| `ingestor` | 8090 | Fetch daily forecasts for 5 counties; idempotent upsert |
| `predictor` | 8091 | Score outbreak risk; enqueue alerts for HIGH/MEDIUM |
| `notifier` | 8092 | Render bilingual SMS, respect consent + quiet hours, dispatch via a Channel |
| `ledger` | 8093 | Commit immunization events to per-day Merkle trees; anchor roots |
| `registry` | 8082 | Children, guardians, KEPI schedule, immunization events (Connect API) |
| `publicapi` | **8080** | Public read-only aggregates (REST + Connect) |
| `web` | **8081** | One-page MapLibre risk map |

Only `publicapi` and `web` are intended to be publicly exposed.

### Risk model (v1, deterministic rules)

| Disease | Driver | HIGH | MEDIUM |
|---|---|---|---|
| Cholera | 14-day peak rainfall | ≥ 60 mm | ≥ 30 mm |
| Malaria | 14-day peak rainfall | ≥ 40 mm | ≥ 20 mm |
| Pneumonia | 14-day mean max temp | ≤ 16 °C | ≤ 19 °C |
| Meningitis | 14-day mean max temp | ≥ 39 °C | ≥ 36 °C |

These thresholds are published in the funding proposal. They live in exactly
one place — [`internal/predict/rules.go`](internal/predict/rules.go) — and every
one has at/just-below/just-above boundary tests. **No accuracy, sensitivity or
specificity claim is made for this ruleset**; it has not been validated against
outbreak surveillance data. A trained model is future work
(`internal/predict/onnx.go` is an interface stub that reports `ErrNotImplemented`).

Every `risk_scores` row records the predictor name and version that produced
it, so scores stay auditable across model changes.

---

## Public API

All endpoints are unauthenticated, read-only, and return aggregates only.

| Endpoint | Description |
|---|---|
| `GET /health` | `200` with `{"status":"ok"}`; `503` if the database is unreachable |
| `GET /metrics` | Prometheus metrics |
| `GET /v1/risk/current` | Latest risk score per county × disease |
| `GET /v1/risk/history` | Historical scores; filters below |
| `GET /v1/stats` | Per-county counts derived from people (k≥10 suppressed) |

`GET /v1/risk/history` accepts `area` (county name), `disease`
(`cholera`\|`malaria`\|`pneumonia`\|`meningitis`), `from` and `to`
(`YYYY-MM-DD`), and `limit` (clamped to 1000).

**Formats.** Add `?format=csv` or `?format=geojson` to any `/v1` endpoint;
JSON is the default. GeoJSON is available on the risk endpoints only (stats
have no geometry). Unsupported combinations return `400`, never `500`.

```bash
curl -s localhost:8080/v1/risk/current | jq .
curl -s "localhost:8080/v1/risk/current?format=geojson" | jq .type   # FeatureCollection
curl -s "localhost:8080/v1/stats?format=csv"
```

The same messages are served over ConnectRPC at
`/climateshield.v1.PublicService/{GetCurrentRisk,GetRiskHistory,GetStats}`.
Protobuf definitions live in [`proto/climateshield/v1`](proto/climateshield/v1);
the dashboard's TypeScript client is generated from them, so no response type
is hand-written twice.

### Availability behaviour

A public read **never returns 500**. Each endpoint caches its last good
response; if the database becomes unreachable the cached body is served with:

```
X-Data-Stale: true
```

On a cold start with a dead database the response is an empty but structurally
valid payload, still `200`, still flagged stale. `/health` reports `503`
independently, so monitoring sees the truth while readers keep getting data.

```bash
docker compose stop postgres
curl -si localhost:8080/v1/risk/current | grep -i x-data-stale   # X-Data-Stale: true
docker compose start postgres
```

### Privacy guarantees

- Responses contain **no** child or guardian identifiers, names, phones, or
  dates of birth, and no per-child hash.
- People-derived counts in `/v1/stats` are suppressed when `0 < n < 10`: the
  value is omitted and a `*_suppressed: true` flag is set (CSV emits an empty
  cell). Zero is reported as zero — absence of a population is not
  individually identifying.

Both properties are enforced by contract tests (below).

---

## Development

```bash
make verify   # fmt · vet · lint · build · test · coverage gate · buf lint · contracts · tsc · web build
make test     # tests only, with coverage profile
make generate # regenerate protobuf (Go + TS) and sqlc code
make migrate  # apply migrations against DATABASE_URL
make help     # list documented targets
```

`make verify` is the gate that must pass before any change lands.

**Toolchain.** Go 1.23+ is the only prerequisite besides Docker and Node.
`buf`, `sqlc`, `protoc-gen-go` and `protoc-gen-connect-go` are pinned as
`go.mod` tool dependencies and run via `go tool`; `golangci-lint` is pinned by
version and installed into `./bin` by the Makefile. Nothing needs a global
install, and every version is locked by `go.sum`.

**Tests.** `go test ./...` runs unit tests plus database-backed tests that
start a throwaway PostGIS container via testcontainers (Docker must be
running). **No test touches the network**: the Open-Meteo client is tested
against a local `httptest` server replaying committed golden JSON. Run
`go test -short ./...` to skip everything that needs Docker.

### Coverage

The gate requires **≥80%** statement coverage over `./internal/...`, enforced
by [`scripts/covergate`](scripts/covergate) in both `make verify` and CI
(Codecov is a paid SaaS and therefore not used). Generated code —
`internal/gen` (protobuf/Connect) and `internal/store/db` (sqlc) — is excluded;
nothing else is. `cmd/` binaries are thin `main` functions that delegate to
`internal/<service>.Run`, and are outside the measured set.

Current: **81.1%** (1110/1369 statements). Per-package figures are in
[NOTES.md](NOTES.md).

### Contract tests — do not delete

Two tests encode commitments rather than behaviour:

| Test | Guarantee |
|---|---|
| `TestContract_PIILeak` | No PII value or forbidden field name appears in any public response, in any format, over REST or Connect |
| `TestContract_KAnonymity` | k≥10 suppression holds for every people-derived count |

CI runs both **by name** and greps for their `RUN` and `PASS` lines, because
`go test -run` exits 0 when a test has been deleted or renamed.
`scripts/contract-checks.sh` additionally fails if either is missing from the
package, and also verifies SPDX headers and that only the ledger's query file
references the `sealed` schema.

### Repository layout

```
cmd/            thin service binaries + migrate + demo
internal/
  climate/      ClimateSource interface, Open-Meteo + fixture sources, ingestor service
  predict/      Predictor interface, deterministic rules, ONNX stub, predictor service
  registry/     children, guardians, KEPI due/overdue, Connect API
  ledger/       canonical serialization, HMAC leaves, Merkle trees, anchors, erasure
  notify/       Channel port, mock/smpp/africastalking adapters, GSM-7 templates
  publicapi/    public REST + Connect, formats, k-anonymity, stale cache
  store/        migrations, sqlc queries, seed data, testcontainers harness
  platform/     config, redacting logger, AES-GCM fields, EAT clock, metrics, HTTP
  integration/  boots all six services against a real database
proto/          protobuf contract (source of truth for API types)
testdata/golden Open-Meteo-shaped fixtures encoding the demo scenario
web/            Vite + React + MapLibre dashboard
reference/      the original Python prototype, kept for provenance only
docs/           architecture diagrams and wireframes
```

---

## Configuration

Copy `.env.example` to `.env`. Every value there is a working development
default; none is a credential.

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | `postgres://climateshield:climateshield@localhost:5432/climateshield?sslmode=disable` | Postgres DSN used by host-side tooling |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | `climateshield` | Local database container credentials |
| `PII_KEY_HEX` | 64-char dev value | AES-256-GCM key for encrypted registry columns. **Generate per deployment:** `openssl rand -hex 32` |
| `CLIMATE_SOURCE` | `fixture` | `fixture` (committed golden JSON, deterministic, offline) or `openmeteo` (live) |
| `OPENMETEO_BASE_URL` | `https://api.open-meteo.com` | Forecast API origin (no key required) |
| `CLIMATE_FIXTURE_DIR` | `testdata/golden` | Where the fixture source reads from |
| `FORECAST_DAYS` | `14` | Forecast window length |
| `INGEST_INTERVAL` | `6h` | How often the ingestor sweeps all counties |
| `NOTIFY_CHANNEL` | `mock` | `mock` (writes JSONL, sends nothing) or `smpp` |
| `MOCK_OUTBOX_PATH` | `var/outbox.jsonl` | Where the mock channel records would-be messages |
| `SMPP_ADDR` / `SMPP_SYSTEM_ID` / `SMPP_PASSWORD` | dummy | SMPP bind settings; only read when `NOTIFY_CHANNEL=smpp` |
| `LEDGER_SWEEP_INTERVAL` | `1h` | How often the ledger commits new events and recomputes roots |
| `ONNX_MODEL_PATH` | *(empty)* | If set, the predictor requires an ONNX model. Not implemented — a non-empty value fails startup rather than silently falling back |
| `PUBLICAPI_ADDR` | `:8080` | Public API listen address |
| `REGISTRY_ADDR` | `:8082` | Registry (internal) listen address |
| `INGESTOR_ADDR` / `PREDICTOR_ADDR` / `NOTIFIER_ADDR` / `LEDGER_ADDR` | `:8090`–`:8093` | Health/metrics listen addresses |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` — JSON to stdout, PII-redacted |

`.env` is gitignored. No secret is committed to this repository.

---

## Operational notes

**Messaging honesty.** With `NOTIFY_CHANNEL=mock` (the default), nothing is
sent anywhere. Alert rows are recorded with status `would_send`, never `sent` —
only a real carrier adapter may write `sent`. Log lines and demo output say
`[mock] would send N alerts`.

**Quiet hours.** No alert is dispatched between 21:00 and 07:00 East Africa
Time (fixed UTC+3; Kenya observes no DST). Jobs landing in that window are
rescheduled to the next 07:00 rather than dropped.

**Consent.** `consent_log` is an append-only event log and the most recent row
per guardian decides. A guardian whose latest action is `OPT_OUT` is skipped,
and the skip is recorded as `skipped_consent`.

**Data protection.** Child names, guardian names, phone numbers and national
IDs are stored only as AES-256-GCM ciphertext (`internal/platform/crypto`), with
the key supplied via `PII_KEY_HEX` and never stored in the database. Logging
goes through a redacting handler that masks phone-shaped strings even when a
caller forgets the typed wrappers.

**Immutability and erasure.** `immunization_events` is append-only, enforced by
a database trigger: `UPDATE` is always rejected, and `DELETE` only inside the
guarded transaction used by right-to-erasure. `ledger.ForgetChild` deletes the
child's records, scrubs the child linkage from ledger leaves, and destroys the
child's HMAC key — after which previously published Merkle roots still verify
structurally, but nothing links those leaves to a person.

**Basemap tiles.** The dashboard fetches its base map style from MapLibre's
public demo tile server. It needs no API key, but it does require internet
access; without it the county markers still render over a blank background.
Everything else in the stack runs fully offline.

---

## Not in scope for this skeleton

USSD, an Android app, ONNX inference, model training, FHIR/DHIS2 integration,
real SMS delivery, blockchain anchoring, authentication, multi-tenancy,
Kubernetes, and an admin UI. See [NOTES.md](NOTES.md) for exactly where each
boundary sits in the code.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). All first-party files carry
`SPDX-License-Identifier: Apache-2.0`; `make verify` fails if one is missing.
