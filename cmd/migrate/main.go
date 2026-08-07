// SPDX-License-Identifier: Apache-2.0

// Command migrate applies (or rolls back) the database schema: app
// migrations, River's job tables, and the reference areas. It is the single
// migration path shared by docker-compose, `make migrate`, and the test
// harness.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/jarida-io/climateshield/internal/platform/config"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/store"
	"github.com/jarida-io/climateshield/internal/store/seed"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load[struct {
		config.Common
		config.DB
	}]()
	if err != nil {
		return err
	}
	log := logging.New(os.Stdout, cfg.LogLevel)

	direction := "up"
	if len(os.Args) > 1 {
		direction = os.Args[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	switch direction {
	case "up":
		if err := store.MigrateUp(cfg.URL); err != nil {
			return err
		}
		pool, err := store.Connect(ctx, cfg.URL)
		if err != nil {
			return err
		}
		defer pool.Close()

		riverMig, err := rivermigrate.New(riverpgxv5.New(pool), nil)
		if err != nil {
			return err
		}
		if _, err := riverMig.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
			return fmt.Errorf("river migrations: %w", err)
		}
		if err := seed.Areas(ctx, pool); err != nil {
			return err
		}
		log.Info("migrations applied", "direction", "up", "areas_seeded", len(seed.Counties))
		return nil
	case "down":
		if err := store.MigrateDown(cfg.URL); err != nil {
			return err
		}
		log.Info("migrations rolled back", "direction", "down")
		return nil
	default:
		return fmt.Errorf("unknown direction %q (want up or down)", direction)
	}
}
