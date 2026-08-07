# Reference: Python prototype (historical)

This directory preserves the original Python prototype of ClimateShield AI for
provenance. It is **not built, not tested, and not part of the running system**.
The Go services under `cmd/` and `internal/` supersede it.

Two things in here are canonical and were carried into the Go implementation:

- **Risk thresholds** in `climate-engine/ingest.py` (cholera ≥60/≥30 mm peak
  rainfall, malaria ≥40/≥20 mm, pneumonia ≤16/≤19 °C mean max temp,
  meningitis ≥39/≥36 °C). The single source of truth is now
  `internal/predict/rules.go`.
- **The demo scenario** (Kisumu long-rains outbreak window) in the same file,
  now encoded as Open-Meteo-shaped golden fixtures under `testdata/golden/`.

Known defect, fixed in the rewrite: the prototype printed
`"SMS sent to under-vaccinated families"` and
`"dispatched via Africa's Talking SMS gateway"` without sending anything.
The Go notifier's mock channel prints `[mock] would send N alerts` instead —
output never implies an action that was not taken.
