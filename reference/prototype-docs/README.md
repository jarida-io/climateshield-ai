# Prototype artefacts — superseded, kept for provenance

These diagrams and wireframes describe the **original Python/PHP prototype**,
not the system in this repository. They are preserved because they show where
the project started and what was promised; they are **not** documentation of
anything that runs today. For the current architecture see
[`docs/architecture.md`](../../docs/architecture.md).

Read them with these corrections in mind.

- **The calibration claim was never substantiated.** `diagrams/risk-scoring.md`
  says the thresholds are "calibrated against historical KEPI surveillance
  reports". No such calibration exists in this repository, and no surveillance
  data has ever been in it. The four cutoffs are the ones published in the
  funding proposal; when they were finally checked against a decade of
  reanalysis weather, **two of them turned out to be unreachable in the
  monitored counties**. That check, and what was done about it, is in
  [`docs/threshold-validation.md`](../../docs/threshold-validation.md).
- **The delivery claims were never substantiated.** `diagrams/data-flow.md` and
  `diagrams/parent-alert-journey.md` show messages going out through Africa's
  Talking, and quote a "risk score → SMS dispatched: < 8 seconds (sandbox)"
  figure. The prototype printed `"SMS sent to under-vaccinated families"` while
  sending nothing at all. In the current system the default channel is a mock
  that records `would_send`, never `sent`, and every surface says so. No
  latency figure of any kind is claimed anywhere in the current system.
- **The message content contradicts the current templates.** The example SMS
  names the disease ("High cholera risk") beside a named child. The current
  templates say "outbreak risk" instead: a named disease next to a named child
  on a plaintext SMS is diagnosis-adjacent, and a test now asserts that no
  disease name can appear in a message body.
- **The stack is different.** MySQL, PHP, Apache and session auth were replaced
  by Go services, PostgreSQL/PostGIS, ConnectRPC and a React dashboard. The
  "ML Predictor — outbreak probability" box in `diagrams/system-architecture.md`
  describes something that does not exist: nothing in this repository estimates
  an outbreak probability, and the ONNX predictor is a stub that fails startup
  rather than silently falling back.
- **The wireframes were never built as drawn.** `wireframes/` shows a guardian
  login, a child schedule screen and a USSD flow. The current dashboard is a
  public, read-only, aggregate-only surface with no guardian login and no
  per-child screen, because per-child data must not appear on a public surface
  at all.

The one thing in here that remains canonical is the *shape* of the demo
scenario — a Kisumu long-rains window — which now lives as committed fixtures
under `testdata/golden/`.
