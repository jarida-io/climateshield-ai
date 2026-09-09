<!-- SPDX-License-Identifier: Apache-2.0 -->

# ClimateShield

**Climate-responsive early warning for child immunization in Kenya.**

Climate data comes in. Outbreak risk is scored per county. Guardians of
under-vaccinated children are selected for alerting. Immunization events are
committed to a tamper-evident ledger. County-level aggregates are published on
a free, unauthenticated, read-only API — and a dashboard lets anyone check
each of those claims against live data rather than taking them on trust.

Built by [Jarida One](https://jarida.io) with support from the UNICEF
Innovation Fund. Licensed under [Apache 2.0](LICENSE).

---

## What this repository is, and is not

It is a **walking skeleton**: a thin but complete vertical slice. Every service
is real, tested and runnable. No service is feature-complete.
[NOTES.md](NOTES.md) is a deliberately unflattering account of what is
implemented, what is stubbed, and what is thin — read it before forming a view.

Three claims this project refuses to make:

- **No SMS is sent.** The default messaging channel is a mock that records what
  it *would* send and says so. Only a real carrier adapter may record `sent`.
- **No accuracy claim.** There is no outbreak surveillance data here, so no
  predictor has been validated against disease outcomes and none reports a
  sensitivity, specificity or accuracy figure anywhere.
- **No public blockchain.** The ledger is tamper-evident by construction —
  Merkle roots over per-child HMAC leaves. With `ANCHOR_MODE=evm` (the compose
  default) each day's root is also written to a small `RootAnchor` contract on
  a **local development chain started by this stack — not a public network** —
  and read back before it is reported. Nothing here writes to any public chain,
  and the dashboard says which kind of chain it is talking to.

And two it does make:

- **No personal data reaches any public surface.** Aggregates only, with k≥10
  suppression on every count derived from people. Enforced by a contract test
  CI runs by name.
- **No credentials required.** `git clone && cp .env.example .env && make up`
  works on a clean machine, offline.

---

## Quick start

```bash
cp .env.example .env
make up
make demo
```

`make up` starts ten containers — nine long-running services (including
`anvil`, the local development chain the ledger anchors to) plus a one-shot
migration that exits 0 — and waits for every health check. The **first** run
compiles seven Go binaries and the dashboard, which takes several minutes on a
cold Docker cache; afterwards it reaches healthy in about a minute.

| Surface | URL |
|---|---|
| Dashboard | <http://localhost:8081> |
| Public API | <http://localhost:8080/v1/risk/current> |
| Registry API (internal) | <http://localhost:8082> |

`make down` tears everything down and deletes the database volume.

### What `make demo` prints

The demo ingests a committed fixture scenario — a Kisumu long-rains window — so
its output is identical on every machine:

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

It then verifies a Merkle inclusion proof for an immunization event it recorded
moments earlier, and reports the source it *actually* scored from — read back
from the database, never assumed from configuration, so the output cannot claim
fixture data while showing live numbers.

`make demo-live` runs the same flow against live Open-Meteo forecasts. Real
weather means real results: the risk levels will differ from the above.

---

## The dashboard

Seven views. Each one exists so a reviewer can **verify** a capability against
running data, and each carries an on-screen statement of what it does *not*
prove.

| View | What it lets you check |
|---|---|
| **Risk map** | Current risk per county, filterable by disease |
| **Model** | Which predictor is scoring, and the reference climatology behind any county and month |
| **Weather** | The exact 14-day forecast window each score was computed from |
| **Messaging** | Message outcomes, both language templates, and a live GSM-7 previewer |
| **History** | Daily Merkle roots and anchors |
| **Automation** | Job history from the queue, with optional auto-refresh |
| **Open data** | Every endpoint, runnable from the page itself |

Charts are hand-built SVG — no charting dependency on a page with an uptime
obligation — and every chart ships a data table so nothing is gated behind
colour. All forms **query or preview**; none writes, because the public tier is
read-only. The one control that accepts free text (a child's first name in the
message previewer) renders entirely in the browser, so nothing typed into it is
transmitted, logged or stored.

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
                          publicapi ──▶ JSON / CSV / GeoJSON + Connect ──▶ dashboard
```

Six services, one Go module, one Postgres database. Work moves between services
through [River](https://riverqueue.com) (Postgres-backed), so no message broker
is required.

| Service | Port | Responsibility |
|---|---|---|
| `ingestor` | 8090 | Fetch daily forecasts for 5 counties; idempotent upsert |
| `predictor` | 8091 | Score outbreak risk; enqueue alerts for HIGH/MEDIUM |
| `notifier` | 8092 | Render bilingual SMS, respect consent and quiet hours, dispatch via a Channel |
| `ledger` | 8093 | Commit immunization events to daily Merkle trees; anchor roots |
| `registry` | 8082 | Children, guardians, KEPI schedule, immunization events (Connect) |
| `publicapi` | **8080** | Public read-only aggregates (REST + Connect) |
| `web` | **8081** | Demonstration dashboard |

Only `publicapi` and `web` are intended to be publicly exposed.

**Stack.** Go 1.23+ · chi · ConnectRPC + Protobuf (buf) · PostgreSQL 16 +
PostGIS · pgx + sqlc · golang-migrate · River · `log/slog` with mandatory PII
redaction · Prometheus · testcontainers · TypeScript strict + React + Vite +
MapLibre GL JS · Docker Compose · GitHub Actions. Every dependency is open
source and free; the system builds, tests and runs with zero credentials.

---

## Risk scoring

Two predictors ship. `PREDICTOR=rules` is the default.

### `rules` — the published thresholds

| Disease | Driver | HIGH | MEDIUM |
|---|---|---|---|
| Cholera | 14-day peak rainfall | ≥ 60 mm | ≥ 30 mm |
| Malaria | 14-day peak rainfall | ≥ 40 mm | ≥ 20 mm |
| Pneumonia | 14-day mean max temp | ≤ 16 °C | ≤ 19 °C |
| Meningitis | 14-day mean max temp | ≥ 39 °C | ≥ 36 °C |

These come from the funding proposal. They live in exactly one place —
[`internal/predict/rules.go`](internal/predict/rules.go) — and every cutoff has
at / just-below / just-above boundary tests.

> ### ⚠ Two of these four cutoffs cannot fire
>
> Validated against ten years of ERA5 reanalysis (2015–2024, ~18,200 fourteen-day
> windows across the five counties): the **coldest single-day maximum on record
> is 16.3 °C**, so the pneumonia rule — which needs a 14-day *mean* ≤ 16 °C — can
> never trigger. The **hottest is 36.2 °C** against a meningitis cutoff of 39 °C.
> As absolute cutoffs, both are inert.
>
> The meningitis figure appears calibrated for the Sahel meningitis belt rather
> than the Kenyan highlands and coast. The pneumonia rule measures the wrong
> variable: cold stress shows up in daily *minimum* temperature, where Eldoret,
> Nairobi and Nakuru sit at 10–12 °C.
>
> **The thresholds are unchanged in this repository** — they are contractual, and
> amending them is a proposal decision, not a commit. The finding is reported
> instead: see [docs/threshold-validation.md](docs/threshold-validation.md), the
> `Reachable?` column on `GET /v1/model`, and a test that fails if it ever stops
> being true.

### `climatology` — per-county seasonal percentiles

Scores each forecast window against that county's own distribution for that
calendar month, measured from the same reference decade and embedded in the
binary as a 63 KB artifact.

It reports an **exceedance probability of the climate driver** — "this window is
in the most extreme 2% of the last decade for this county and month". That is a
property of the *weather*. It is **not** a probability that an outbreak will
occur, and this system cannot estimate one. Where no reference distribution
exists it reports "not scored" rather than a confident LOW.

A percentile is defined in every climate, which is precisely why it still works
for the two diseases whose absolute cutoffs do not. For cold stress it uses
daily minimum temperature. Every `risk_scores` row records the predictor name
and version that produced it, so scores stay auditable across model changes.

---

## Public API

Unauthenticated, read-only, aggregates only.

| Endpoint | Returns |
|---|---|
| `GET /health` | `200` when ready; `503` if the database is unreachable |
| `GET /metrics` | Prometheus metrics |
| `GET /v1/risk/current` | Latest risk per county × disease |
| `GET /v1/risk/history` | Historical scores; filters below |
| `GET /v1/stats` | Per-county counts derived from people (k≥10 suppressed) |
| `GET /v1/model` | Active predictor, published thresholds, reachability, reference record |
| `GET /v1/climatology` | Reference distribution for one county and month (`?area=&month=`) |
| `GET /v1/climate/series` | The forecast window each score was computed from |
| `GET /v1/ledger/summary` | Daily Merkle roots and anchors — never individual leaves |
| `GET /v1/alerts/summary` | Messaging outcomes, channel status, rendered templates |
| `GET /v1/pipeline` | Job history and data volumes |

`GET /v1/risk/history` accepts `area`, `disease`, `from`, `to` (`YYYY-MM-DD`)
and `limit` (clamped to 1000).

**Formats.** Add `?format=csv` or `?format=geojson` to the risk endpoints
(`csv` also on `/v1/stats`); JSON is the default. Unsupported combinations
return `400`, never `500`.

```bash
curl -s localhost:8080/v1/risk/current | jq .
curl -s "localhost:8080/v1/risk/current?format=geojson" | jq .type   # FeatureCollection
curl -s localhost:8080/v1/model | jq '.rules[] | {disease, reachableInReferencePeriod}'
```

The same messages are served over ConnectRPC at
`/climateshield.v1.PublicService/…`. Protobuf definitions live in
[`proto/climateshield/v1`](proto/climateshield/v1) and are the single source of
truth: the Go services and the dashboard's TypeScript client are both generated
from them, so no response type is written twice.

### Availability

A public read **never returns 500**. Each endpoint caches its last good
response; if the database becomes unreachable that body is served with
`X-Data-Stale: true`. On a cold start with a dead database the response is an
empty but structurally valid payload — still `200`, still flagged. `/health`
reports `503` independently, so monitoring sees the truth while readers keep
getting data.

```bash
docker compose stop postgres
curl -si localhost:8080/v1/risk/current | grep -i x-data-stale   # X-Data-Stale: true
docker compose start postgres
```

### Privacy

- No child or guardian identifier, name, phone, date of birth — and **no
  per-child hash** — appears in any response. Ledger leaves are per-child HMACs
  and are never published; only whole-day roots are.
- Counts derived from people are withheld when `0 < n < 10`, with a
  `*_suppressed` flag. Zero and ≥10 pass through.

Both properties are enforced by `TestContract_PIILeak` and
`TestContract_KAnonymity`, which CI runs **by name** and greps for their `RUN`
and `PASS` lines — because `go test -run` exits 0 when a test has been deleted.

---

## Development

```bash
make verify   # fmt · vet · lint · build · test · coverage gate · buf lint · contracts · tsc · web build
make test     # tests only, with a coverage profile
make generate # regenerate protobuf (Go + TS) and sqlc output
make migrate  # apply migrations against DATABASE_URL
make help     # list documented targets
```

**Toolchain.** Go 1.23+, Docker, and Node 20+ (dashboard only). `buf`, `sqlc`
and the protobuf plugins are pinned as `go.mod` tool dependencies and run via
`go tool`; `golangci-lint` is version-pinned into `./bin` by the Makefile.
Nothing needs a global install and every version is locked by `go.sum`.

**Tests.** Unit tests plus database-backed tests using a throwaway PostGIS
container (testcontainers; Docker must be running). **No test touches the
network** — the Open-Meteo client is tested against a local `httptest` server
replaying committed golden JSON. `go test -short ./...` skips everything
needing Docker.

> Run `go test ./... -p 2`. Running every package's containers concurrently
> saturates Docker and the six-service integration test fails to connect.

### Coverage

The gate requires **≥80%** statement coverage over `./internal/...`, enforced by
[`scripts/covergate`](scripts/covergate) in `make verify` and CI. Generated code
(`internal/gen`, `internal/store/db`) is excluded; nothing else is.

> **Current: 66.8% — the gate is RED.** The climatology model, the evidence API
> and the dashboard added roughly 790 statements faster than tests for them. The
> gap is concentrated in service bootstrap paths. This is tracked, and the fix is
> to write the tests rather than move the threshold. See NOTES.md.

### Repository layout

```
cmd/            thin service binaries + migrate + demo
internal/
  climate/      ClimateSource interface, Open-Meteo + fixture sources, ingestor
  predict/      Predictor interface, rules, climatology model + reference data
  registry/     children, guardians, KEPI due/overdue, Connect API
  ledger/       canonical serialization, HMAC leaves, Merkle trees, erasure
  notify/       Channel port, mock/smpp/africastalking adapters, GSM-7 templates
  publicapi/    REST + Connect, formats, k-anonymity, stale cache, evidence views
  store/        migrations, sqlc queries, seed data, testcontainers harness
  platform/     config, redacting logger, AES-GCM fields, EAT clock, metrics
  integration/  boots all six services against a real database
proto/          protobuf contract — the source of truth for API types
testdata/golden Open-Meteo-shaped fixtures encoding the demo scenario
web/            Vite + React + MapLibre dashboard
reference/      the original Python prototype, kept for provenance only
docs/           threshold validation, architecture diagrams, wireframes
```

---

## Configuration

Copy `.env.example` to `.env`. Every value there is a working development
default; none is a credential. `.env` is gitignored and no secret is committed.

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | local Postgres DSN | Used by host-side tooling |
| `POSTGRES_USER` / `_PASSWORD` / `_DB` | `climateshield` | Local container credentials |
| `PII_KEY_HEX` | 64-char dev value | AES-256-GCM key for encrypted columns. **Generate per deployment:** `openssl rand -hex 32` |
| `PREDICTOR` | `rules` | `rules` or `climatology` |
| `CLIMATE_SOURCE` | `fixture` | `fixture` (deterministic, offline) or `openmeteo` (live) |
| `OPENMETEO_BASE_URL` | `https://api.open-meteo.com` | Forecast API origin (no key) |
| `CLIMATE_FIXTURE_DIR` | `testdata/golden` | Where the fixture source reads from |
| `FORECAST_DAYS` | `14` | Forecast window length |
| `INGEST_INTERVAL` | `6h` | How often the ingestor sweeps all counties |
| `NOTIFY_CHANNEL` | `mock` | `mock` (sends nothing) or `smpp` |
| `MOCK_OUTBOX_PATH` | `var/outbox.jsonl` | Where the mock channel records would-be messages |
| `SMPP_ADDR` / `_SYSTEM_ID` / `_PASSWORD` | dummy | Only read when `NOTIFY_CHANNEL=smpp` |
| `LEDGER_SWEEP_INTERVAL` | `1h` | How often the ledger commits and recomputes roots |
| `ONNX_MODEL_PATH` | *(empty)* | Not implemented; a non-empty value **fails startup** rather than silently falling back |
| `PUBLICAPI_ADDR` | `:8080` | Public API listen address |
| `REGISTRY_ADDR` | `:8082` | Registry listen address |
| `INGESTOR_/PREDICTOR_/NOTIFIER_/LEDGER_ADDR` | `:8090`–`:8093` | Health and metrics addresses |
| `LOG_LEVEL` | `info` | JSON to stdout, PII-redacted |

---

## Operational notes

**Messaging honesty.** With `NOTIFY_CHANNEL=mock` nothing is sent anywhere.
Alerts are recorded as `would_send`, never `sent`. Output says
`[mock] would send N alerts`.

**Quiet hours.** No alert is dispatched between 21:00 and 07:00 East Africa
Time (fixed UTC+3; Kenya observes no DST). Jobs landing in the window are
rescheduled to the next 07:00 rather than dropped.

**Consent.** `consent_log` is append-only and the most recent row per guardian
decides. A guardian whose latest action is `OPT_OUT` is skipped, and the skip is
recorded as `skipped_consent`.

**Data protection.** Child names, guardian names, phone numbers and national
IDs are stored only as AES-256-GCM ciphertext, with the key supplied via
`PII_KEY_HEX` and never stored in the database. Logging goes through a redacting
handler that masks phone-shaped strings even when a caller forgets the typed
wrappers.

**Immutability and erasure.** `immunization_events` is append-only, enforced by
a database trigger: `UPDATE` is always rejected, `DELETE` only inside the
guarded transaction used for right-to-erasure. `ledger.ForgetChild` deletes the
child's records, scrubs the child linkage from ledger leaves and destroys the
child's HMAC key — after which previously published roots still verify, but
nothing links those leaves to a person.

**Basemap.** The dashboard fetches its base map from MapLibre's public demo tile
server. It needs no API key but does require internet access; without it the map
says so plainly and the county markers and risk levels remain accurate.
Everything else runs fully offline.

---

## Not in scope

USSD, an Android app, ONNX inference, model training, FHIR/DHIS2 integration,
real SMS delivery, anchoring to a public chain (only the local development
chain is anchored to), authentication, multi-tenancy, Kubernetes, and an admin
UI. [NOTES.md](NOTES.md) says exactly where each
boundary sits in the code.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). All first-party files carry
`SPDX-License-Identifier: Apache-2.0`; `make verify` fails if one is missing.
For anything with security or privacy implications — especially a suspected data
leak on a public surface — email **hello@jarida.io** rather than opening a
public issue.
