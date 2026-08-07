// SPDX-License-Identifier: Apache-2.0

// Command covergate enforces the repository's coverage threshold from a Go
// coverage profile. It exists because Codecov (paid SaaS) is a forbidden
// dependency; the gate must be first-party and auditable.
//
// Policy (documented in CLAUDE.md and README): total statement coverage over
// ./internal/... must be >= the threshold. Generated code is excluded:
//
//	internal/gen       (protoc-gen-go / protoc-gen-connect-go output)
//	internal/store/db  (sqlc output)
//
// Nothing else is excluded. cmd/ mains are thin (delegate to internal
// run functions) and are outside -coverpkg entirely.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var excludedPathParts = []string{
	"/internal/gen/",
	"/internal/store/db/",
}

func main() {
	profile := flag.String("profile", "coverage.out", "coverage profile path")
	threshold := flag.Float64("threshold", 80, "minimum percent covered")
	flag.Parse()

	covered, total, err := tally(*profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "covergate: %v\n", err)
		os.Exit(2)
	}
	if total == 0 {
		fmt.Fprintln(os.Stderr, "covergate: profile contains no statements")
		os.Exit(2)
	}
	pct := 100 * float64(covered) / float64(total)
	if pct < *threshold {
		fmt.Printf("covergate: FAIL — coverage %.1f%% < %.0f%% (%d/%d statements)\n", pct, *threshold, covered, total)
		os.Exit(1)
	}
	fmt.Printf("covergate: OK — coverage %.1f%% >= %.0f%% (%d/%d statements)\n", pct, *threshold, covered, total)
}

// block identifies one coverage block: file plus source span.
type block struct {
	location string // "pkg/file.go:startLine.startCol,endLine.endCol"
	stmts    int64
}

// tally merges the profile the way `go tool cover` does. With -coverpkg,
// EVERY test binary reports every instrumented block, so the same block
// appears many times — once per package under test. Summing those repeats
// would inflate the denominator and count a block as uncovered in each
// binary that did not exercise it. Merging by block and keeping the highest
// hit count yields the true statement coverage.
func tally(path string) (covered, total int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = f.Close() }() // read-only file; close error carries no signal

	hitsByBlock := map[block]int64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "mode:") || line == "" {
			continue
		}
		if excluded(line) {
			continue
		}
		// Format: name.go:startLine.startCol,endLine.endCol numStmts hitCount
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return 0, 0, fmt.Errorf("malformed profile line: %q", line)
		}
		stmts, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("malformed statement count in %q: %w", line, err)
		}
		hits, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("malformed hit count in %q: %w", line, err)
		}
		b := block{location: fields[0], stmts: stmts}
		// Insert unconditionally so blocks that are never hit by any binary
		// still land in the denominator, then keep the highest count seen.
		if prev, seen := hitsByBlock[b]; !seen || hits > prev {
			hitsByBlock[b] = hits
		}
	}
	if err := sc.Err(); err != nil {
		return 0, 0, err
	}

	for b, hits := range hitsByBlock {
		total += b.stmts
		if hits > 0 {
			covered += b.stmts
		}
	}
	return covered, total, nil
}

func excluded(line string) bool {
	for _, part := range excludedPathParts {
		if strings.Contains(line, part) {
			return true
		}
	}
	return false
}
