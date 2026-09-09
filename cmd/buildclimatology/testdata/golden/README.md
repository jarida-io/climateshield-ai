<!-- SPDX-License-Identifier: Apache-2.0 -->

# Golden archive responses — SYNTHETIC, not weather

These five files have the wire shape of an Open-Meteo `/v1/archive` daily
response and are served by `httptest` so the generator's HTTP path can be
tested without a network. **The numbers in them are made up.** They are not
ERA5 reanalysis, not measurements of anything, and must never be quoted as
Kenyan climate. The county names and coordinates are real only so the test
can route a request to the right file.

Every value is a fixed arithmetic function of the day index `i` (0-based from
2015-01-01) and the county index `c` (nairobi 0, kisumu 1, mombasa 2,
nakuru 3, eldoret 4), rounded to one decimal:

    precipitation_sum   = ((i*7  + c*13) mod 50) / 10      →  0.0 … 4.9 mm
    temperature_2m_max  = 20 + ((i*3  + c*5)  mod 120) / 10 → 20.0 … 31.9 °C
    temperature_2m_min  = 10 + ((i*11 + c*7)  mod 80)  / 10 → 10.0 … 17.9 °C

Range: 2015-01-01 … 2015-03-15, 74 days. With the 14-day window that is 60
windows per county: 31 starting in January, 28 in February, 1 in March.

The real reference artifact is
`internal/predict/climatologydata/kenya-5county-2015-2024.json`, built from
the live archive by a person running `make climatology`. Its own shape is
checked against this generator's arithmetic in `build_test.go`.
