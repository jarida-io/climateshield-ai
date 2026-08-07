<!-- SPDX-License-Identifier: Apache-2.0 -->

# Contributing to ClimateShield

ClimateShield is an open-source climate-health early warning system for
Kenya. Contributions are welcome across backend, data, frontend and
documentation. Read [CLAUDE.md](CLAUDE.md) first — it records the constraints
this project is bound by, several of which come from a funding agreement rather
than from taste.

## Development setup

Prerequisites: **Go 1.23+**, **Docker** (for the database and tests), and
**Node 20+** (for the dashboard only). Nothing else needs installing — `buf`,
`sqlc` and the protobuf plugins are pinned as `go.mod` tool dependencies, and
`golangci-lint` is fetched into `./bin` by the Makefile at a pinned version.

```bash
git clone https://github.com/jarida-io/climateshield.git
cd climateshield
cp .env.example .env
make up      # full stack, healthy in under a minute
make demo    # end-to-end pipeline against committed fixtures
make verify  # everything CI runs
```

`make verify` must pass before you open a pull request. It runs formatting,
`go vet`, `golangci-lint`, the build, all tests, the coverage gate, `buf lint`,
the contract checks, `tsc --noEmit` and the web build.

## Non-negotiable rules

These fail CI, and in most cases they exist because breaking them would breach
the funding agreement:

1. **Dependencies must be open source and free**, and the system must build,
   test and run with **zero credentials**. CLAUDE.md lists specific forbidden
   packages (TimescaleDB, Redis 7.4+, Mapbox GL v2+, Fiber, GORM, Codecov and
   others) with the approved alternative for each. If you think you need one,
   open an issue rather than substituting silently.
2. **No output may imply an action that did not happen.** If a mock adapter is
   active, output says so (`[mock] would send N alerts`). No fabricated
   benchmarks, accuracy figures or performance claims anywhere — including in
   comments and documentation.
3. **No personal data on any public surface**, and no per-child hash. Counts
   derived from people are k≥10 suppressed.
4. **Never log PII.** Use the typed wrappers in
   `internal/platform/logging`.
5. **Coverage must stay ≥80%** over `./internal/...`, excluding generated code.
6. **Every first-party source file carries** `SPDX-License-Identifier: Apache-2.0`.
7. **Do not delete or rename the contract tests** (`TestContract_PIILeak`,
   `TestContract_KAnonymity`). CI runs them by name.
8. **Risk thresholds live only in `internal/predict/rules.go`.** They are
   published in the funding proposal; changing them requires a proposal
   amendment, not a pull request.

## Coding standards

- Standard Go style: `gofmt`, `go vet` and `golangci-lint` clean.
- **Test-first for pure logic** — thresholds, Merkle trees, canonical
  serialization, template rendering, suppression. These are the parts a
  reviewer must be able to trust by reading a test.
- **No test may access the network.** Use committed fixtures and `httptest`.
  Database tests use testcontainers via `internal/store/testdb`.
- SQL is written by hand in `internal/store/queries` and compiled by sqlc; run
  `make generate` after changing queries or the protobuf contract, and commit
  the generated output.
- Keep `cmd/*/main.go` thin — configuration and signal handling only,
  delegating to `internal/<service>.Run`.

## Pull requests

- Use [Conventional Commits](https://www.conventionalcommits.org/):
  `feat(publicapi): …`, `fix(ledger): …`, `docs: …`, `ci: …`.
- Explain **why** in the commit body, not just what changed.
- One logical change per pull request; keep generated-code updates in the same
  commit as the source change that caused them.
- Paste your `make verify` output in the description.
- Update [NOTES.md](NOTES.md) if you change what is real versus stubbed.

## Reporting issues

Use the issue templates in `.github/ISSUE_TEMPLATE`. For anything with security
or privacy implications — especially a suspected data leak on a public surface
— email **hello@jarida.io** instead of filing a public issue.

## Community

Be respectful and constructive. This project handles data about children;
assume every design discussion carries that weight.

## Licence

Contributions are licensed under the [Apache License 2.0](LICENSE).
