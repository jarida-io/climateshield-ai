// SPDX-License-Identifier: Apache-2.0

package evm_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm"
	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm/evmtest"
)

func TestClientReadsBasicNodeFacts(t *testing.T) {
	f := evmtest.NewFake(t)
	c := evm.NewClient(f.URL(), nil)
	ctx := context.Background()
	require.Equal(t, f.URL(), c.URL())

	id, err := c.ChainID(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 31337, id)

	accounts, err := c.Accounts(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{evmtest.DefaultAccount}, accounts)

	n, err := c.BlockNumber(ctx)
	require.NoError(t, err)
	require.Zero(t, n)

	code, err := c.GetCode(ctx, "0x0000000000000000000000000000000000000001")
	require.NoError(t, err)
	require.Empty(t, code, "an empty account has no code")

	// A receipt for an unknown hash is "not mined", not an error.
	rcpt, err := c.TransactionReceipt(ctx, "0x01")
	require.NoError(t, err)
	require.Nil(t, rcpt)

	logs, err := c.GetLogs(ctx, evm.LogFilter{Address: "0x01"})
	require.NoError(t, err)
	require.Empty(t, logs)

	// Parameters are sent as a JSON array even when there are none.
	for _, r := range f.RequestsFor("eth_chainId") {
		require.JSONEq(t, "[]", string(r.Params))
	}
}

func TestClientSurfacesNodeErrors(t *testing.T) {
	f := evmtest.NewFake(t)
	c := evm.NewClient(f.URL(), nil)
	ctx := context.Background()

	f.FailNext("eth_chainId", 1)
	_, err := c.ChainID(ctx)
	var rpcErr *evm.RPCError
	require.ErrorAs(t, err, &rpcErr)
	require.Equal(t, -32000, rpcErr.Code)
	require.Contains(t, rpcErr.Error(), "injected failure")

	// Unknown method.
	require.Error(t, c.Call(ctx, "eth_noSuchMethod", nil))

	// A transaction from an account the node does not hold.
	_, err = c.SendTransaction(ctx, evm.Tx{From: "0x00000000000000000000000000000000000000aa", Data: []byte{1}})
	require.Error(t, err)
}

func TestClientHandlesBrokenServers(t *testing.T) {
	ctx := context.Background()

	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(broken.Close)
	_, err := evm.NewClient(broken.URL, nil).ChainID(ctx)
	require.ErrorContains(t, err, "HTTP 502")

	garbage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	t.Cleanup(garbage.Close)
	_, err = evm.NewClient(garbage.URL, nil).ChainID(ctx)
	require.ErrorContains(t, err, "decode")

	wrongType := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"not":"a string"}}`))
	}))
	t.Cleanup(wrongType.Close)
	_, err = evm.NewClient(wrongType.URL, nil).ChainID(ctx)
	require.ErrorContains(t, err, "decode result")

	emptyHash := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":""}`))
	}))
	t.Cleanup(emptyHash.Close)
	_, err = evm.NewClient(emptyHash.URL, nil).SendTransaction(ctx, evm.Tx{From: "0x01", Data: []byte{1}})
	require.ErrorContains(t, err, "no hash")

	// Nothing listening at all.
	_, err = evm.NewClient("http://127.0.0.1:1", nil).ChainID(ctx)
	require.Error(t, err)

	// A cancelled context stops the call.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = evm.NewClient(broken.URL, nil).ChainID(cancelled)
	require.ErrorIs(t, err, context.Canceled)
}

func TestHexHelpers(t *testing.T) {
	n, err := evm.ParseQuantity("0x1a")
	require.NoError(t, err)
	require.EqualValues(t, 26, n)
	n, err = evm.ParseQuantity("0X0")
	require.NoError(t, err)
	require.Zero(t, n)
	for _, bad := range []string{"1a", "0x", "0xzz", ""} {
		_, err := evm.ParseQuantity(bad)
		require.Error(t, err, bad)
	}
	require.Equal(t, "0x1a", evm.Quantity(26))

	b, err := evm.DecodeHex("0x")
	require.NoError(t, err)
	require.Empty(t, b)
	b, err = evm.DecodeHex("0xdeadBEEF")
	require.NoError(t, err)
	require.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, b)
	require.Equal(t, "0xdeadbeef", evm.EncodeHex(b))
	_, err = evm.DecodeHex("deadbeef")
	require.Error(t, err)
	_, err = evm.DecodeHex("0x" + "zz" + "0123456789012345678901234567890123456789")
	require.Error(t, err)

	var rpcErr *evm.RPCError
	require.False(t, errors.As(err, &rpcErr), "a hex error is not an RPC error")
}
