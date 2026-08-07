// SPDX-License-Identifier: Apache-2.0

// Command ingestor runs the climate ingestion service.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jarida-io/climateshield/internal/climate/ingestor"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := ingestor.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ingestor: %v\n", err)
		os.Exit(1)
	}
}
