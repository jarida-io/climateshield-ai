// SPDX-License-Identifier: Apache-2.0

package seed_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/seed"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

func TestAreasSeededByHarness(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)

	areas, err := q.ListAreas(context.Background())
	require.NoError(t, err)
	require.Len(t, areas, 5)

	byName := map[string][2]float64{}
	for _, a := range areas {
		byName[a.Name] = [2]float64{a.Latitude, a.Longitude}
	}
	require.InDelta(t, -0.1022, byName["Kisumu"][0], 1e-9)
	require.InDelta(t, 34.7617, byName["Kisumu"][1], 1e-9)
	require.InDelta(t, -1.2921, byName["Nairobi"][0], 1e-9)
}

func TestDemoPopulationShape(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	key, err := crypto.NewRandomKey()
	require.NoError(t, err)

	sum, err := seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)

	// The by-area counts are load-bearing: Kisumu and Eldoret must clear the
	// k>=10 threshold, Mombasa and Nakuru must sit under it.
	require.Equal(t, 12, sum.ChildrenByArea["kisumu"])
	require.Equal(t, 11, sum.ChildrenByArea["eldoret"])
	require.Equal(t, 3, sum.ChildrenByArea["mombasa"])
	require.Equal(t, 2, sum.ChildrenByArea["nakuru"])
	require.Zero(t, sum.ChildrenByArea["nairobi"])
	require.Equal(t, 28, sum.Children)
	require.Equal(t, 1, sum.OptedOutGuardians)
	require.Positive(t, sum.Events)

	// Consent: exactly one guardian's latest action is OPT_OUT.
	rows, err := pool.Query(ctx, `
		SELECT count(*) FROM (
			SELECT DISTINCT ON (guardian_id) action
			FROM consent_log ORDER BY guardian_id, occurred_at DESC, id DESC
		) latest WHERE action = 'OPT_OUT'`)
	require.NoError(t, err)
	defer rows.Close()
	require.True(t, rows.Next())
	var optedOut int
	require.NoError(t, rows.Scan(&optedOut))
	require.Equal(t, 1, optedOut)

	// PII spot check: stored bytes are ciphertext, decryptable with the key.
	children, err := db.New(pool).ListChildren(ctx)
	require.NoError(t, err)
	require.Len(t, children, 28)
	name, err := crypto.FromBytes[string](children[0].NameEnc).Open(key)
	require.NoError(t, err)
	require.NotEmpty(t, name)
	require.NotContains(t, string(children[0].NameEnc), name[:3],
		"child name must not appear in plaintext in the stored blob")
}
