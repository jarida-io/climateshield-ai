// SPDX-License-Identifier: Apache-2.0

// Command predictor runs the risk scoring service.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jarida-io/climateshield/internal/predict"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := predict.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "predictor: %v\n", err)
		os.Exit(1)
	}
}
