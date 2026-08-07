#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Structural contract checks. These guard commitments from the funding
# agreement; CI and `make verify` both run them. Do not weaken silently.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0

# ---------------------------------------------------------------------------
# 1. SPDX headers on all first-party source files.
#    Generated output, the preserved Python prototype, docs and fixtures are
#    exempt (generators own their headers; reference/ is historical).
# ---------------------------------------------------------------------------
while IFS= read -r f; do
  if ! head -n 3 "$f" | grep -q "SPDX-License-Identifier: Apache-2.0"; then
    echo "SPDX header missing: $f"
    fail=1
  fi
done < <(
  git ls-files '*.go' '*.ts' '*.tsx' '*.proto' '*.sql' \
    | grep -v -E '^(internal/gen/|internal/store/db/|web/src/gen/|reference/|testdata/)'
)

# ---------------------------------------------------------------------------
# 2. Ledger key isolation: the sealed schema (per-child HMAC keys) may only be
#    referenced from the ledger's query file. A hit anywhere else means child
#    keys are leaking into another service's reach.
# ---------------------------------------------------------------------------
while IFS= read -r f; do
  case "$f" in
    internal/store/queries/ledger.sql) ;; # the one allowed site
    internal/store/db/ledger.sql.go)   ;; # sqlc output generated from it
    internal/store/migrations/*)       ;; # schema definition itself
    scripts/contract-checks.sh)        ;;
    *)
      echo "forbidden reference to sealed. schema in: $f"
      fail=1
      ;;
  esac
done < <(git grep -l 'sealed\.' -- '*.go' '*.sql' 2>/dev/null || true)

# ---------------------------------------------------------------------------
# 3. Contract tests must exist once their packages exist. Deleting or renaming
#    them fails this check (and CI runs them by name as well).
# ---------------------------------------------------------------------------
if [ -d internal/publicapi ]; then
  if ! grep -rq "func TestContract_PIILeak" internal/publicapi/; then
    echo "contract test missing: TestContract_PIILeak in internal/publicapi"
    fail=1
  fi
  if ! grep -rq "func TestContract_KAnonymity" internal/publicapi/; then
    echo "contract test missing: TestContract_KAnonymity in internal/publicapi"
    fail=1
  fi
fi

if [ "$fail" -ne 0 ]; then
  echo "contract-checks: FAIL"
  exit 1
fi
echo "contract-checks: OK"
