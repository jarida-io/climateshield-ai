// SPDX-License-Identifier: Apache-2.0

// Command briefing runs the county risk briefing service.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jarida-io/climateshield/internal/briefing"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := briefing.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "briefing: %v\n", err)
		os.Exit(1)
	}
}
