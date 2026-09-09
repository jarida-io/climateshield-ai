// SPDX-License-Identifier: Apache-2.0

package evmtest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Image is the foundry release the stack runs, pinned by digest. It is the
// same string docker-compose.yml uses; a test asserts they do not drift.
const Image = "ghcr.io/foundry-rs/foundry:v1.5.1@sha256:3a70bfa9bd2c732a767bb60d12c8770b40e8f9b6cca28efc4b12b1be81c7f28e"

// ChainID is anvil's default development chain id.
const ChainID int64 = 31337

// Anvil starts a throwaway development chain in a container and returns its
// JSON-RPC URL. The container is terminated when the test ends. Skipped
// under -short like every other container-backed test.
func Anvil(t testing.TB) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: requires local Docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// The image's entrypoint is `/bin/sh -c`, so the command is one string.
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        Image,
			ExposedPorts: []string{"8545/tcp"},
			Cmd:          []string{fmt.Sprintf("exec anvil --host 0.0.0.0 --port 8545 --chain-id %d", ChainID)},
			WaitingFor:   wait.ForLog("Listening on").WithStartupTimeout(90 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err, "anvil container failed to start")
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })

	host, err := c.Host(ctx)
	require.NoError(t, err)
	port, err := c.MappedPort(ctx, "8545/tcp")
	require.NoError(t, err)
	return fmt.Sprintf("http://%s:%s", host, port.Port())
}
