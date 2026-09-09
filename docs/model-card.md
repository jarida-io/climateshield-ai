<!-- SPDX-License-Identifier: Apache-2.0 -->

# Model card — ClimateShield risk scoring

> *A fitted statistical baseline over a decade of reanalysis, used to flag
> climatological extremes; no disease model has been trained or validated.*

That sentence is the whole claim. Everything below expands it, and nothing
below widens it.

This card covers both scorers in the system, because both are in the alerting
path depending on configuration:

| | `rules` (default) | `climatology` |
|---|---|---|
| What it is | four published cutoffs, applied as written | empirical per-county, per-month distributions |
| Fitted parameters | none | 3,780 quantiles measured from the reference record |
| Decides the tier | yes, by default | yes, when `PREDICTOR=climatology` |
| Provenance stamped on every score | `rules` + version | `climatology` + version |

Whichever is active, every score is annotated with the reference
climatology's view of its own driver value. **The annotation never changes a
tier** — see `internal/predict/annotate.go` and the test
`TestAnnotationNeverChangesWhatTheRulesDecided`, which fails if it ever does.

## Intended use

**In scope.** Flagging that a county's next 14-day forecast window is
climatologically unusual, or crosses a cutoff published in the funding
proposal, so that a health officer can decide whether to prioritise
outreach for children with due or overdue immunizations in that county.
Every score is published with the driver, the driver value and a one-line
reason a health officer can dispute.

**Out of scope, and unsupported.**

- Diagnosis, triage, or any statement about an individual child.
- Any claim that an outbreak will occur, or an estimate of its probability.
- Clinical guidance. The system holds no case data, no surveillance feed and
  no health outcomes of any kind.
- Deployment outside the five monitored counties. The reference distributions
  do not exist elsewhere, and the scorer reports that rather than guessing
  (`predict.ErrNoClimatology`).

**Users.** County health officers and programme staff, through the public
dashboard and the read-only public API. There is no automated action: an
elevated score renders and records a message on a mock channel, and a person
decides what happens next.

## Reference data

| | |
|---|---|
| Source | Open-Meteo historical archive, serving **ERA5 reanalysis** (`archive-api.open-meteo.com`) |
| Licence | Open-Meteo data, **CC BY 4.0** |
| Cost / credentials | free, keyless — no account is used or needed |
| Period | 2015-01-01 … 2024-12-31 |
| Geography | one point per county, at the county centroid: Nairobi, Kisumu, Mombasa, Nakuru, Eldoret |
| Variables | `precipitation_sum`, `temperature_2m_max`, `temperature_2m_min`, daily, Africa/Nairobi calendar |
| Records | 3,653 days per county |

ERA5 is a reanalysis product, not a station observation. A single centroid
point does not represent a whole county, and reanalysis smooths local
extremes. Both facts are limitations, listed again below.

## Method

1. Fetch each county's daily record for the reference period.
2. Slide a **14-day window** along it — the same window length the runtime
   predictor scores — and reduce each window to the same three features
   `predict.FeaturesFrom` computes at runtime:
   - peak daily rainfall in the window (mm)
   - mean daily maximum temperature (°C)
   - mean daily minimum temperature (°C)
3. Group the windows by the **calendar month of the window's first day**, which
   is how a live forecast window is labelled.
4. For each county × month × feature, store an evenly spaced ladder of
   **21 order statistics** (p0, p5 … p100). Each stored value is a value the
   record actually produced: the index is `p/100 × (n−1)` rounded half to
   even, with no interpolation between neighbouring windows. Values are
   rounded to three decimals.
5. Write the artifact with its own provenance inside it — source, licence,
   period, window length, quantile ladder, and the generator that built it.

At runtime, **exceedance** is the share of reference windows for that county
and month at least as extreme as the observed value, in the direction that is
hazardous for that disease. Values beyond the ends of the ladder clamp to
0 or 1 rather than extrapolating a tail the record does not evidence.

Nothing above is fitted to a health outcome. The parameters are descriptions
of weather, estimated only from weather.

## Reproducing the artifact

| | |
|---|---|
| Artifact | `internal/predict/climatologydata/kenya-5county-2015-2024.json` |
| SHA-256 | `acc41f6890e10eccd16e0052f533a3e7737dcd0da71d723d968c815541c41c8d` |
| Generator | `cmd/buildclimatology` — the path in the artifact's own `generated_by` field |
| Size | 5 counties × 12 months × 3 features × 21 quantiles = **3,780 stored quantiles**, measured from **18,195** 14-day windows |

```sh
make climatology-digest   # hash the committed file; no network request
make climatology          # rebuild it from the archive; prints the SHA-256 it wrote
```

`GET /v1/model` publishes `reference_file`, `reference_sha256`,
`reference_generator`, `reference_windows` and `quantile_steps` so the running
system's copy can be checked against this card.

`make climatology` is the only thing that ever reads the archive, and it runs
only when a person types it. It needs no account and no key. **No test in this
repository touches the network**: the generator's HTTP path is tested against
`httptest` with committed synthetic fixtures
(`cmd/buildclimatology/testdata/golden/`, which say plainly that their numbers
are made up). Other commands do reach the internet for other reasons —
`make demo-live` and `CLIMATE_SOURCE=openmeteo` fetch a live *forecast*,
`make lint` downloads a linter, `make up` pulls container images — so this is
"the only archive read", not "the only network call".

Two things are proven in CI without a network, and they are what make the
digest meaningful:

- `TestEncoderReproducesTheCommittedArtifactByteForByte` — the generator's
  writer re-emits the committed artifact byte for byte, so a rebuild can
  differ only where a measured number differs, never in formatting.
- `TestWindowCountsPerMonthMatchTheCommittedArtifact` — the windowing produces
  the committed artifact's per-county, per-month sample counts exactly.

What is **not** proven in this repository is that re-running the generator
today returns the same ERA5 values it returned when the artifact was built.
Reanalysis products are revised. If a rebuild changes the digest, that is
information, not a bug — record the new digest here and in the commit.

## Operating points

These are **declared choices**, not fitted ones. There is nothing to fit them
against, and stating them here is what makes them arguable.

**Published thresholds (`rules`).** Contractual, from the funding proposal.
They live in exactly one place, `internal/predict/rules.go`, and are not
changed by this work:

| Disease | Driver | HIGH | MEDIUM |
|---|---|---|---|
| Cholera | 14-day peak rainfall | ≥ 60 mm | ≥ 30 mm |
| Malaria | 14-day peak rainfall | ≥ 40 mm | ≥ 20 mm |
| Pneumonia | 14-day mean daily maximum temperature | ≤ 16 °C | ≤ 19 °C |
| Meningitis | 14-day mean daily maximum temperature | ≥ 39 °C | ≥ 36 °C |

**Exceedance cut-points (`climatology`).** A HIGH is roughly a 1-in-50 window
for that county and month, a MEDIUM roughly 1-in-10:

| Tier | Exceedance |
|---|---|
| HIGH | ≤ 0.02 |
| MEDIUM | ≤ 0.10 |

**Driver choice per disease (`climatology`).** Cholera and malaria track peak
rainfall; meningitis tracks the mean daily maximum; pneumonia tracks the mean
daily **minimum**, because the hazard is cold stress and Kenyan daily maxima
stay mild even where nights are cold. See docs/threshold-validation.md.

## Evaluation

**Reachability only. There is no outcome validation of any kind.**

The one evaluation this project can honestly perform is whether each published
cutoff can be reached at all in the reference record. Two of the four cannot:

| Published cutoff | Reachable in the reference record? |
|---|---|
| Cholera ≥ 60 mm | yes — the wettest 14-day peak in the record is 138.3 mm |
| Malaria ≥ 40 mm | yes |
| Pneumonia ≤ 16 °C | **no** — the lowest 14-day mean maximum in the record is 19.9 °C |
| Meningitis ≥ 39 °C | **no** — the highest is 35.2 °C |

Method, per-county firing rates and the recommendation are in
docs/threshold-validation.md. The finding is enforced by
`TestPublishedTemperatureThresholdsAreUnreachableInReferenceDecade`, which
fails if it ever stops being true, and is published per rule on `/v1/model`
with the number each verdict was measured against.

**Not measured, and not claimed anywhere in this repository:** accuracy,
sensitivity, specificity, positive predictive value, lead time, calibration
against cases, or any comparison with observed outbreaks. No such number
exists here, and none may be quoted from this system. Reachable is not the
same as correct.

## Limitations

- **No health outcomes.** Nothing here has been checked against disease data,
  because this system holds none. Whether the reachable cutoffs predict
  outbreaks is unknown and untested.
- **Not learned from outcomes.** The fitted parameters describe weather. Any
  description of this system as AI, machine learning or a trained disease
  model is wrong and should be corrected, including in slides.
- **Ten years is short for tail behaviour.** A p100 is the highest of about
  300 windows, not a physical maximum. A cutoff unreached in this record is
  not proven impossible — though a 14-day mean below the coldest single day in
  the record is impossible regardless of record length.
- **One point per county.** ERA5 at a centroid is not a station observation
  and a county is not climatically uniform.
- **Five counties only.** Outside them there is no reference distribution and
  the scorer says so rather than scoring LOW.
- **The generator drops the final window of the record.** Its loop takes a
  window only while at least one further day follows it, so December carries
  296 windows per decade rather than 297 — 18,195 windows in total rather
  than 18,200. It is preserved deliberately so the committed artifact stays
  reproducible, and pinned by
  `TestWindowLoopStopsOneWindowShortOfTheRecord`.
- **Sub-daily exposure is invisible.** A 14-day mean cannot express a single
  severe night, and daily aggregates cannot express a downpour's intensity.
- **Reanalysis is revised.** The archive may return different values for the
  same historical days in future, which would change the artifact's digest.

## Provenance recorded with every score

Each row in `risk_scores` carries the predictor name, the predictor version,
the driver, the driver value, the forecast date, the window length and — where
the reference record covers the county and month — the exceedance and a
one-line explanation. A score can therefore always be traced to a number, a
method and a sentence.

## Maintenance

Owner: the ClimateShield engineering team. This card is updated in the same
commit as any change to `internal/predict`, to the reference artifact, or to
the artifact's digest. The published thresholds are contractual: changing one
requires a proposal amendment, not a commit.
