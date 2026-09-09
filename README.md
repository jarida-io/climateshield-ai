<!-- SPDX-License-Identifier: Apache-2.0 -->

# ClimateShield

**Climate-responsive early warning for child immunization in Kenya.**

Climate data comes in. Outbreak risk is scored per county against the published
thresholds. Guardians of under-vaccinated children are selected for alerting. A
plain-language county briefing is written from the aggregates, in English and
Kiswahili. Immunization events are committed to a tamper-evident ledger whose
daily root is anchored to a chain and read back. County-level aggregates are
published on a free, unauthenticated, read-only API — and a dashboard lets
anyone check each of those claims against live data rather than taking them on
trust.

Built by [Jarida One](https://jarida.io) with support from the UNICEF
Innovation Fund. Licensed under [Apache 2.0](LICENSE).

---

## What the proposal promised, and what runs today

Every "See it yourself" below is a command, an endpoint or a test name that
exists in this repository and was run against the running stack while this
README was written.

| Pillar | What runs today | Status | See it yourself |
|---|---|---|---|
| **Prediction model** | Four published threshold rules decide every risk level. A fitted climatology — 3,780 empirical quantiles over 18,195 fourteen-day windows of ERA5 reanalysis — annotates every stored score with how unusual that weather was, and can be promoted to the deciding scorer with `PREDICTOR=climatology`. | Runs. Not validated against any disease outcome, because this repository holds no outbreak data. | Dashboard → **Model**; `curl -s localhost:8080/v1/model \| jq '{activePredictor, referenceSha256, exceedanceRole}'` |
| **Threshold validation** | Two of the four published cutoffs (pneumonia, meningitis) cannot fire in the five monitored counties. Reported, not amended — they are contractual. | Runs, and is the most useful result here. | `go test ./internal/predict -run TestPublishedTemperatureThresholdsAreUnreachableInReferenceDecade` |
| **Generative AI — county briefings** | A briefing service writes one briefing per county per language from an aggregate fact sheet. The **default generator is a deterministic template that says so on its first line**. A language model is opt-in (`make up-ai`, or the Claude API with a key this repo never ships). Every model draft is checked against the fact sheet and a failing draft is rejected, never served. | Runs by default with no model. The model paths are tested against committed response shapes, **never against a live model on this machine**. | Dashboard → **Briefing**; `curl -s "localhost:8080/v1/briefings?area=Kisumu&lang=en" \| jq .` |
| **Blockchain — chain anchor** | Each day's Merkle root is written to a `RootAnchor` contract on the development chain this stack starts, and read back with `eth_call` before the day is reported as anchored. A read-back mismatch is an error. | Runs. **A local development chain, not a public network**; its history is deleted by `make down -v`. | Dashboard → **History**; `curl -s localhost:8080/v1/ledger/anchors/verify \| jq .` |
| **Tamper-evident immunization record** | Per-child HMAC-SHA256 leaves, RFC 6962 Merkle trees, inclusion proofs, append-only events enforced by a database trigger, and a guarded right-to-erasure path. | Runs. `ForgetChild` is a tested library function with no endpoint calling it. | `make demo` prints an inclusion proof for an event it recorded moments earlier |
| **Guardian messaging** | Bilingual GSM-7 templates, consent gate, quiet hours, per-child dedup, and a Channel port. The default channel **records what it would send and sends nothing**. | Runs to the channel boundary. No SMS has ever been sent by this system. | Dashboard → **Messaging**; `var/outbox.jsonl` after `make demo` |
| **No personal data on a public surface** | Aggregates only, with k≥10 suppression on every count derived from people. Ledger leaves are per-child HMACs and are never published; only whole-day roots are. | Enforced by two contract tests CI runs by name. | `go test ./internal/publicapi -run 'TestContract_PIILeak\|TestContract_KAnonymity' -v` |
| **Zero credentials, one command** | `cp .env.example .env && make up && make demo` on a clean machine, offline once the images are pulled. No key, token or account anywhere. | Runs. | The Quick start below |
| **Coverage gate ≥80%** | 90.6% of statements over `./internal/...`, excluding generated code. | Green. | `go run ./scripts/covergate -profile coverage.out -threshold 80` |

[NOTES.md](NOTES.md) is a deliberately unflattering account of what is
implemented, what is stubbed and what is thin. [docs/roadmap.md](docs/roadmap.md)
says what each pillar needs next and which of those needs are dependencies on
someone else. Read both before forming a view.

---

## Quick start

```bash
cp .env.example .env
make up
make demo
```

`make up` starts **eleven containers** — ten long-running (Postgres, the seven
Go services, `anvil` the local development chain, and the dashboard) plus a
one-shot migration that exits 0 — and waits for every health check. The
**first** run compiles eight Go binaries and the dashboard, which takes several
minutes on a cold Docker cache; afterwards it reaches healthy in about a
minute.

| Surface | URL |
|---|---|
| Dashboard | <http://localhost:8081> |
| Public API | <http://localhost:8080/v1/risk/current> |
| Registry API (internal) | <http://localhost:8082> |

`make down` tears everything down and deletes the database volume **and the
development chain's history**.

### What `make demo` prints

Copied verbatim from a real run against the stack described above. The demo
ingests a committed fixture scenario — a Kisumu long-rains window — so its
output is identical on every machine.

```
============================================================
ClimateShield — walking skeleton demo
requesting ingest from: fixture (committed demo scenario, not live weather)
============================================================
seeded fictional population: 15 guardians (1 opted out), 28 children, 272 immunization events
enqueued climate_ingest -> risk_predict -> alert_dispatch
recorded immunization event a31dcda0-ca6a-4478-8f71-97ff04970595 (opv3) via registry API
NOTE: quiet hours (21:00-07:00 EAT) — alert dispatch is deferred to 07:00 EAT;
      alert counts below will be zero, honestly.
waiting for risk scores for all 5 counties ... done
waiting for ledger sweep of the recorded event. ... done

--- Outbreak risk (latest per county x disease) ---
scored from observations ingested via: fixture (committed demo scenario, not live weather) [5 counties]
    Eldoret  cholera     LOW    (peak_rainfall_mm_14d = 8.0, rules v1.0.0)
    Eldoret  malaria     LOW    (peak_rainfall_mm_14d = 8.0, rules v1.0.0)
    Eldoret  meningitis  LOW    (mean_max_temp_c_14d = 17.2, rules v1.0.0)
  ⚠ Eldoret  pneumonia   MEDIUM (mean_max_temp_c_14d = 17.2, rules v1.0.0)
  ⚠ Kisumu   cholera     HIGH   (peak_rainfall_mm_14d = 74.0, rules v1.0.0)
  ⚠ Kisumu   malaria     HIGH   (peak_rainfall_mm_14d = 74.0, rules v1.0.0)
    Kisumu   meningitis  LOW    (mean_max_temp_c_14d = 28.1, rules v1.0.0)
    Kisumu   pneumonia   LOW    (mean_max_temp_c_14d = 28.1, rules v1.0.0)
  ⚠ Mombasa  cholera     MEDIUM (peak_rainfall_mm_14d = 41.0, rules v1.0.0)
  ⚠ Mombasa  malaria     HIGH   (peak_rainfall_mm_14d = 41.0, rules v1.0.0)
    Mombasa  meningitis  LOW    (mean_max_temp_c_14d = 31.6, rules v1.0.0)
    Mombasa  pneumonia   LOW    (mean_max_temp_c_14d = 31.6, rules v1.0.0)
    Nairobi  cholera     LOW    (peak_rainfall_mm_14d = 18.0, rules v1.0.0)
    Nairobi  malaria     LOW    (peak_rainfall_mm_14d = 18.0, rules v1.0.0)
    Nairobi  meningitis  LOW    (mean_max_temp_c_14d = 23.4, rules v1.0.0)
    Nairobi  pneumonia   LOW    (mean_max_temp_c_14d = 23.4, rules v1.0.0)
    Nakuru   cholera     LOW    (peak_rainfall_mm_14d = 12.0, rules v1.0.0)
    Nakuru   malaria     LOW    (peak_rainfall_mm_14d = 12.0, rules v1.0.0)
    Nakuru   meningitis  LOW    (mean_max_temp_c_14d = 21.0, rules v1.0.0)
    Nakuru   pneumonia   LOW    (mean_max_temp_c_14d = 21.0, rules v1.0.0)
5 elevated (HIGH/MEDIUM) county-disease pairs

--- Same weather, both scorers (kisumu, 14-day window from 2026-08-07) ---
the levels that produced the alerts above came from: rules v1.0.0
  disease      published thresholds                    reference climatology
  cholera      HIGH   74.0mm [at the record extreme]   HIGH   74.0mm [at the record extreme]
  malaria      HIGH   74.0mm [at the record extreme]   HIGH   74.0mm [at the record extreme]
  pneumonia    LOW    28.1C [top 59.5%]                LOW    18.1C [top 61.5%]
  meningitis   LOW    28.1C [top 40.5%]                LOW    28.1C [top 40.5%]
the two columns read the same weather; they do not always read the same variable —
for pneumonia the published rule uses the 14-day mean MAXIMUM temperature and the
climatology uses the mean MINIMUM (see docs/threshold-validation.md).
neither column is validated against disease outcomes: this system holds none.
only the active predictor above wrote scores or triggered alerts; the other column
was computed by this demo for comparison and sent nothing.

--- Alerts ---
[mock] would send 0 alerts
(mock channel active: NO SMS was sent; see var/outbox.jsonl for the rendered messages)

--- Tamper-evident ledger ---
  2026-09-09: 273 leaves, root 0560fd76f0d1ef59…
    anchored: chain id 31337 (local development chain started by this stack — not a public network)
              contract 0x5fbdb2315678afecb367f032d93f642f64180aa3
              tx 0x5faf417978e6c4bd444716d2b0a73218c751ff265fa5f42bff67b02f0a40685a in block 2
              read-back rootOf(day) == database root: OK
    check it yourself, without trusting this program:
      docker compose exec anvil cast call 0x5fbdb2315678afecb367f032d93f642f64180aa3 "rootOf(bytes32)(bytes32)" 0x323032362d30392d303900000000000000000000000000000000000000000000 --rpc-url http://127.0.0.1:8545
  inclusion proof for event a31dcda0…: OK (leaf 192 of 273 under root 0560fd76f0d1ef59…)

--- Public API ---
  GET http://localhost:8080/v1/risk/current -> 200 (5681 bytes JSON)
  dashboard: http://localhost:8081

--- County briefing ---
waiting for the briefing service to write Kisumu's briefing. ... done
  Generated by a deterministic template — no language model ran. Template template-v1, facts b9c80c69025a.
  | [mock] no language model ran — deterministic template.
  | 
  | Kisumu, forecast window 2026-08-07 to 2026-08-20 (14 days, source: fixture).
  | 
  | Cholera: HIGH. peak 14-day rainfall of 74.0mm is at or above the HIGH threshold of 60mm. For reference, that is the highest value on record for kisumu in month 8 (310 reference windows); the published threshold, not this figure, set the level.
  | Malaria: HIGH. peak 14-day rainfall of 74.0mm is at or above the HIGH threshold of 40mm. For reference, that is the highest value on record for kisumu in month 8 (310 reference windows); the published threshold, not this figure, set the level.
  | Pneumonia: LOW. mean 14-day maximum temperature of 28.1°C is above the MEDIUM threshold of 19°C. For reference, 59.5% of reference windows for kisumu in month 8 are at least this low (310 reference windows); the published threshold, not this figure, set the level.
  | Meningitis: LOW. mean 14-day maximum temperature of 28.1°C is below the MEDIUM threshold of 36°C. For reference, 40.5% of reference windows for kisumu in month 8 are at least this high (310 reference windows); the published threshold, not this figure, set the level.
  | 
  | Elevated now: Cholera (HIGH), Malaria (HIGH). For those, prepare clinic stock and staffing, and check which children in Kisumu are due or overdue for immunization.
  | The mock channel is active: alerts are rendered and recorded, and no SMS is sent.
  | These risk levels describe weather measured against the published thresholds. They do not forecast an outbreak, and this system holds no outbreak surveillance data.
  | 
  | Scored by rules v1.0.0.
  facts behind it: 4 scored diseases, window 2026-08-07 to 2026-08-20 (source fixture), facts b9c80c69025a…
  read it yourself, in English or Kiswahili:
    curl -s "http://localhost:8080/v1/briefings?area=Kisumu&lang=sw" | jq .

demo complete.
============================================================
```

Two things about that run are worth knowing before a live demonstration.

**`[mock] would send 0 alerts` is quiet hours, not a broken alert path.** This
capture was taken between 21:00 and 07:00 East Africa Time, when dispatch is
deferred to 07:00 — the demo prints a NOTE saying exactly that before the risk
grid. Run it outside those hours and the same fixture produces a fan-out of
alerts, still as `would_send`, still transmitted nowhere.

**The demo reports the source it *actually* scored from**, read back from the
database rather than assumed from its own configuration, so it cannot claim
fixture data while showing live numbers. `make demo-live` runs the same flow
against live Open-Meteo forecasts; real weather means the risk levels will
differ from the above.

---

## The prediction model

Two scorers ship. `PREDICTOR=rules` is the default, and in the default
deployment the published thresholds decide **every** level.

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
> Checked against ten years of ERA5 reanalysis (2015–2024, 18,195 fourteen-day
> windows across the five counties): the **coldest single-day maximum on record
> is 16.3 °C**, so the pneumonia rule — which needs a 14-day *mean* ≤ 16 °C —
> can never trigger. The **hottest is 36.2 °C** against a meningitis cutoff of
> 39 °C. As absolute cutoffs, both are inert.
>
> The meningitis figure appears calibrated for the Sahel meningitis belt rather
> than the Kenyan highlands and coast. The pneumonia rule measures the wrong
> variable: cold stress shows up in daily *minimum* temperature, where Eldoret,
> Nairobi and Nakuru sit at 10–12 °C.
>
> **The thresholds are unchanged in this repository** — they are contractual,
> and amending them is a proposal decision, not a commit. The finding is
> reported instead: see
> [docs/threshold-validation.md](docs/threshold-validation.md), the per-rule
> `note` on `GET /v1/model` (which names the reference value each verdict was
> measured against), and a test that fails if it ever stops being true.

### `climatology` — per-county seasonal percentiles

Scores each forecast window against that county's own distribution for that
calendar month, measured from the same reference decade and embedded in the
binary as a 63 KB artifact: 5 counties × 12 months × 3 drivers × 21 quantile
steps = **3,780 empirical quantiles**, measured from **18,195** windows.

It reports an **exceedance of the climate driver** — "this window is in the
most extreme 2% of the last decade for this county and month". That is a
property of the *weather*. It is **not** a probability that an outbreak will
occur, and this system cannot estimate one. Where no reference distribution
exists it reports "not scored" rather than a confident LOW.

A percentile is defined in every climate, which is precisely why it still works
for the two diseases whose absolute cutoffs do not. For cold stress it uses
daily minimum temperature.

### How the two fit together in a default deployment

A `PREDICTOR=rules` deployment records an exceedance **annotation** on every
stored score, plus one sentence explaining it. The annotation is measured after
the fact and moves nothing: the published thresholds still set every tier and
trigger every alert. `GET /v1/model` carries the sentence describing which case
the live deployment is in, so a reader never has to infer it.

Where the annotation surfaces: on the Model view, in the fact sheet returned by
`GET /v1/briefings`, and in the "same weather, both scorers" block `make demo`
prints. The risk endpoints themselves report the level, driver and driver value
that the published thresholds acted on, and nothing more.

Every `risk_scores` row records the predictor name and version that produced
it, so scores stay auditable across scorer changes.

Neither scorer is machine learning, and neither has been trained on health
data. [docs/model-card.md](docs/model-card.md) is the canonical statement of
intended use, method, operating points, evaluation and limitations; it states
plainly that reachability is the **only** evaluation performed and that there is
no outcome validation of any kind.

The reference artifact is rebuilt by `make climatology`, a developer-only tool
that reads the Open-Meteo archive (free, keyless) and prints the SHA-256 it
wrote. Nothing in `make up`, `make demo`, the tests or CI runs it. The committed
artifact's digest is `acc41f68…c41c8d`, published on `GET /v1/model` as
`reference_sha256` and recorded in the model card and the threshold-validation
document.

> **Unproven:** `make climatology` has not been run in this branch, so
> byte-identical regeneration from the archive has not been demonstrated
> end to end. What *is* proven without a network: the generator re-emits the
> committed artifact byte for byte, and its windowing reproduces the committed
> per-county-per-month sample counts exactly.

**Verify it yourself.**

```bash
go test ./internal/predict -run TestPublishedTemperatureThresholdsAreUnreachableInReferenceDecade -v
curl -s localhost:8080/v1/model | jq '.rules[] | {disease, note}'
make climatology-digest    # hashes the committed artifact; makes no network request
```

---

## The tamper-evident ledger and its chain anchor

Every immunization event recorded through the registry is canonically
serialized, hashed into a **per-child HMAC-SHA256 leaf**, and folded into that
day's RFC 6962 Merkle tree. `immunization_events` is append-only, enforced by a
database trigger: `UPDATE` is always rejected, and `DELETE` only inside the
guarded transaction used for right-to-erasure.

Individual leaves are **never published**. A leaf is a per-child artifact and a
public surface cannot be k-suppressed after the fact, so only whole-day roots
leave the system.

### The chain anchor

With `ANCHOR_MODE=evm` — the compose default — each day's root is also written
to a small `RootAnchor` contract and then **read back with `eth_call` before
the day is reported as anchored**. A read-back mismatch is an error, never a
silent success. The contract is single-publisher and append-only per day,
because the sweep legitimately re-anchors a day as late immunizations arrive.

The chain is `anvil`, started by this repository's own `docker compose`, chain
id 31337. Every surface derives that label from `eth_chainId` at runtime rather
than hard-coding a claim, which is why the demo, the API and the dashboard all
say *"local development chain started by this stack — not a public network"*
and that its history does not outlive `make down -v`.

`GET /v1/ledger/anchors/verify` performs a live `rootOf(day)` call and returns
`verified`, `mismatch` or `unavailable` with a plain-language reason. A check
that cannot run — no chain configured, no RPC reachable, a node reporting a
different chain id — is `unavailable` and says why. It never invents a match and
it never fails the read.

The contract is compiled **once** by `make contract` with a digest-pinned
`solc` image; the ABI and bytecode are committed and a test fails if they drift
from the hashes in `BUILD.txt`. Nothing at build, test or run time needs `solc`.

**Verify it yourself.** Ask the API, then ask the chain directly and compare the
two hex strings by eye.

```bash
curl -s localhost:8080/v1/ledger/anchors/verify | jq '{status, dbRootHex, chainRootHex, chainLabel}'
docker compose exec anvil cast call $(curl -s localhost:8080/v1/ledger/anchors/verify | jq -r .contractAddress) \
  "rootOf(bytes32)(bytes32)" \
  $(curl -s localhost:8080/v1/ledger/anchors/verify | jq -r .dayBytes32) \
  --rpc-url http://127.0.0.1:8545
```

To watch it catch a tamper, change `daily_roots.root` in Postgres and call the
verify endpoint again: it returns `mismatch` and names the day. Restoring the
row returns it to `verified`.

---

## County briefings

The briefing service turns aggregates this system already publishes into a
plain-language county summary in English and Kiswahili, and prints the fact
sheet beside it so every sentence can be checked against the numbers it was
allowed to use.

**The default generator is a deterministic template**, and every briefing it
writes begins with `[mock] no language model ran — deterministic template.`
That is why `make up` works offline with zero credentials.

A language model is **opt-in**, two ways:

- `BRIEFING_GENERATOR=openai` — a locally hosted open-weights model behind an
  OpenAI-compatible endpoint. `make up-ai` starts one (Ollama + qwen2.5:1.5b,
  Apache-2.0). Still no credential.
- `BRIEFING_GENERATOR=anthropic` — the Claude API. Requires `ANTHROPIC_API_KEY`,
  which this repository never ships and CI never sets.

Asking for a generator without its credential **fails startup** rather than
silently serving templates while claiming a model.

### Why a language model cannot invent anything here

- **It never sees a person.** The `FactSheet` type has no field for a child, a
  guardian, a phone number or a count below the k≥10 threshold.
  `TestFactSheetHasNoPersonFields`, plus a request-shape test in each adapter,
  assert that on the actual bytes that leave the process.
- **Every draft is checked against its fact sheet.** A number not traceable to
  the sheet, another county, an unscored disease, a disease at the wrong tier,
  an accuracy or outbreak-prediction claim, an "SMS sent" claim in English or
  Kiswahili, a person- or phone-shaped string, or a model writing our own
  `[mock]` label — each **rejects** the draft.
- **A rejected draft is never served, stored or logged.** The labelled template
  is served instead, and the violation *kinds* are published on
  `GET /v1/briefings` so the refusal is visible without repeating the invented
  text.
- **No generated text ever reaches a guardian.** Alert SMS comes only from the
  fixed, length-checked templates in `internal/notify`.
- **Nothing generates on the request path.** Briefings are written by a River
  job on the briefing service's own sweep; a county whose fact-sheet hash has
  not changed regenerates nothing.

> **Unproven, and stated as such:** `make up-ai` has never been run on this
> machine, and no live Anthropic API call has ever been made from this
> repository. Both model adapters are verified against committed golden
> response shapes served by `httptest`, never against a network host. Nobody
> here has watched a language model write one of these briefings.
>
> The **Kiswahili wording is not reviewed by a Kiswahili speaker.** It was
> hand-written by the implementer. The grounding check catches invented facts;
> it does not judge grammar. Qwen2.5's model card does not list Kiswahili, so a
> local model's Kiswahili output quality is unverified too. Every surface says
> so, and keeps saying so until a named reviewer signs it off.

**Verify it yourself.**

```bash
curl -s "localhost:8080/v1/briefings?area=Kisumu&lang=en" | jq '{generator, model, promptVersion, grounded, status, groundingNotes}'
go test ./internal/briefing -run TestAdversarialDrafts -v
go test ./internal/briefing/facts -run TestFactSheetHasNoPersonFields -v
```

---

## The honesty commitments

Four things this project refuses to do, each checkable in the code.

**1. No SMS is sent.** The default messaging channel is a mock that records
what it *would* send and says so. Alerts are recorded as `would_send`, never
`sent`; output says `[mock] would send N alerts`. Only a real carrier adapter
may write `sent`, and none has ever been connected.

**2. No accuracy claim.** There is no outbreak surveillance data here, so no
scorer has been validated against disease outcomes and none reports a
sensitivity, specificity or accuracy figure anywhere. There is no latency,
uptime or "families protected" number in this repository either, and there
should not be one until an evaluation exists.

**3. The chain is a local development chain, and the README says which.** What
*does* happen: `make up` starts `anvil` (chain id 31337) as part of this stack;
each day's Merkle root is written to a `RootAnchor` contract on it and read back
with `eth_call` before the day is reported as anchored; `make down -v` deletes
that chain's history along with the database. What does **not** happen: nothing
here is written to any public network, and no surface in this repository calls
this chain public, immutable or decentralised. Anchoring to a public network is
**deliberately not wired**, because it would need a funded signing key and this
project's zero-credential rule forbids one.

**4. Generated text is labelled, grounded, or not served.** The default
briefing generator is a deterministic template whose first line says a language
model did not run. A model is opt-in. Every briefing carries its generator,
model and prompt version alongside the hash of the fact sheet it was written
from, and a draft that fails the grounding check is rejected — the labelled
template is served in its place with the reasons why. Serving model-labelled
text that no model produced would be the "SMS sent" lie in a new costume.

And two commitments it does make:

- **No personal data reaches any public surface.** Aggregates only, with k≥10
  suppression on every count derived from people. Enforced by
  `TestContract_PIILeak` and `TestContract_KAnonymity`, which CI runs **by
  name** and greps for their `RUN` and `PASS` lines — because `go test -run`
  exits 0 when a test has been deleted.
- **No credentials required.** `git clone && cp .env.example .env && make up`
  works on a clean machine.

---

## The dashboard

Nine views. Each exists so a reviewer can **verify** a capability against
running data, and each carries an on-screen statement of what it does *not*
prove.

| View | What it lets you check |
|---|---|
| **Overview** | What runs today, with a live figure per pillar and the k-anonymity table |
| **Risk map** | Current risk per county, filterable by disease |
| **Model** | Both scorers on the same weather, and how the published thresholds were checked |
| **Weather** | The exact 14-day forecast window each score was computed from |
| **Briefing** | The county briefing beside the fact sheet every sentence must come from |
| **Messaging** | Message outcomes, both language templates, and a live GSM-7 previewer |
| **History** | Daily Merkle roots, the anchor, and a "verify on chain now" button |
| **Automation** | Job history from the queue, with optional auto-refresh |
| **Open data** | Every endpoint, runnable from the page itself |

A status strip under the nav puts the weather source, the messaging channel
(`mock — no SMS is sent`), the active scorer and `demo population: fictional`
on **every** view, so the mock disclosure follows the reader rather than living
on one page.

Charts are hand-built SVG — no charting dependency on a page with an uptime
obligation — and every chart ships a data table so nothing is gated behind
colour. All forms **query or preview**; none writes, because the public tier is
read-only. The one control that accepts free text (a child's first name in the
message previewer) renders entirely in the browser, so nothing typed into it is
transmitted, logged or stored.

---

## Architecture

[docs/architecture.md](docs/architecture.md) is the current reference, drawn
from what the Go code does. In brief:

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
registry ──▶ immunization_events ──▶ ledger ──▶ HMAC leaves ──▶ daily Merkle root
                                                                     │
                                          local anchors table ◀──────┴──────▶ RootAnchor
                                                                              on anvil
                                                                       (read back before
                    risk_scores + aggregate counts                      reporting success)
                              │
   briefing ◀── fact sheet ───┤  (its own sweep; unchanged facts regenerate nothing)
                              ▼
                          publicapi ──▶ JSON / CSV / GeoJSON + Connect ──▶ dashboard
```

Seven Go services, one Go module, one Postgres database. Work moves between
services through [River](https://riverqueue.com) (Postgres-backed), so no
message broker is required.

| Service | Port | Responsibility |
|---|---|---|
| `ingestor` | 8090 | Fetch daily forecasts for 5 counties; idempotent upsert |
| `predictor` | 8091 | Score outbreak risk; annotate with exceedance; enqueue alerts for HIGH/MEDIUM |
| `notifier` | 8092 | Render bilingual SMS, respect consent and quiet hours, dispatch via a Channel |
| `ledger` | 8093 | Commit immunization events to daily Merkle trees; anchor and read back roots |
| `briefing` | 8094 | Write one county briefing per language from the aggregate fact sheet |
| `registry` | 8082 | Children, guardians, KEPI schedule, immunization events (Connect) |
| `publicapi` | **8080** | Public read-only aggregates (REST + Connect) |
| `web` | **8081** | Demonstration dashboard |

Alongside them: `postgres`, `anvil` (the development chain), and a one-shot
`migrate` container that applies migrations and exits.

Only `publicapi` and `web` are intended to be publicly exposed; the production
overlay in [`deploy/`](deploy/README.md) publishes nothing else, including the
development chain.

**Stack.** Go 1.26 · chi · ConnectRPC + Protobuf (buf) · PostgreSQL 16 +
PostGIS · pgx + sqlc · golang-migrate · River · `log/slog` with mandatory PII
redaction · Prometheus · testcontainers · TypeScript strict + React + Vite +
MapLibre GL JS · Docker Compose · GitHub Actions. Every dependency is open
source and free; the system builds, tests and runs with zero credentials.

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
| `GET /v1/model` | Active scorer, published thresholds, reachability, reference record and its digest |
| `GET /v1/climatology` | Reference distribution for one county and month (`?area=&month=`) |
| `GET /v1/climate/series` | The forecast window each score was computed from |
| `GET /v1/ledger/summary` | Daily Merkle roots and anchors — never individual leaves |
| `GET /v1/ledger/anchors/verify` | A live `rootOf(day)` call: `verified`, `mismatch` or `unavailable` |
| `GET /v1/briefings` | One county briefing (`?area=&lang=en\|sw`), its provenance and its fact sheet |
| `GET /v1/alerts/summary` | Messaging outcomes, channel status, rendered templates |
| `GET /v1/pipeline` | Job history and data volumes |

`GET /v1/risk/history` accepts `area`, `disease`, `from`, `to` (`YYYY-MM-DD`)
and `limit` (clamped to 1000).

**Formats.** Add `?format=csv` or `?format=geojson` to the risk endpoints
(`csv` also on `/v1/stats`); JSON is the default. `/v1/briefings` is JSON only.
Unsupported combinations return `400`, never `500`.

```bash
curl -s localhost:8080/v1/risk/current | jq .
curl -s "localhost:8080/v1/risk/current?format=geojson" | jq .type   # FeatureCollection
curl -s localhost:8080/v1/model | jq '.rules[] | {disease, note}'
```

Each rule's `note` names the reference value its verdict was measured against,
for the two cutoffs that can fire as well as the two that cannot. The boolean
`reachableInReferencePeriod` sits beside it and is omitted from the JSON when
false, which is protobuf's default-value encoding rather than a missing answer
— read the `note`.

The same messages are served over ConnectRPC at
`/climateshield.v1.PublicService/…`. Protobuf definitions live in
[`proto/climateshield/v1`](proto/climateshield/v1) and are the single source of
truth: the Go services and the dashboard's TypeScript client are both generated
from them, so no response type is written twice.

### Availability

A public read **never returns 500**. Each endpoint caches its last good
response; if the database becomes unreachable that body is served with
`X-Data-Stale: true`, and the dashboard says under the nav that it is showing
the last good response rather than presenting cached figures as current. On a
cold start with a dead database the response is an empty but structurally valid
payload — still `200`, still flagged. `/health` reports `503` independently, so
monitoring sees the truth while readers keep getting data.

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
  `*_suppressed` flag. Zero and ≥10 pass through. `GET /v1/stats` against the
  demo population shows both cases at once.

Both properties are enforced by `TestContract_PIILeak` and
`TestContract_KAnonymity`, which CI runs by name.

---

## Development

```bash
make verify   # fmt · vet · lint · build · test · coverage gate · buf lint · contracts · tsc · web build
make test     # tests only, with a coverage profile
make generate # regenerate protobuf (Go + TS) and sqlc output
make migrate  # apply migrations against DATABASE_URL
make help     # list documented targets
```

**Toolchain.** Go 1.26 (`go.mod` pins it, and the build image is
`golang:1.26-alpine`), Docker, and Node 22 for the dashboard only. `buf`, `sqlc`
and the protobuf plugins are pinned as `go.mod` tool dependencies and run via
`go tool`; `golangci-lint` is version-pinned into `./bin` by the Makefile.
Nothing needs a global install and every version is locked by `go.sum`.

**Tests.** Unit tests plus database-backed tests using a throwaway PostGIS
container (testcontainers; Docker must be running). **No test touches the
network** — the Open-Meteo client, both language-model adapters and the
JSON-RPC anchor client are all tested against local `httptest` servers replaying
committed golden JSON. `go test -short ./...` skips everything needing Docker.

> Run `go test ./... -p 2`. Running every package's containers concurrently
> saturates Docker and the integration test fails to connect.

The repository does make outbound requests outside the test suite, and it is
worth being precise about which: `make climatology` reads the Open-Meteo
archive, `make demo-live` and `CLIMATE_SOURCE=openmeteo` fetch a live forecast,
`make lint` downloads `golangci-lint`, `make up` pulls images, `make up-ai`
pulls a model, and the dashboard's basemap comes from a public tile server. No
test makes any of them.

### Coverage

The gate requires **≥80%** statement coverage over `./internal/...`, enforced by
[`scripts/covergate`](scripts/covergate) in `make verify` and CI. Generated code
(`internal/gen`, `internal/store/db`) is excluded; nothing else is. The
exclusions are policy, recorded in [CLAUDE.md](CLAUDE.md).

> **Current: 90.6% (2819/3113 statements) — the gate is green.** It was red at
> 66.8% one branch ago; the fix was to write the tests, not to move the
> threshold. Per-package figures are in [NOTES.md](NOTES.md).

```bash
make test && go run ./scripts/covergate -profile coverage.out -threshold 80
```

### Repository layout

```
cmd/            thin service binaries + migrate + demo + buildclimatology
internal/
  briefing/     fact sheet, generator port (template/openai/anthropic), grounding check
  climate/      ClimateSource interface, Open-Meteo + fixture sources, ingestor
  predict/      Predictor interface, rules, climatology model + reference data
  registry/     children, guardians, KEPI due/overdue, Connect API
  ledger/       canonical serialization, HMAC leaves, Merkle trees, anchors, erasure
  notify/       Channel port, mock/smpp/africastalking adapters, GSM-7 templates
  publicapi/    REST + Connect, formats, k-anonymity, stale cache, evidence views
  store/        migrations, sqlc queries, seed data, testcontainers harness
  platform/     config, redacting logger, AES-GCM fields, EAT clock, metrics
  integration/  boots all seven services against a real database
proto/          protobuf contract — the source of truth for API types
testdata/golden Open-Meteo-shaped fixtures encoding the demo scenario
web/            Vite + React + MapLibre dashboard
docs/           architecture, model card, threshold validation, roadmap
reference/      the original Python prototype, plus prototype-docs/ — the
                superseded diagrams and wireframes, with a README naming the
                claims in them that were never substantiated
```

---

## Configuration

Copy `.env.example` to `.env`. Every value there is a working development
default; none is a credential. `.env` is gitignored and no secret is committed.
Variables below that `.env.example` does not list take the same default from
the service's own config struct, so leaving them unset is the documented case.

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | local Postgres DSN | Used by host-side tooling |
| `POSTGRES_USER` / `_PASSWORD` / `_DB` | `climateshield` | Local container credentials |
| `PII_KEY_HEX` | 64-char dev value | AES-256-GCM key for encrypted columns. **Generate per deployment:** `openssl rand -hex 32` |
| `PII_ALLOW_DEV_KEY` | `true` in `.env.example`; `false` in compose if unset | Services refuse to start on the published placeholder key unless this is `true`. The production overlay never sets it |
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
| `ANCHOR_MODE` | `evm` in `.env.example` and compose; `local` in code | `local` records a row in this system's own database only; `evm` also writes and reads back the `RootAnchor` contract |
| `ANCHOR_RPC_URL` | `http://localhost:8545` | Host-side URL; compose points services at `http://anvil:8545` |
| `ANCHOR_FROM` | *(empty)* | Sending account; empty uses the node's first unlocked development account |
| `ANCHOR_CONTRACT_ADDRESS` | *(empty)* | Empty deploys once per chain and remembers the address |
| `ANCHOR_CONFIRM_TIMEOUT` | `30s` | How long to wait for a receipt before failing |
| `BRIEFING_GENERATOR` | `mock` | `mock` (deterministic template, no model), `openai` or `anthropic` |
| `BRIEFING_MODEL` | *(empty)* | Empty uses the selected generator's default |
| `BRIEFING_SWEEP_INTERVAL` | `15m` | How often a county's fact sheet is re-checked; unchanged facts regenerate nothing |
| `BRIEFING_TIMEOUT` | `120s` | Bound on one generation, which never happens on a request path |
| `BRIEFING_OPENAI_BASE_URL` | `http://localhost:11434/v1` | OpenAI-compatible endpoint for a local model |
| `ANTHROPIC_API_KEY` | *(empty)* | Not a credential this project has, needs or ships |
| `ONNX_MODEL_PATH` | *(empty)* | Not implemented; a non-empty value **fails startup** rather than silently falling back |
| `PUBLICAPI_ADDR` | `:8080` | Public API listen address |
| `REGISTRY_ADDR` | `:8082` | Registry listen address |
| `INGESTOR_/PREDICTOR_/NOTIFIER_/LEDGER_/BRIEFING_ADDR` | `:8090`–`:8094` | Health and metrics addresses |
| `LOG_LEVEL` | `info` | JSON to stdout, PII-redacted |

---

## Operational notes

**Messaging honesty.** With `NOTIFY_CHANNEL=mock` nothing is sent anywhere.
Alerts are recorded as `would_send`, never `sent`. Output says
`[mock] would send N alerts`.

**Quiet hours.** No alert is dispatched between 21:00 and 07:00 East Africa
Time (fixed UTC+3; Kenya observes no DST). Jobs landing in the window are
rescheduled to the next 07:00 rather than dropped — which is why a demo run
inside that window honestly reports zero alerts.

**Consent.** `consent_log` is append-only and the most recent row per guardian
decides. A guardian whose latest action is `OPT_OUT` is skipped, and the skip is
recorded as `skipped_consent`.

**Data protection.** Child names, guardian names, phone numbers and national
IDs are stored only as AES-256-GCM ciphertext, with the key supplied via
`PII_KEY_HEX` and never stored in the database. Services refuse to start on the
published placeholder key unless `PII_ALLOW_DEV_KEY` is set — a demo cannot
quietly become a production deployment. Logging goes through a redacting handler
that masks phone-shaped strings even when a caller forgets the typed wrappers.

**Immutability and erasure.** `immunization_events` is append-only, enforced by
a database trigger: `UPDATE` is always rejected, `DELETE` only inside the
guarded transaction used for right-to-erasure. `ledger.ForgetChild` deletes the
child's records, scrubs the child linkage from ledger leaves and destroys the
child's HMAC key — after which previously published roots still verify, but
nothing links those leaves to a person. It is a library function; no endpoint
calls it yet.

**Basemap.** The dashboard fetches its base map from MapLibre's public demo tile
server. It needs no API key but does require internet access; without it the map
says so plainly and the county markers and risk levels remain accurate.
Everything else runs fully offline.

---

## Not in scope

USSD, an Android app, ONNX inference, model training, FHIR/DHIS2 integration,
real SMS delivery, anchoring to a public chain (only the local development
chain is anchored to), authentication, multi-tenancy, Kubernetes, and an admin
UI. [NOTES.md](NOTES.md) says exactly where each boundary sits in the code, and
[docs/roadmap.md](docs/roadmap.md) says what each would need.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). All first-party files carry
`SPDX-License-Identifier: Apache-2.0`; `make verify` fails if one is missing.
For anything with security or privacy implications — especially a suspected data
leak on a public surface — email **hello@jarida.io** rather than opening a
public issue.
