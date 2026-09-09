// SPDX-License-Identifier: Apache-2.0

// Command buildclimatology builds the reference climatology that
// internal/predict embeds — the file named in its own generated_by field.
//
// It is a developer tool, not a service. Nothing in the running system calls
// it, no test exercises its network path, and `make up`, `make demo` and CI
// never run it. It exists so that the numbers the model is measured against
// can be rebuilt from their source instead of taken on trust:
//
//	go run ./cmd/buildclimatology -out internal/predict/climatologydata/kenya-5county-2015-2024.json
//
// It contacts archive-api.open-meteo.com (free, keyless, CC BY 4.0) and is
// the only way the reference data is ever fetched: no test in this repository
// makes that request, and neither does any service. (The live climate
// ingestor and `make demo-live` reach Open-Meteo's FORECAST API for a
// different purpose; nothing else here reads the archive.) It prints the
// SHA-256 of what it wrote, which is the digest GET /v1/model publishes for
// the embedded file.
//
//	go run ./cmd/buildclimatology -digest -out <file>
//
// prints the digest of an existing file and makes no request at all.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/jarida-io/climateshield/internal/store/seed"
)

// Defaults describing the committed artifact.
const (
	defaultOut     = "internal/predict/climatologydata/kenya-5county-2015-2024.json"
	defaultFrom    = "2015-01-01"
	defaultTo      = "2024-12-31"
	defaultBase    = "https://archive-api.open-meteo.com"
	defaultWindow  = 14
	generatorName  = "cmd/buildclimatology"
	sourceLabel    = "Open-Meteo historical archive (ERA5 reanalysis), archive-api.open-meteo.com"
	licenceLabel   = "Open-Meteo data CC BY 4.0"
	defaultTimeout = 2 * time.Minute
)

type options struct {
	out        string
	from       string
	to         string
	base       string
	windowDays int
	timeout    time.Duration
	digestOnly bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "buildclimatology: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	opts, err := parseFlags(args, out)
	if err != nil {
		return err
	}
	if opts.digestOnly {
		return printDigest(opts.out, out)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return build(ctx, opts, out)
}

func parseFlags(args []string, out io.Writer) (options, error) {
	var opts options
	fs := flag.NewFlagSet("buildclimatology", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&opts.out, "out", defaultOut, "artifact to write")
	fs.StringVar(&opts.from, "from", defaultFrom, "first day of the reference period (YYYY-MM-DD)")
	fs.StringVar(&opts.to, "to", defaultTo, "last day of the reference period (YYYY-MM-DD)")
	fs.StringVar(&opts.base, "base", defaultBase, "archive API origin")
	fs.IntVar(&opts.windowDays, "window", defaultWindow, "window length in days")
	fs.DurationVar(&opts.timeout, "timeout", defaultTimeout, "per-request timeout")
	fs.BoolVar(&opts.digestOnly, "digest", false, "print the SHA-256 of -out and exit (no network request)")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.windowDays <= 1 {
		return opts, fmt.Errorf("-window must be at least 2 days, got %d", opts.windowDays)
	}
	for _, d := range []string{opts.from, opts.to} {
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return opts, fmt.Errorf("dates must be YYYY-MM-DD: %w", err)
		}
	}
	return opts, nil
}

// build fetches every county's record and writes the artifact.
func build(ctx context.Context, opts options, out io.Writer) error {
	client := newArchiveClient(opts.base, opts.timeout)
	period := opts.from + ".." + opts.to

	say(out, "requesting %d counties from %s for %s\n", len(seed.Counties), opts.base, period)
	say(out, "no credentials are used, and no test in this repository makes this request\n")

	records := make([]countyRecord, 0, len(seed.Counties))
	for _, county := range seed.Counties {
		days, err := client.daily(ctx, county, opts.from, opts.to)
		if err != nil {
			return err
		}
		say(out, "  %-8s %d daily records\n", county.ID, len(days))
		records = append(records, countyRecord{ID: county.ID, Days: days})
	}

	clim, err := buildClimatology(records, opts.windowDays, period, sourceLabel, licenceLabel, generatorName)
	if err != nil {
		return err
	}
	encoded := encodeClimatology(clim)
	if err := os.WriteFile(opts.out, encoded, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", opts.out, err)
	}

	sum := sha256.Sum256(encoded)
	say(out, "wrote %s\n", opts.out)
	say(out, "  %d counties, %d %d-day windows, %d quantile steps per distribution\n",
		len(clim.Counties), clim.TotalSamples(), clim.WindowDays, clim.QuantileSteps())
	say(out, "  sha256 %s\n", hex.EncodeToString(sum[:]))
	say(out, "compare that digest with reference_sha256 on GET /v1/model after rebuilding.\n")
	return nil
}

// printDigest hashes an existing artifact without touching the network, so
// the digest published by the API can be checked offline.
func printDigest(path string, out io.Writer) error {
	raw, err := os.ReadFile(path) // #nosec G304 -- developer tool, path is an explicit flag
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	say(out, "%s  %s\n", hex.EncodeToString(sum[:]), path)
	return nil
}

// say writes progress to the tool's own output. A failed write to a console
// is not worth failing a rebuild over, so the error is dropped deliberately
// rather than by omission.
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
