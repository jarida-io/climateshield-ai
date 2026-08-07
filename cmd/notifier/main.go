// SPDX-License-Identifier: Apache-2.0

// Command notifier runs the alert dispatch service.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jarida-io/climateshield/internal/notify/notifier"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := notifier.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "notifier: %v\n", err)
		os.Exit(1)
	}
}
