// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type testCfg struct {
	Common
	DB
	Extra string `env:"CONFIG_TEST_EXTRA" envDefault:"fallback"`
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load[testCfg]()
	require.NoError(t, err)
	require.Equal(t, "info", cfg.LogLevel)
	require.Contains(t, cfg.URL, "postgres://")
	require.Equal(t, "fallback", cfg.Extra)
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("DATABASE_URL", "postgres://u:p@example.internal:5432/x")
	t.Setenv("CONFIG_TEST_EXTRA", "set")

	cfg, err := Load[testCfg]()
	require.NoError(t, err)
	require.Equal(t, "debug", cfg.LogLevel)
	require.Equal(t, "postgres://u:p@example.internal:5432/x", cfg.URL)
	require.Equal(t, "set", cfg.Extra)
}
