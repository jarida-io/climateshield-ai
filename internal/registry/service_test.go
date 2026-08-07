// SPDX-License-Identifier: Apache-2.0

package registry_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/registry"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/seed"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

func uuidStr(t *testing.T, u pgtype.UUID) string {
	t.Helper()
	v, err := u.Value()
	require.NoError(t, err)
	s, ok := v.(string)
	require.True(t, ok)
	return s
}

func TestRecordImmunizationHappyPath(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	key, err := crypto.NewRandomKey()
	require.NoError(t, err)
	_, err = seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)

	q := db.New(pool)
	children, err := q.ListChildren(ctx)
	require.NoError(t, err)
	child := children[0]

	svc := registry.NewService(pool)
	resp, err := svc.RecordImmunization(ctx, connect.NewRequest(&climateshieldv1.RecordImmunizationRequest{
		ChildId:        uuidStr(t, child.ID),
		VaccineCode:    "opv3",
		AdministeredAt: timestamppb.Now(),
		Facility:       "Kisumu County Hospital",
	}))
	require.NoError(t, err)
	require.NotEmpty(t, resp.Msg.GetEventId())

	events, err := q.ListEventsForChild(ctx, child.ID)
	require.NoError(t, err)
	var found bool
	for _, e := range events {
		if e.VaccineCode == "opv3" {
			found = true
		}
	}
	require.True(t, found)
}

func TestRecordImmunizationValidation(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	svc := registry.NewService(pool)

	_, err := svc.RecordImmunization(ctx, connect.NewRequest(&climateshieldv1.RecordImmunizationRequest{}))
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = svc.RecordImmunization(ctx, connect.NewRequest(&climateshieldv1.RecordImmunizationRequest{
		ChildId: "not-a-uuid", VaccineCode: "bcg", AdministeredAt: timestamppb.Now(),
	}))
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = svc.RecordImmunization(ctx, connect.NewRequest(&climateshieldv1.RecordImmunizationRequest{
		ChildId: "00000000-0000-0000-0000-000000000001", VaccineCode: "bcg", AdministeredAt: timestamppb.Now(),
	}))
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestRecordImmunizationUnknownVaccine(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	key, err := crypto.NewRandomKey()
	require.NoError(t, err)
	_, err = seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)

	children, err := db.New(pool).ListChildren(ctx)
	require.NoError(t, err)

	svc := registry.NewService(pool)
	_, err = svc.RecordImmunization(ctx, connect.NewRequest(&climateshieldv1.RecordImmunizationRequest{
		ChildId:        uuidStr(t, children[0].ID),
		VaccineCode:    "smallpox", // not in KEPI
		AdministeredAt: timestamppb.Now(),
	}))
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestGetDueSummaryMatchesSeedExpectations(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	key, err := crypto.NewRandomKey()
	require.NoError(t, err)
	sum, err := seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)

	svc := registry.NewService(pool)
	resp, err := svc.GetDueSummary(ctx, connect.NewRequest(&climateshieldv1.GetDueSummaryRequest{}))
	require.NoError(t, err)

	// Every 150-day-old missed the 14-week visit: opv3, dpt3, pcv3, ipv due
	// and overdue. The 10-day-old newborn has nothing due. 27 x 4 = 108.
	require.Len(t, resp.Msg.GetDue(), (sum.Children-1)*4)
	for _, d := range resp.Msg.GetDue() {
		require.Contains(t, []string{"opv3", "dpt3", "pcv3", "ipv"}, d.GetVaccineCode())
		require.True(t, d.GetOverdue())
		require.NotEmpty(t, d.GetChildId())
	}

	// Area filter.
	kisumu, err := svc.GetDueSummary(ctx, connect.NewRequest(&climateshieldv1.GetDueSummaryRequest{AreaId: "kisumu"}))
	require.NoError(t, err)
	require.Len(t, kisumu.Msg.GetDue(), 12*4)
	for _, d := range kisumu.Msg.GetDue() {
		require.Equal(t, "kisumu", d.GetAreaId())
	}
}
