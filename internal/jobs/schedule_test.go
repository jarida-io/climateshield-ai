// SPDX-License-Identifier: Apache-2.0

package jobs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func TestScheduleFiresImmediatelyThenOnTick(t *testing.T) {
	var calls atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go Schedule(ctx, discardLogger(), "test", 20*time.Millisecond, func(context.Context) error {
		calls.Add(1)
		return nil
	})

	// The first fire is immediate — a restarted service must not wait a full
	// interval before ingesting.
	require.Eventually(t, func() bool { return calls.Load() >= 1 },
		2*time.Second, 5*time.Millisecond, "schedule did not fire on start")
	require.Eventually(t, func() bool { return calls.Load() >= 3 },
		2*time.Second, 5*time.Millisecond, "schedule did not keep ticking")
}

func TestScheduleStopsOnContextCancel(t *testing.T) {
	var calls atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		Schedule(ctx, discardLogger(), "test", 10*time.Millisecond, func(context.Context) error {
			calls.Add(1)
			return nil
		})
		close(done)
	}()

	require.Eventually(t, func() bool { return calls.Load() >= 2 }, 2*time.Second, 5*time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Schedule did not return after cancel")
	}
	settled := calls.Load()
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, settled, calls.Load(), "schedule kept firing after cancel")
}

func TestScheduleLogsInsertFailuresAndKeepsGoing(t *testing.T) {
	// A transient database blip must not silently kill the schedule.
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	var calls atomic.Int64

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Schedule(ctx, log, "climate_ingest", 10*time.Millisecond, func(context.Context) error {
		calls.Add(1)
		return errors.New("database unavailable")
	})

	require.Eventually(t, func() bool { return calls.Load() >= 3 },
		2*time.Second, 5*time.Millisecond, "schedule stopped after an error")
	require.Contains(t, buf.String(), "failed to schedule job")
	require.Contains(t, buf.String(), "climate_ingest")
}
