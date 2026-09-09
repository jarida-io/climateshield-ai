<!-- SPDX-License-Identifier: Apache-2.0 -->

# Roadmap

What each pillar of the funding proposal does today, what the next milestone
for it is, and the test that will decide whether that milestone was reached.

There are **no dates in this document**. Several of these milestones are not
blocked on effort — they are blocked on data, on a sandbox account, or on a
named person's review, and none of those is ours to schedule. Those are listed
separately at the end, as dependencies rather than as plans.

An acceptance test below is a real thing that can pass or fail: a Go test name
that does not exist yet, a command whose output can be compared, or an
observation a named person signs off. "Looks better" is not an acceptance test.

## Pillar by pillar

| Proposal pillar | What runs today | Next milestone | Acceptance test |
|---|---|---|---|
| **Prediction model** | Four published threshold rules in `internal/predict/rules.go` decide every risk level. A fitted climatology (3,780 quantiles over 18,195 windows, `internal/predict/climatologydata/`) annotates every score with the driver's exceedance. `PREDICTOR=climatology` promotes it to the deciding scorer. | Outcome validation: measure how the four rules and the climatology would actually have performed against recorded outbreaks. | A test that reads a committed outbreak dataset and reports per-rule hit and false-alarm counts, with the numbers published on `GET /v1/model` — or, if the rules do not perform, a written finding saying so in `docs/model-card.md`. Blocked: see D1. |
| **Prediction model — reproducibility** | `make climatology` rebuilds the reference artifact from the Open-Meteo archive; `make climatology-digest` hashes the committed copy offline. The generator is proven to re-emit the committed file byte for byte, and to reproduce its per-county-per-month sample counts. | Prove a rebuild from the archive returns the same artifact, confirming the inferred quantile index rule. | A human runs `make climatology` and the SHA-256 it prints equals `acc41f6890e10eccd16e0052f533a3e7737dcd0da71d723d968c815541c41c8d`. If it differs, the difference is investigated and recorded before anything else changes. |
| **Threshold validation** | Two of the four published cutoffs cannot fire in the monitored counties. Reported on `GET /v1/model`, in `docs/threshold-validation.md`, on the dashboard's Model view, and guarded by `TestPublishedTemperatureThresholdsAreUnreachableInReferenceDecade`. | A decision from the funder on the pneumonia and meningitis rules: amend, re-scope, or accept them as inert. | A proposal amendment (or a written decision to keep them) referenced from `docs/threshold-validation.md`. The code changes only after that. Blocked: see D5. |
| **Generative AI — county briefings** | The briefing service writes one briefing per county per language from an aggregate fact sheet. The default generator is a deterministic template that labels itself. Every model draft passes a grounding check or is rejected, and rejected text is never served. | Watch a real language model write one, on this machine, and watch the grounding check refuse one. | `make up-ai`, then `curl -s "localhost:8080/v1/briefings?area=Kisumu&lang=en"` returns `generator: "openai"` with a real model name and `grounded: true` — plus at least one recorded rejection with its kinds, showing the refusal path firing against a real draft rather than a fixture. |
| **Generative AI — language quality** | English and Kiswahili templates, both hand-written by the implementer. Every surface says the Kiswahili is unreviewed. | A named Kiswahili speaker reviews the SMS templates and the briefing template. | The reviewer's corrections are merged, the "not reviewed by a Kiswahili speaker" label is replaced by the reviewer's name and date on the Briefing view and in `NOTES.md`, and `TestTemplateSWFits160Septets` still passes on the corrected text. Blocked: see D4. |
| **Blockchain — chain anchor** | Each day's Merkle root is written to the `RootAnchor` contract on the local development chain this stack starts, and read back with `eth_call` before the day is reported anchored. `GET /v1/ledger/anchors/verify` re-runs the check live and reports `verified`, `mismatch` or `unavailable`. | Decide whether a public-network anchor is wanted at all, and if so how its signing key is funded and held. | Not a code milestone until the key question is answered. If it is answered yes: an anchor to a public testnet whose transaction hash is resolvable from a block explorer the assessor picks, with the same read-back check passing, and `chain_label` on the API naming that network. Blocked: see D6. |
| **Tamper-evident record** | Per-child HMAC leaves, RFC 6962 Merkle trees, inclusion proofs, an append-only trigger, and `ledger.ForgetChild` as a tested library function. | Make erasure operable: an audited endpoint that invokes `ForgetChild`, and ledger key isolation at the database-role level. | A test that calls the endpoint, asserts the child's records are gone, asserts previously published roots still verify, and asserts the audit log line contains no PII — plus a test that the ledger's role cannot read `sealed.child_keys` through any other query file. |
| **Guardian messaging** | Bilingual GSM-7 templates, consent gate, quiet hours, per-child dedup, and a Channel port. The mock channel records `would_send` and transmits nothing. `internal/notify/smpp` compiles and binds lazily but has never met a carrier. | One real message to one handset, with the delivery receipt recorded. | A manual run against a carrier sandbox in which one alert reaches one test handset and the row's status is `sent` with a delivery receipt stored — the only circumstance in which anything in this system may write `sent`. Blocked: see D2. |
| **Immunization schedule** | 16 KEPI doses seeded, with due ages and an assumed 14-day overdue grace period (`vaccine_schedule.overdue_grace_days`). | Confirm the schedule and the grace period against MoH guidance. | A KEPI officer confirms each dose's due age and the grace period in writing; the seed data is corrected where it differs and `NOTES.md` assumption 1 is replaced by the citation. Blocked: see D3. |
| **Public surface and privacy** | Aggregates only, k≥10 suppression, never-500 reads with a stale cache, two contract tests run by name in CI. | Exercise the stale path against a real outage rather than only in unit tests. | `docker compose stop postgres`, then `curl -si localhost:8080/v1/risk/current` returns `200` with `X-Data-Stale: true` and the dashboard shows its stale banner — observed, screenshotted, and recorded in `NOTES.md`. |
| **Operability** | A production compose overlay, TLS via Caddy, a fail-closed PII key guard, and a deployment guide written to be run by a human on the host. | The four items `deploy/README.md` lists as missing before this could hold real data: key management, ledger role isolation, an invocable erasure path, rehearsed backups. | A restore drill: destroy the volume, restore from backup, and have `make demo`'s inclusion proof verify against a root committed before the destruction. |

## Dependencies, not dates

Each of these is something this repository cannot produce for itself. They are
listed so nobody mistakes them for work that is merely unscheduled.

**D1 — Outbreak surveillance data, for any outcome validation.** This system
holds none, and that single fact is why there is no accuracy, sensitivity or
specificity figure anywhere in it. County-level case counts for cholera,
malaria, pneumonia and meningitis over a period overlapping the reference
decade would make the first row of the table above possible. Without them, the
honest position is the current one: the thresholds are published and traceable,
their reachability has been checked, and nothing further is claimed.

**D2 — A carrier sandbox, for one real SMS.** The Channel port is the riskiest
untested boundary in the system. Proving it needs an SMPP or Africa's Talking
sandbox account and a test handset. Until that exists, `sent` is a status no
row in this database has ever held, and the mock channel says so on every
surface.

**D3 — A KEPI officer, to confirm the schedule and the 14-day grace period.**
The dose codes and due ages are standard; the point at which a due dose becomes
*overdue* is an assumption encoded in the seed data. Everything the notifier
does about "overdue" children rests on it, so it should be confirmed by someone
with the authority to confirm it rather than reasoned about further.

**D4 — A Kiswahili reviewer.** Both the SMS templates and the briefing template
were written by the implementer, who is not a Kiswahili speaker's idea of one.
The grounding check catches invented facts; it does not judge grammar, and a
local open-weights model whose card does not list Kiswahili will not either.
This is the cheapest item on the list and the one most likely to embarrass the
project in front of the people it is for.

**D5 — A proposal amendment, for the two unreachable thresholds.** The
pneumonia and meningitis cutoffs are contractual. `docs/threshold-validation.md`
recommends measuring cold stress on daily minimum temperature and either
re-scoping the meningitis rule to arid northern counties or dropping it — but
amending a published number is a funder decision, not a commit. The finding is
reported and the code is unchanged, deliberately, until that decision exists.

**D6 — A funded signing key, for any public-network anchor.** Anchoring to a
public chain is deliberately not wired: it needs a key with gas in it, and this
project's rule that everything builds, tests and runs with zero credentials
forbids holding one. The development-chain anchor demonstrates the mechanism —
write, read back, refuse to report success on a mismatch — and stops exactly
where a funded key would be required. Whether to cross that line is a funder
and governance decision, not an engineering one.
