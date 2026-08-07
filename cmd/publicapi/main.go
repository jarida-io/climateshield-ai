// SPDX-License-Identifier: Apache-2.0

// Command publicapi runs the public read-only API tier.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jarida-io/climateshield/internal/publicapi"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := publicapi.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "publicapi: %v\n", err)
		os.Exit(1)
	}
}
