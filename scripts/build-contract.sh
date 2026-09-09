#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Compiles internal/ledger/anchor/evm/contract/RootAnchor.sol ONCE with a
# pinned solc image and writes the artifacts next to it. The artifacts are
# committed; nothing at build, test or run time compiles Solidity, and
# `make up` never touches this script. A test asserts that the committed
# bytecode still hashes to what BUILD.txt records, so an edit to the .sol
# without a recompile fails the suite instead of shipping stale bytecode.
#
# Usage: make contract          (from the repository root)
set -euo pipefail
cd "$(dirname "$0")/.."

# Pinned by digest. The image is published for linux/amd64 only; on an arm64
# host Docker runs it under emulation, which is fine for a one-off compile.
SOLC_IMAGE="ghcr.io/argotorg/solc:0.8.30@sha256:b116bf835554d40c501feab0b2c943a8c5eec003b804bcc5b326b85c93da00c2"

# paris: no PUSH0, MCOPY or transient storage in the output, so the bytecode
# runs on any post-merge EVM. --metadata-hash none keeps the runtime code
# free of a source-path-dependent hash, so eth_getCode of a deployed instance
# equals RootAnchor.bin-runtime byte for byte — that is what the ledger
# service checks before it trusts a contract address.
SOLC_FLAGS="--optimize --optimize-runs 200 --evm-version paris --metadata-hash none"

CONTRACT_DIR="internal/ledger/anchor/evm/contract"
OUT_DIR="$CONTRACT_DIR/build"
mkdir -p "$OUT_DIR"

docker run --rm --platform linux/amd64 \
  -v "$(pwd)/$CONTRACT_DIR:/c" -w /c \
  "$SOLC_IMAGE" \
  $SOLC_FLAGS --abi --bin --bin-runtime --hashes --overwrite -o /c/build RootAnchor.sol

SOLC_VERSION="$(docker run --rm --platform linux/amd64 "$SOLC_IMAGE" --version | grep '^Version:' | sed 's/^Version: //')"

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

{
  echo "# RootAnchor build record — written by scripts/build-contract.sh (make contract)."
  echo "# internal/ledger/anchor/evm/contract_test.go asserts the committed artifacts"
  echo "# still hash to these values. Rebuild rather than editing this file."
  echo "solc_version=$SOLC_VERSION"
  echo "solc_image=$SOLC_IMAGE"
  echo "solc_flags=$SOLC_FLAGS"
  echo "sha256 RootAnchor.sol $(sha256 "$CONTRACT_DIR/RootAnchor.sol")"
  echo "sha256 build/RootAnchor.abi $(sha256 "$OUT_DIR/RootAnchor.abi")"
  echo "sha256 build/RootAnchor.bin $(sha256 "$OUT_DIR/RootAnchor.bin")"
  echo "sha256 build/RootAnchor.bin-runtime $(sha256 "$OUT_DIR/RootAnchor.bin-runtime")"
  echo "sha256 build/RootAnchor.signatures $(sha256 "$OUT_DIR/RootAnchor.signatures")"
} > "$CONTRACT_DIR/BUILD.txt"

echo "contract: compiled with $SOLC_VERSION"
cat "$CONTRACT_DIR/BUILD.txt"
