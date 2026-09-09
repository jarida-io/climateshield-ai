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
| Dashboard | Nine views, each backed by a live endpoint, each carrying an on-screen statement of what it does NOT prove. A status strip under the nav puts the weather source, the channel ("mock — no SMS is sent"), the active scorer and "demo population: fictional" on EVERY view. Filter/preview forms throughout; hand-built SVG charts (no charting dependency). Types generated from protobuf. |
| Climatology predictor | Real. Empirical per-county, per-month distributions from 18,195 historical 14-day windows (3,780 stored quantiles), embedded in the binary. Reports exceedance of the CLIMATE driver, never an outbreak probability. |
| Annotated scores | Real. A default `PREDICTOR=rules` deployment now records an exceedance annotation and one explaining sentence on every score. The published thresholds still decide every tier; the annotation is measured after the fact and moved nothing. The bare `RulesPredictor` is byte-identical and its boundary tests are untouched. |
| Chain anchor | Real, against a LOCAL DEVELOPMENT CHAIN. Each day's Merkle root goes to a `RootAnchor` contract on the anvil node this stack starts (chain id 31337) and is read back with `eth_call` before the day is reported anchored; a mismatch is an error. `GET /v1/ledger/anchors/verify` re-runs the check live. Nothing is written to any public network. |
| Briefing service | Real. One plain-language briefing per county per language (en, sw) built from an aggregate fact sheet. The DEFAULT generator is a deterministic template that labels itself. Model drafts pass a grounding check or are rejected. |
| Threshold validation | Real, and the most useful thing in here. Two of four published cutoffs are unreachable in the monitored counties; see docs/threshold-validation.md. Encoded as a failing-if-untrue test and exposed on /v1/model. |
| Pipeline | Real. River-backed ingest → predict → alert, plus a ledger sweep and a briefing sweep, exercised end to end by an integration test that boots all seven services. |

`make verify`, `make up` and `make demo` all pass as of the last commit on this
branch. The demo output in the README is copied verbatim from an actual run —
quiet-hours note, zero alert count and all — not retyped or tidied.

## What is stubbed, and exactly where

| Stub | Location | Behaviour |
|---|---|---|
| ONNX predictor | `internal/predict/onnx.go` | `NewONNXPredictor` returns `ErrNotImplemented`. Setting `ONNX_MODEL_PATH` **fails startup** rather than silently using rules — a configured model that cannot load must never be mistaken for one that works. |
| CHIRPS source | `internal/climate/chirps/chirps.go` | `TODO(Q1)`; constructor returns `ErrNotImplemented`. |
| ERA5 source | `internal/climate/era5/era5.go` | `TODO(Q1)`; constructor returns `ErrNotImplemented`. |
| Africa's Talking | `internal/notify/at/at.go` | Returns `ErrNotConfigured`. No account, no credentials, by design. |
| SMPP channel | `internal/notify/smpp/smpp.go` | Wired against `fiorix/go-smpp` and it compiles and binds lazily, but it has **never been tested against a live carrier**. Treat it as unproven. |
| Public-chain anchor | `internal/ledger/anchor/evm/` | The anchor itself is real, but only against a **local development chain**: `RootAnchor.sol` (compiled once, artifacts committed and hash-checked) receives each day's root via a hand-rolled JSON-RPC client and the root is read back before it is reported. `make up` starts anvil for it; its history is deleted by `make down -v`. Anchoring to a **public** network is deliberately not wired — it needs a funded signing key and the zero-credential rule forbids one. Nothing in this repo writes to any public chain, and no surface may call this chain public, immutable or decentralised. |
| Language-model briefings | `internal/briefing/openaicompat/`, `internal/briefing/anthropic/` | Both adapters are written and covered by committed golden response shapes served by `httptest`. **Neither has ever produced a briefing on this machine.** `make up-ai` (Ollama + qwen2.5:1.5b) has never been run here — it needs a multi-GB image and model pull — and no live Anthropic API call has ever been made from this repository, deliberately: tests may not touch the network and the repo ships no key. The compose graph for the `ai` profile was validated with `docker compose config` and the image digest resolved with `docker manifest inspect`, and that is the whole of what is known. |
| Kiswahili wording | `internal/briefing/mock/`, `internal/notify/` templates | The Kiswahili text is the implementer's, not a Kiswahili speaker's. The grounding check catches invented facts; it does not judge grammar. Every surface says so and keeps saying so until a named reviewer signs it off. |

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

**Total: 90.6% (2819/3113 statements). The gate is 80% and is GREEN.**
Generated code (`internal/gen`, `internal/store/db`) is excluded; nothing else
is. Reproduce with:

```sh
make test
go run ./scripts/covergate -profile coverage.out -threshold 80
```

This is up from a red 66.8% one branch ago. It was closed by writing tests, not
by moving the threshold or adding an exclusion — which was the stated policy
here when the number was embarrassing, and the policy did not change once it
stopped being embarrassing.

The per-package figures below come from the same `coverage.out` the gate reads,
merged block-by-block the way `covergate` merges it and then summed per
directory:

```sh
awk '!/^mode:/ && !/\/internal\/(gen|store\/db)\// {
       if (!($1 in best) || $3 > best[$1]) best[$1] = $3; st[$1] = $2 }
     END { for (b in st) { p = b; sub(/:[0-9.,]+$/, "", p); sub(/\/[^\/]*$/, "", p)
             t[p] += st[b]; if (best[b] > 0) c[p] += st[b] }
           for (p in t) printf "%.1f\t%s\n", 100*c[p]/t[p], p }' coverage.out | sort -k2
```

Because `make test` runs with `-coverpkg=./internal/...`, every test binary
reports every instrumented block, so a package's number includes coverage
contributed by other packages' tests — the integration test in particular. That
is the honest answer to "is this code exercised anywhere in the suite", and it
is higher than what `go test ./internal/foo` alone would print for the same
package. Where the two disagree, say which one you mean.

| Package | Coverage | |
|---|---|---|
| `internal/briefing` | 95.6% | |
| `internal/briefing/anthropic` | 97.0% | never called against the live API |
| `internal/briefing/facts` | 96.7% | |
| `internal/briefing/facts/factstest` | 100.0% | test harness |
| `internal/briefing/mock` | 100.0% | the deterministic template |
| `internal/briefing/openaicompat` | 92.1% | never called against a live model |
| `internal/climate` | 95.8% | |
| `internal/climate/chirps` | 100.0% | stub |
| `internal/climate/era5` | 100.0% | stub |
| `internal/climate/fixture` | 92.3% | |
| `internal/climate/ingestor` | 78.9% | service bootstrap |
| `internal/climate/openmeteo` | 86.4% | |
| `internal/jobs` | 100.0% | |
| `internal/ledger` | 85.8% | |
| `internal/ledger/anchor` | 100.0% | |
| `internal/ledger/anchor/evm` | 95.5% | |
| `internal/ledger/anchor/evm/evmtest` | 84.9% | test harness |
| `internal/notify` | 97.5% | |
| `internal/notify/at` | 100.0% | stub |
| `internal/notify/mock` | 74.1% | |
| `internal/notify/notifier` | 79.9% | |
| `internal/notify/smpp` | 58.8% | **weakest**, and untested against a carrier |
| `internal/platform/clock` | 100.0% | |
| `internal/platform/config` | 75.0% | |
| `internal/platform/crypto` | 83.3% | |
| `internal/platform/httpx` | 85.7% | |
| `internal/platform/logging` | 100.0% | |
| `internal/platform/metrics` | 100.0% | |
| `internal/predict` | 92.7% | |
| `internal/publicapi` | 93.0% | |
| `internal/registry` | 88.2% | |
| `internal/store` | 73.8% | |
| `internal/store/seed` | 84.8% | |
| `internal/store/testdb` | 68.5% | test harness |

The number still flatters pure logic over error paths. `internal/notify/smpp`
at 58.8% is the one I would fix first, and it is also the package whose real
behaviour a test cannot establish at all (see below).

## Where this is thin — read this part

- **The thing that decides a risk level is still four `if` statements.** A
  fitted baseline now annotates every score with how unusual the weather was,
  and can be promoted to the deciding scorer with `PREDICTOR=climatology` — but
  in the default deployment the published thresholds set every tier and trigger
  every alert, and the annotation moves nothing. That is defensible as a v1
  because the thresholds are published and traceable, but neither scorer is
  machine learning, neither has been validated against outbreak data, and **no
  accuracy claim is made anywhere in this repository**. Do not let a demo imply
  otherwise.
- **The reference climatology has not been rebuilt from the archive here.**
  `make climatology` was never run in this branch, so byte-identical
  regeneration is unproven. What is proven without a network: the generator
  re-emits the committed artifact byte for byte
  (`TestEncoderReproducesTheCommittedArtifactByteForByte`) and its windowing
  reproduces the committed per-county-per-month sample counts. A human should
  run `make climatology` once and compare the digest before the assessment.
- **The artifact's quantile index rule was inferred, not recovered.** The
  repository never contained the original generator, so the rule
  (virtual index `p/100*(n-1)`, rounded half to even — NumPy's
  `method="nearest"`) was reverse-engineered. It is consistent with every value
  in the committed file being an exact order statistic: all 2,520 temperature
  quantiles are exact multiples of 1/140 and all 1,260 rainfall quantiles are
  exactly one decimal. Consistent is not the same as confirmed; only a rebuild
  confirms it.
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
- **The dashboard is nine views and no test runner.** `web/` has no test
  framework at all, so none of the TypeScript is covered by anything; adding
  one is a stack decision that has not been taken. The basemap still needs
  internet, dark mode is still not implemented, and the stale-data banner's
  logic has been reviewed but never exercised against a real `X-Data-Stale`
  response — producing one means stopping the database under the running
  stack, which nobody has done yet.
- **No language model has ever written a briefing here.** The default
  deterministic template is exercised constantly; the two model adapters are
  exercised only against committed golden response shapes. `make up-ai` has
  never been run on this machine and no live Anthropic call has ever been made.
  The grounding check is well tested against adversarial drafts
  (`go test ./internal/briefing -run TestAdversarialDrafts`), but every one of
  those drafts was written by a human pretending to be a model.
- **The Kiswahili has had no native-speaker review.** Both the SMS templates
  and the briefing template are the implementer's Kiswahili. This is the
  cheapest outstanding item on the list and the one most likely to embarrass
  the project in front of the people it is for.
- **Error paths are thinner than happy paths.** Retry, backoff and partial
  failure handling largely rely on River's defaults, which have not been tuned.
- **The SMPP adapter may well not work.** It compiles and fails cleanly against
  a dead SMSC. That is all that is known.
- **Alert selection is naive.** Every child with any due dose in an affected
  county is alerted; there is no prioritisation by risk, distance to a clinic,
  or how overdue they are.
- **Most of the dashboard was verified in an embedded browser, not a real one.**
  Marker placement, colours, data binding, basemap tile rendering, the
  Comfortaa wordmark and the logo were all confirmed against the running stack,
  but only after forcing a viewport size, because the embedded browser reports
  0×0 and will not paint on its own. The History view and its "verify on chain
  now" button are the exception: those were confirmed in a real browser against
  the running stack. Responsive behaviour at real window sizes and on mobile
  remains unverified. Open it in a normal browser before any demo.

## Dashboard forms and charts

Every form on the dashboard **queries or previews**. None writes. The public
tier is read-only, and a write path on an unauthenticated public surface would
breach that, so the controls are filters, selectors and previewers only.

The message previewer accepts free text (a child's first name). It renders
**entirely in the browser** — nothing typed into it is transmitted, logged or
stored. That required a second GSM-7 septet counter in TypeScript, mirroring
`internal/notify/gsm7.go`. The Go one remains authoritative: it is what
actually refuses to render an over-long message before anything reaches a
channel. The TypeScript copy is a transcription of the published GSM 03.38
tables for live feedback. It is a duplication and a drift risk, and is
recorded here as such.

Charts are hand-built SVG rather than a charting library: the forms are simple
and a library would add ~150KB to a page with an uptime obligation. Two real
defects were caught by rendering them and looking, rather than by reasoning:
a stretched coordinate system (`preserveAspectRatio="none"`) was turning every
marker circle into a flat ellipse, and markers at a distribution's extreme were
clipped by the plot edge. Both are fixed by measuring the container and
drawing in real pixel coordinates.

The status palette was also wrong on accessibility grounds: the original
MEDIUM amber (#F4A259) sat at 2.07:1 against white label text, far below the
4.5:1 floor. All three tiers were re-stepped and now clear it. The four-slot
categorical palette used for diseases was checked for colour-vision-deficiency
separation before use; three of its slots fall below 3:1 against the page
surface, so every chart ships a table view and none relies on colour alone.

A visual review pass in a real 1440x900 viewport then found four more defects
that no amount of reasoning had surfaced:

- **Bars rendered past the top of their plot and overlapped the text above.**
  The axis tick ladder stopped one step below the maximum whenever the max
  fell between ticks (143 septets against a top tick of 100), and bars are
  scaled against the top tick. Ticks now always cover the maximum.
- **Only the top axis tick was labelled**, leaving nothing to read
  intermediate values against. Every tick now carries its value.
- **A two- or three-bar chart spread across 1400px** of empty surface. Plot
  width is now capped at ~110px per category.
- **The map cropped Mombasa off the east edge** — it claimed five counties
  while showing four. It now frames its own data with `fitBounds`.

**Basemap resilience.** The dashboard's basemap comes from MapLibre's public
demo tile server. During review it stalled repeatedly in the test browser with
NO error event at all — MapLibre parses tiles in Web Workers, and when those
stall the map reports nothing and simply never loads. The map now (a) frames
the county markers immediately rather than waiting for a style that may never
arrive, and (b) after ten seconds says "Basemap unavailable — county markers
and risk levels below are still accurate" instead of presenting a blank blue
rectangle. This matters beyond the test environment: any restricted or offline
deployment hits the same path.

**Not done:** dark mode. The dashboard is light-only, and the charts have not
been stepped for a dark surface. Responsive behaviour below ~900px wide is
also unverified.

## On calling it a "model"

Be ready for this question, because a good evaluator will ask it.

The climatology predictor has **fitted parameters** — roughly 3,800 empirical
quantiles learned from real reanalysis, per county, per month, per driver — and
it produces a continuous, interpretable number. That is more than four
if-statements. But it is **not** supervised learning: nothing was fitted to
health outcomes, because no outcome data exists here. It predicts nothing about
disease. It answers "how unusual is this weather for this place at this time of
year", precisely and defensibly, and stops there.

The honest sentence is: *"a fitted statistical baseline over a decade of
reanalysis, used to flag climatological extremes; no disease model has been
trained or validated."* Anyone who wants to call that "AI" should be
corrected — including in your own slides.

## On calling it "AI"

There are now two things in this repository that a reader could point at and
say "AI", and only one of them has any claim to the word. Keep them apart.

**The briefing generator can be real generative text.** When a deployment opts
in — `make up-ai` for a local open-weights model, or the Claude API with a key
this repository never ships — a language model writes the county briefing. That
is a language model doing what language models do, and calling it generative AI
is fair. Four things bound it:

- It is **off by default**. The shipped generator is a deterministic template
  whose first line is `[mock] no language model ran — deterministic template.`
  If you see that line, no model ran, and the surface says so rather than
  leaving you to guess.
- It is **grounded**. Every draft is checked against the aggregate fact sheet it
  was given. A number not traceable to the sheet, another county, an unscored
  disease, a disease at the wrong tier, an accuracy or outbreak-prediction
  claim, an "SMS sent" claim in either language, a person- or phone-shaped
  string, or a model writing our own `[mock]` label — each rejects the draft.
  A rejected draft is never served, stored or logged; the template is served in
  its place with the violation kinds published.
- It is **labelled**. Every briefing carries its generator, model and prompt
  version alongside the hash of the fact sheet it was written from.
- It **cannot see a person**. The `FactSheet` type has no field for a child, a
  guardian, a phone number or a sub-k count, and no generated text is ever sent
  to a guardian — SMS comes only from the fixed, length-checked templates.

And the honest caveat, which belongs in the same breath: **no model has ever
written one of these on this machine.** Both adapters are tested against
committed golden response shapes, never a live model. Until someone runs
`make up-ai` and watches it, "the generative pillar works" means "the plumbing
and the refusal path work".

**The risk scorer is not AI, in either mode.** Neither the four published
threshold rules nor the fitted climatology is machine learning; nothing was
trained on health outcomes; nothing predicts a disease. Calling the scorer AI
because there is now a language model elsewhere in the same repository would be
exactly the sort of borrowed credibility this project exists to avoid.

The sentence that covers both: *"a language model can write the county
briefing, and every sentence it writes is checked against the aggregates it was
given; the risk levels themselves come from published threshold rules and a
fitted weather baseline, neither of which is machine learning."*

## What I would do next

Ordered by how much each changes whether the system helps anyone.
[docs/roadmap.md](docs/roadmap.md) says which of these are blocked on somebody
else rather than on effort.

1. **Validate the thresholds, or stop calling them a model.** Pull historical
   county outbreak data and CHIRPS/ERA5 reanalysis, and measure how these four
   rules would actually have performed. That either earns the v1 claim or
   redirects effort to the trained model — and it is the only work here that
   changes whether the system helps anyone.
2. **Close the privacy gap between "tested" and "operable".** Give the ledger
   its own database role with schema-scoped grants, move the PII key behind
   OpenBao, and expose `ForgetChild` through an audited operator endpoint. The
   guarantees are implemented; they are not yet administrable.
3. **Amend the pneumonia and meningitis thresholds.** Two of the four published
   rules are inert. The pneumonia fix is small and valuable — measure cold
   stress on daily minimum temperature, where the highland signal is real — and
   the meningitis rule needs either re-scoping to arid northern counties or
   dropping. This needs a proposal amendment, not a commit.
4. **Prove one real SMS path end to end.** The Channel port is the riskiest
   untested boundary: get an SMPP or Africa's Talking sandbox delivering to one
   handset, with delivery receipts recorded, before anyone plans a pilot.
   Nothing else in the system is worth much if the last hop does not work.
5. **Get the Kiswahili reviewed, and run `make up-ai` once.** Two small jobs
   that convert two "unproven" labels into "verified". A named Kiswahili
   reviewer signs off the SMS and briefing templates; a human runs the ai
   profile and watches a local model write one briefing and, ideally, watches
   the grounding check refuse one. Neither is hard. Both are currently caveats
   in front of an assessor.
