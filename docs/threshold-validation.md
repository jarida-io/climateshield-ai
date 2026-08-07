<!-- SPDX-License-Identifier: Apache-2.0 -->

# Threshold validation

**Finding: two of the four published risk thresholds cannot fire in any of the
five monitored counties.** They are not merely rare — they are unreachable.

This document records how that was measured, what it means, and what is
recommended. The thresholds themselves have **not** been changed: they are
published in the funding proposal, and amending them is a proposal decision,
not a code change.

## Method

Daily values for each county centroid were retrieved from the Open-Meteo
historical archive (ERA5 reanalysis), 1 January 2015 to 31 December 2024 —
3,653 days per county. Every 14-day window in that record was reduced to the
same features the predictor computes at runtime, using the same window length:

- peak daily rainfall in the window (mm)
- mean daily maximum temperature (°C)
- mean daily minimum temperature (°C)

That gives ~3,639 windows per county, ~18,200 in total. The published cutoffs
were then applied to each window.

Reproduce with `go test ./internal/predict -run Climatology`, or rebuild the
reference data itself from the archive API (free, no key) — the derived table
lives at `internal/predict/climatologydata/kenya-5county-2015-2024.json` and
carries its own provenance and licence fields.

## Result

How often each published HIGH cutoff is reached, by county:

| County | Cholera ≥60 mm | Malaria ≥40 mm | Pneumonia ≤16 °C | Meningitis ≥39 °C |
|---|---|---|---|---|
| Nairobi | 1.2% | 3.0% | **0.0%** | **0.0%** |
| Kisumu | 0.8% | 1.9% | **0.0%** | **0.0%** |
| Mombasa | 4.4% | 7.2% | **0.0%** | **0.0%** |
| Nakuru | 0.0% | 0.4% | **0.0%** | **0.0%** |
| Eldoret | 0.0% | 2.3% | **0.0%** | **0.0%** |

The temperature rules are not just unfired in the sample — they are out of
range by a wide margin:

- **Pneumonia** requires a 14-day **mean** daily maximum ≤ 16 °C. The coldest
  **single day** maximum anywhere in the ten-year record was **16.3 °C**. A
  mean of fourteen such days can never be lower than the coldest single day,
  so the cutoff is unreachable by construction.
- **Meningitis** requires a 14-day mean daily maximum ≥ 39 °C. The hottest
  single day maximum in the record was **36.2 °C**.

The coldest and hottest 14-day means actually observed were **19.9 °C**
(Eldoret) and **35.2 °C** (Mombasa).

## Interpretation

The rainfall rules behave sensibly. Cholera and malaria fire at plausible
rates, and the coastal/inland difference is what you would expect.

The temperature rules look transplanted from a different climate:

- **Meningitis ≥ 39 °C** matches the African meningitis belt — the Sahel,
  roughly Senegal to Ethiopia, where the hot dry season genuinely reaches
  those temperatures. The Kenyan highlands and coast do not.
- **Pneumonia ≤ 16 °C** is measuring the wrong variable rather than the wrong
  number. Cold stress in infants is an overnight exposure, and Kenyan daily
  *maxima* stay mild even where nights are cold. Measured on daily
  **minimum** temperature, the signal is real and large: Eldoret, Nairobi and
  Nakuru sit at 10–12 °C for the 14-day mean minimum, cold enough to matter
  for a small child, while Mombasa never drops below about 22 °C.

## What was changed, and what was not

**Unchanged.** `internal/predict/rules.go` still encodes the published
thresholds exactly, and the rules predictor remains the default. Changing a
contractual number in code would be the wrong way to resolve this.

**Added.** A second predictor (`PREDICTOR=climatology`) scores each window
against that county's own distribution for that calendar month, rather than
against a single global cutoff. A percentile is defined in every climate, so
it does not silently die when a threshold does not transfer. For cold stress
it uses the daily minimum, for the reason above.

**Surfaced.** The finding is not buried:

- `TestPublishedTemperatureThresholdsAreUnreachableInReferenceDecade` fails if
  it ever stops being true.
- `GET /v1/model` reports `reachable_in_reference_period` per rule, with a
  note explaining any that cannot fire.
- The dashboard's Model view shows it in the threshold table.

## Recommendation

1. **Amend the pneumonia rule to use minimum temperature.** This is the
   substantive change and the one with real public-health value. Suggested
   starting point for discussion, pending clinical input: HIGH at a 14-day
   mean minimum ≤ 11 °C, MEDIUM ≤ 13 °C, which fires in the highland counties
   in the cold season and never on the coast.
2. **Either re-scope or drop the meningitis rule.** At ≥ 39 °C it is inert in
   these five counties. It becomes meaningful only if the programme extends to
   arid northern counties (Turkana, Marsabit, Mandera), and even there the
   cutoff should be re-derived rather than assumed.
3. **Keep the rainfall thresholds for now**, but revisit them against outbreak
   surveillance data when any exists. Their firing rates are plausible, which
   is not the same as being correct.

## Limitations

- ERA5 reanalysis at a single county centroid is not a station observation and
  smooths local extremes; a county is not climatically uniform.
- Ten years is short for tail behaviour. A cutoff unreached in this record is
  not proven impossible, though a 14-day mean below the coldest observed
  single day is impossible regardless of record length.
- **None of this validates any threshold against disease outcomes.** It shows
  only whether a cutoff is reachable in the climate record. Whether the
  reachable ones actually predict outbreaks is unknown and untested, and this
  project makes no claim that they do.
