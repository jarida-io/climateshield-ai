<!-- SPDX-License-Identifier: Apache-2.0 -->

# NOTES — what this skeleton actually is

Written for a reviewer who has to decide whether to trust it. It is deliberately
unflattering. Everything below was verified by running it, not by intention.

## What is genuinely built

| Area | State |
|---|---|
| Climate ingestion | Real. Open-Meteo client (free, keyless) + fixture source behind one `ClimateSource` interface. Idempotent upsert on `(area_id, forecast_date, issued_at)`, proven by test. |
| Risk scoring | Real, deterministic. Four diseases, published thresholds, boundary-tested at/±0.1 on every cutoff. Every score row stamped with predictor name + version. |
| Registry | Real. Children, guardians, KEPI schedule (16 doses), due/overdue computation, Connect API. PII encrypted at rest with AES-256-GCM. |
| Append-only events | Real, enforced in the database. `UPDATE` always rejected; `DELETE` only via a transaction-scoped erasure flag. Tests prove the trigger fires and the flag does not leak past its transaction. |
| Ledger | Real. Canonical serialization (golden-tested), per-child HMAC-SHA256 leaves, RFC 6962 Merkle trees, inclusion proofs verified for every leaf across many tree sizes, single-bit mutation property test, local anchoring. |
| Erasure | Real. `ForgetChild` deletes records, scrubs leaf linkage, destroys the HMAC key; tested that roots still verify structurally afterwards. |
| Notification | Real up to the channel boundary. Bilingual GSM-7 templates, consent gate, quiet hours, per-child-per-score dedup. The mock channel writes JSONL and sends nothing. |
| Public API | Real. REST (JSON/CSV/GeoJSON) + Connect over the same proto messages, k≥10 suppression, last-good-response stale cache, never-500 reads. |
| Dashboard | Real but minimal. One page, five markers, types generated from protobuf. |
| Pipeline | Real. River-backed ingest → predict → alert, plus a ledger sweep, exercised end to end by an integration test that boots all six services. |

`make verify`, `make up` and `make demo` all pass; the demo output in the README
is copied from an actual run.

## What is stubbed, and exactly where

| Stub | Location | Behaviour |
|---|---|---|
| ONNX predictor | `internal/predict/onnx.go` | `NewONNXPredictor` returns `ErrNotImplemented`. Setting `ONNX_MODEL_PATH` **fails startup** rather than silently using rules — a configured model that cannot load must never be mistaken for one that works. |
| CHIRPS source | `internal/climate/chirps/chirps.go` | `TODO(Q1)`; constructor returns `ErrNotImplemented`. |
| ERA5 source | `internal/climate/era5/era5.go` | `TODO(Q1)`; constructor returns `ErrNotImplemented`. |
| Africa's Talking | `internal/notify/at/at.go` | Returns `ErrNotConfigured`. No account, no credentials, by design. |
| SMPP channel | `internal/notify/smpp/smpp.go` | Wired against `fiorix/go-smpp` and it compiles and binds lazily, but it has **never been tested against a live carrier**. Treat it as unproven. |
| Blockchain anchor | `internal/ledger/anchor/` | Only `LocalAnchor` (writes roots to a table) exists. Nothing in this repo calls any chain, and no Solidity is present. |

## Assumptions I made

1. **KEPI grace period of 14 days.** The schedule (dose codes and due ages) is
   standard, but the point at which a due dose becomes *overdue* is my
   assumption, encoded as `vaccine_schedule.overdue_grace_days`. It needs
   confirmation from MoH guidance.
2. **Disease name removed from SMS.** The spec requires risk level, county,
   child first name, vaccine and STOP, and forbids a diagnosis. The prototype's
   template named the disease; a named disease beside a named child on a
   plaintext SMS is diagnosis-adjacent and stigma-prone, so the templates now
   say "outbreak risk". A test asserts no disease name can appear. **This is a
   product decision worth confirming** — it trades specificity for privacy.
3. **Risk tier labels stay in English in the Swahili template** (`HIGH`,
   `MEDIUM`). Translating them was out of scope for two template files, but it
   reads oddly in Swahili and should be fixed with real linguistic review.
4. **One alert per child per risk score**, mentioning the earliest-due vaccine
   only. A child due for four doses gets one message, not four.
5. **`forecast_date` is the Africa/Nairobi calendar date** as returned by
   Open-Meteo; `issued_at` is UTC. The predictor scores the most recently
   *issued* window, which is why live ingestion overrides fixtures.
6. **The demo population is fictional** and sized deliberately so k-anonymity
   has both visible and suppressed counties (Kisumu 12, Eldoret 11, Mombasa 3,
   Nakuru 2, Nairobi 0). Phone numbers use a fake `+2547000001xx` range.
7. **Sub-county granularity is schema-only.** `areas.level` allows
   `subcounty`, but only the five counties are seeded and scored.
8. **Single replica per service.** Each service schedules its own periodic work
   in-process (see below) with River per-period job uniqueness as the guard;
   this has not been load-tested with multiple replicas.
9. **`postgis/postgis:16-3.4` is amd64-only**, so the stack uses the multi-arch
   community build `imresamu/postgis:16-3.4` to work on arm64 development
   machines. That is a different publisher and worth a supply-chain look before
   production.

## Two real bugs the integration test caught

Both would have shipped if I had only written unit tests, and both are the kind
that look fine in review:

1. **River refuses to insert a job kind absent from the inserting client's own
   `Workers` bundle.** The ingestor could never enqueue `risk_predict`; the
   pipeline stopped dead after ingestion. Fixed with insert-only River clients
   for cross-service enqueues.
2. **River elects one leader per database, and only the leader fires periodic
   jobs.** With four services sharing a database, whichever won the election
   silently starved the others — in practice the ledger swept and the ingestor
   never ran. Fixed by giving each service its own in-process schedule
   (`internal/jobs/schedule.go`) with per-period job uniqueness.

A third defect was in my own tooling: the coverage gate summed the repeated
per-binary blocks that `-coverpkg` produces, reporting 7.6% when real coverage
was ~72%. It now merges by block, keeping the highest hit count, the way
`go tool cover` does.

## Corrected from the prototype

The Python prototype printed `"SMS sent to under-vaccinated families"` and
`"N alert(s) dispatched via Africa's Talking SMS gateway"` while sending
nothing at all. That class of false output is now structurally prevented: the
mock channel records status `would_send` (never `sent`), prints
`[mock] would send N alerts`, and the demo reads the climate source back out of
the database rather than trusting its own configuration, so it cannot claim
fixture data while displaying live numbers.

The prototype README also carried stale thresholds (50/30/18/38) that
contradicted both its own code and the proposal. Thresholds now exist in one
place, with boundary tests.

## Coverage, per package

Total: **81.1%** (1110/1369 statements), gate is 80%. Generated code
(`internal/gen`, `internal/store/db`) excluded; nothing else is.

| Package | Coverage | |
|---|---|---|
| `internal/climate` | 95.8% | |
| `internal/climate/chirps` | 100.0% | stub |
| `internal/climate/era5` | 100.0% | stub |
| `internal/climate/fixture` | 92.3% | |
| `internal/climate/ingestor` | 50.0% | **weakest**; service bootstrap |
| `internal/climate/openmeteo` | 86.4% | |
| `internal/jobs` | 100.0% | |
| `internal/ledger` | 78.2% | |
| `internal/ledger/anchor` | 83.3% | |
| `internal/notify` | 97.5% | |
| `internal/notify/at` | 100.0% | stub |
| `internal/notify/mock` | 74.1% | |
| `internal/notify/notifier` | 80.5% | |
| `internal/notify/smpp` | 58.8% | untested against a carrier |
| `internal/platform/clock` | 100.0% | |
| `internal/platform/config` | 75.0% | |
| `internal/platform/crypto` | 82.2% | |
| `internal/platform/httpx` | 85.7% | |
| `internal/platform/logging` | 97.6% | |
| `internal/platform/metrics` | 100.0% | |
| `internal/predict` | 67.2% | rules logic is ~100%; service wiring drags it |
| `internal/publicapi` | 93.5% | |
| `internal/registry` | 88.2% | |
| `internal/store` | 73.8% | |
| `internal/store/seed` | 84.8% | |
| `internal/store/testdb` | 68.5% | test harness |

The number flatters the pure logic and hides thin spots in service bootstrap
and error paths. `internal/climate/ingestor` at 50% is the one I would fix
first.

## Where this is thin — read this part

- **The risk model is four `if` statements.** It is defensible as a v1 because
  the thresholds are published and traceable, but it is not machine learning,
  it has not been validated against outbreak data, and **no accuracy claim is
  made anywhere in this repository**. Do not let a demo imply otherwise.
- **Nothing has run at scale.** Five counties, 28 fictional children, 273
  ledger leaves. No load test, no query plan review, no index tuning beyond the
  obvious. `climate_observations` has no partitioning yet.
- **The ledger's key separation is honest but modest.** Per-child HMAC keys
  live in a separate `sealed` schema that only the ledger's query file may
  reference (grep-enforced), but it is the same database instance and the same
  role. Production needs a separate role with schema-scoped grants, then
  external key management.
- **`ForgetChild` is not wired to an API.** It is a tested library function; no
  endpoint or operator tool calls it.
- **The dashboard is one page with five dots.** No history view, no legend
  beyond three colours, no error retry. The basemap needs internet.
- **Error paths are thinner than happy paths.** Retry, backoff and partial
  failure handling largely rely on River's defaults, which have not been tuned.
- **The SMPP adapter may well not work.** It compiles and fails cleanly against
  a dead SMSC. That is all that is known.
- **Alert selection is naive.** Every child with any due dose in an affected
  county is alerted; there is no prioritisation by risk, distance to a clinic,
  or how overdue they are.
- **Visual verification of the dashboard was partial.** Marker placement,
  colours, and data binding were confirmed against the running stack, but the
  embedded browser used for checking reports a 0×0 viewport and never completes
  a paint, so basemap tile rendering could not be confirmed there. It should be
  opened in a normal browser before any demo.

## The three things I would do next

1. **Validate the thresholds, or stop calling them a model.** Pull historical
   county outbreak data and CHIRPS/ERA5 reanalysis, and measure how these four
   rules would actually have performed. That either earns the v1 claim or
   redirects effort to the trained model — and it is the only work here that
   changes whether the system helps anyone.
2. **Close the privacy gap between "tested" and "operable".** Give the ledger
   its own database role with schema-scoped grants, move the PII key behind
   OpenBao, and expose `ForgetChild` through an audited operator endpoint. The
   guarantees are implemented; they are not yet administrable.
3. **Prove one real SMS path end to end.** The Channel port is the riskiest
   untested boundary: get an SMPP or Africa's Talking sandbox delivering to one
   handset, with delivery receipts recorded, before anyone plans a pilot.
   Nothing else in the system is worth much if the last hop does not work.
