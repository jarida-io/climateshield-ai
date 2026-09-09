// SPDX-License-Identifier: Apache-2.0

package publicapi_test

import (
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/ledger"
	"github.com/jarida-io/climateshield/internal/ledger/anchor"
	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm"
	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm/evmtest"
	"github.com/jarida-io/climateshield/internal/platform/crypto"
	"github.com/jarida-io/climateshield/internal/platform/logging"
	"github.com/jarida-io/climateshield/internal/publicapi"
	"github.com/jarida-io/climateshield/internal/store/db"
	"github.com/jarida-io/climateshield/internal/store/seed"
	"github.com/jarida-io/climateshield/internal/store/testdb"
)

// anchoredOnFakeChain seeds the demo population and sweeps it through BOTH
// anchors — the local table and the RootAnchor contract on an in-process fake
// node — so the public API has real chain-anchored rows behind it.
func anchoredOnFakeChain(t *testing.T) (*db.Queries, *evmtest.Fake, *httptest.Server) {
	t.Helper()
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	log := logging.New(io.Discard, "info")

	key, err := crypto.NewRandomKey()
	require.NoError(t, err)
	_, err = seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)

	f := evmtest.NewFake(t)
	chain := evm.New(evm.Config{RPCURL: f.URL(), PollInterval: time.Millisecond}, q, log)
	_, _, err = ledger.Sweep(ctx, q, anchor.Multi{anchor.NewLocal(), chain}, log)
	require.NoError(t, err)

	srv := publicapi.NewServer(pool, log).WithAnchor("evm", f.URL())
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)
	return q, f, ts
}

func verification(t *testing.T, ts *httptest.Server, path string) (int, *climateshieldv1.GetAnchorVerificationResponse) {
	t.Helper()
	status, body := get(t, ts, path)
	var msg climateshieldv1.GetAnchorVerificationResponse
	if status == http.StatusOK {
		require.NoError(t, protojson.Unmarshal([]byte(body), &msg))
	}
	return status, &msg
}

func TestLedgerSummaryNamesTheChainOnlyWhenARootReachedIt(t *testing.T) {
	q, f, ts := anchoredOnFakeChain(t)

	_, body := get(t, ts, "/v1/ledger/summary")
	var msg climateshieldv1.GetLedgerSummaryResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &msg))
	require.Equal(t, "evm", msg.GetAnchorMode())

	// The note is computed from the newest anchor row: it names the contract
	// and chain, calls the development chain what it is, and never claims
	// immutability or publicity.
	contract := f.Deployed()[0]
	require.Contains(t, msg.GetAnchorNote(), "RootAnchor contract at "+contract)
	require.Contains(t, msg.GetAnchorNote(), "chain id 31337")
	require.Contains(t, msg.GetAnchorNote(), evm.DevChainLabel)
	require.Contains(t, msg.GetAnchorNote(), "does not outlive `make down -v`")
	require.NotContains(t, strings.ToLower(msg.GetAnchorNote()), "immutable")
	require.NotContains(t, strings.ToLower(msg.GetAnchorNote()), "decentralis")

	// Every published root carries the newest anchor's chain facts and the
	// read-back verdict, and still matches the database.
	days, err := q.ListLeafDays(context.Background())
	require.NoError(t, err)
	require.Len(t, msg.GetRoots(), len(days))
	for _, r := range msg.GetRoots() {
		require.Equal(t, "evm", r.GetAnchorType())
		require.EqualValues(t, 31337, r.GetChainId())
		require.Equal(t, evm.DevChainLabel, r.GetChainLabel())
		require.Equal(t, contract, r.GetContractAddress())
		require.NotEmpty(t, r.GetTxHash())
		require.Equal(t, r.GetTxHash(), r.GetAnchorReference())
		require.Positive(t, r.GetBlockNumber())
		require.True(t, r.GetReadbackMatches())
		require.NotNil(t, r.GetVerifiedAt())
	}
}

func TestAnchorVerificationChecksTheChainLive(t *testing.T) {
	q, f, ts := anchoredOnFakeChain(t)
	ctx := context.Background()
	days, err := q.ListLeafDays(ctx)
	require.NoError(t, err)
	day := days[len(days)-1]
	stored, err := q.GetDailyRoot(ctx, day)
	require.NoError(t, err)
	dayStr := day.Time.Format("2006-01-02")

	// Verified: the chain holds exactly the database root.
	callsBefore := len(f.RequestsFor("eth_call"))
	status, v := verification(t, ts, "/v1/ledger/anchors/verify?day="+dayStr)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "verified", v.GetStatus())
	require.Equal(t, dayStr, v.GetDay())
	require.Equal(t, hex.EncodeToString(stored.Root), v.GetDbRootHex())
	require.Equal(t, v.GetDbRootHex(), v.GetChainRootHex())
	require.EqualValues(t, 31337, v.GetChainId())
	require.Equal(t, evm.DevChainLabel, v.GetChainLabel())
	require.Equal(t, f.Deployed()[0], v.GetContractAddress())
	require.NotEmpty(t, v.GetTxHash())
	require.Equal(t, "evm", v.GetAnchorMode())
	require.NotNil(t, v.GetCheckedAt())
	require.Equal(t, evm.EncodeHex(func() []byte { d := evm.DayBytes32(day.Time); return d[:] }()), v.GetDayBytes32())
	require.Greater(t, len(f.RequestsFor("eth_call")), callsBefore, "the check must really ask the chain")

	// No day given: the newest day with a root.
	status, latest := verification(t, ts, "/v1/ledger/anchors/verify")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, dayStr, latest.GetDay())
	require.Equal(t, "verified", latest.GetStatus())

	// Mismatch: the chain answers a different root. Reported, never smoothed
	// over, and still a 200.
	lie := [32]byte{0xbb}
	f.ReadBackOverride = &lie
	status, v = verification(t, ts, "/v1/ledger/anchors/verify?day="+dayStr)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "mismatch", v.GetStatus())
	require.Equal(t, hex.EncodeToString(lie[:]), v.GetChainRootHex())
	require.Contains(t, v.GetReason(), "different root")

	// An empty root on the chain (reset chain) is a mismatch with its own reason.
	zero := [32]byte{}
	f.ReadBackOverride = &zero
	_, v = verification(t, ts, "/v1/ledger/anchors/verify?day="+dayStr)
	require.Equal(t, "mismatch", v.GetStatus())
	require.Contains(t, v.GetReason(), "holds no root")
	f.ReadBackOverride = nil

	// A day with no root, a malformed day, and a day without a chain anchor.
	_, v = verification(t, ts, "/v1/ledger/anchors/verify?day=1999-01-01")
	require.Equal(t, "unavailable", v.GetStatus())
	require.Contains(t, v.GetReason(), "no daily root")
	status, _ = verification(t, ts, "/v1/ledger/anchors/verify?day=not-a-day")
	require.Equal(t, http.StatusBadRequest, status)

	require.NoError(t, q.UpsertDailyRoot(ctx, db.UpsertDailyRootParams{
		LeafDay: pgtype.Date{Time: time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		Root:    []byte{1, 2, 3}, LeafCount: 20,
	}))
	_, v = verification(t, ts, "/v1/ledger/anchors/verify?day=2001-01-01")
	require.Equal(t, "unavailable", v.GetStatus())
	require.Contains(t, v.GetReason(), "no chain anchor")

	// The RPC disagreeing about which chain it is: unavailable, with the ids.
	f.SetChainID(1)
	_, v = verification(t, ts, "/v1/ledger/anchors/verify?day="+dayStr)
	require.Equal(t, "unavailable", v.GetStatus())
	require.Contains(t, v.GetReason(), "chain id 1")
	f.SetChainID(31337)

	// RPC failures never fail the read.
	f.FailNext("eth_chainId", 1)
	status, v = verification(t, ts, "/v1/ledger/anchors/verify?day="+dayStr)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "unavailable", v.GetStatus())
	require.Contains(t, v.GetReason(), "did not answer")
	f.FailNext("eth_call", 1)
	_, v = verification(t, ts, "/v1/ledger/anchors/verify?day="+dayStr)
	require.Equal(t, "unavailable", v.GetStatus())
	require.Contains(t, v.GetReason(), "eth_call")

	// The Connect twin answers the same.
	resp, err := http.Post(ts.URL+"/climateshield.v1.PublicService/GetAnchorVerification",
		"application/json", strings.NewReader(`{"day":"`+dayStr+`"}`))
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var c climateshieldv1.GetAnchorVerificationResponse
	require.NoError(t, protojson.Unmarshal(body, &c))
	require.Equal(t, "verified", c.GetStatus())
}

func TestAnchorVerificationSaysWhyWhenNoChainIsConfigured(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	log := logging.New(io.Discard, "info")

	// Nothing committed at all.
	srv := publicapi.NewServer(pool, log)
	ts := httptest.NewServer(srv.Router(nil, nil))
	t.Cleanup(ts.Close)
	status, v := verification(t, ts, "/v1/ledger/anchors/verify")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "unavailable", v.GetStatus())
	require.Equal(t, "local", v.GetAnchorMode())
	require.Contains(t, v.GetReason(), "no daily root exists yet")
	require.Empty(t, v.GetChainRootHex(), "no root may be invented")

	// Local-only anchoring: the root exists, there is nothing to check it against.
	key, err := crypto.NewRandomKey()
	require.NoError(t, err)
	_, err = seed.Demo(ctx, pool, key, time.Now())
	require.NoError(t, err)
	_, _, err = ledger.Sweep(ctx, q, anchor.Multi{anchor.NewLocal()}, log)
	require.NoError(t, err)
	_, v = verification(t, ts, "/v1/ledger/anchors/verify")
	require.Equal(t, "unavailable", v.GetStatus())
	require.Contains(t, v.GetReason(), "ANCHOR_MODE=local")
	require.NotEmpty(t, v.GetDbRootHex())
	require.Empty(t, v.GetChainRootHex())

	// The ledger anchored on a chain but this API has no RPC: say so.
	f := evmtest.NewFake(t)
	chain := evm.New(evm.Config{RPCURL: f.URL(), PollInterval: time.Millisecond}, q, log)
	_, _, err = ledger.Sweep(ctx, q, anchor.Multi{anchor.NewLocal(), chain}, log)
	require.NoError(t, err)
	_, v = verification(t, ts, "/v1/ledger/anchors/verify")
	require.Equal(t, "unavailable", v.GetStatus())
	require.Contains(t, v.GetReason(), "no chain RPC configured")
	require.NotEmpty(t, v.GetContractAddress(), "the recorded anchor is still reported")

	// evm mode configured, but the ledger has not anchored on chain yet.
	pool2 := testdb.Pool(t)
	q2 := db.New(pool2)
	_, err = seed.Demo(ctx, pool2, key, time.Now())
	require.NoError(t, err)
	_, _, err = ledger.Sweep(ctx, q2, anchor.Multi{anchor.NewLocal()}, log)
	require.NoError(t, err)
	srv2 := publicapi.NewServer(pool2, log).WithAnchor("evm", f.URL())
	ts2 := httptest.NewServer(srv2.Router(nil, nil))
	t.Cleanup(ts2.Close)
	_, v = verification(t, ts2, "/v1/ledger/anchors/verify")
	require.Equal(t, "unavailable", v.GetStatus())
	require.Contains(t, v.GetReason(), "no chain anchor has been recorded")

	_, body := get(t, ts2, "/v1/ledger/summary")
	var sum climateshieldv1.GetLedgerSummaryResponse
	require.NoError(t, protojson.Unmarshal([]byte(body), &sum))
	require.Equal(t, "evm", sum.GetAnchorMode())
	require.Contains(t, sum.GetAnchorNote(), "No blockchain is written to by this system")
	require.Contains(t, sum.GetAnchorNote(), "no root has reached the chain yet")

	// evm mode with an empty ledger.
	pool3 := testdb.Pool(t)
	srv3 := publicapi.NewServer(pool3, log).WithAnchor("evm", f.URL())
	ts3 := httptest.NewServer(srv3.Router(nil, nil))
	t.Cleanup(ts3.Close)
	_, body = get(t, ts3, "/v1/ledger/summary")
	require.NoError(t, protojson.Unmarshal([]byte(body), &sum))
	require.Contains(t, sum.GetAnchorNote(), "nothing has been written to any chain so far")
}
