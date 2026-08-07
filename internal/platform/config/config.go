// SPDX-License-Identifier: Apache-2.0

// Package config loads service configuration from environment variables.
// Every value has a working development default or is validated at startup;
// the system must run end to end with zero credentials.
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Load parses environment variables into a config struct of type T.
func Load[T any]() (T, error) {
	var cfg T
	if err := env.Parse(&cfg); err != nil {
		return cfg, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

// Common holds settings shared by every service.
type Common struct {
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

// DB holds database connection settings.
type DB struct {
	// URL is the Postgres DSN. Default targets the docker-compose database.
	URL string `env:"DATABASE_URL" envDefault:"postgres://climateshield:climateshield@localhost:5432/climateshield?sslmode=disable"`
}
