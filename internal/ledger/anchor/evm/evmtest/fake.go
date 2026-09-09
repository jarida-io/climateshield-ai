// SPDX-License-Identifier: Apache-2.0

// Package evmtest provides two test doubles for the chain anchor: Fake, an
// in-process JSON-RPC server that mimics the slice of an Ethereum node the
// anchor uses (no Docker, no network), and Anvil, a real development chain
// in a container for tests that must execute the committed bytecode.
package evmtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm"
)

// Request is one recorded JSON-RPC call.
type Request struct {
	Method string
	Params json.RawMessage
}

// Fake is a scripted JSON-RPC node. It does not execute bytecode: it
// recognises RootAnchor's ABI by selector and keeps per-day root history the
// way the contract does, so tests can assert exact calldata and drive every
// failure path (pending receipts, reverts, RPC errors, a lying read-back).
type Fake struct {
	srv *httptest.Server

	mu        sync.Mutex
	chainID   int64
	accounts  []string
	code      map[string][]byte
	publisher map[string]string
	history   map[string]map[[32]byte][][32]byte
	receipts  map[string]*evm.TxReceipt
	pending   map[string]int
	logs      []evm.Log
	block     int64
	nonce     int
	failures  map[string]int
	requests  []Request

	// PendingPolls is how many eth_getTransactionReceipt calls answer null
	// before a transaction counts as mined. Applies to transactions sent
	// after it is set.
	PendingPolls int
	// RevertNext makes the next transaction mine with status 0x0.
	RevertNext bool
	// ReadBackOverride makes rootOf return this value instead of the truth.
	ReadBackOverride *[32]byte
}

// DefaultAccount is the fake's first (and only) account.
const DefaultAccount = "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"

// NewFake starts a fake node on chain id 31337 with one account.
func NewFake(t testing.TB) *Fake {
	t.Helper()
	f := &Fake{
		chainID:   31337,
		accounts:  []string{DefaultAccount},
		code:      map[string][]byte{},
		publisher: map[string]string{},
		history:   map[string]map[[32]byte][][32]byte{},
		receipts:  map[string]*evm.TxReceipt{},
		pending:   map[string]int{},
		failures:  map[string]int{},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

// URL is the node's RPC endpoint.
func (f *Fake) URL() string { return f.srv.URL }

// SetChainID changes the reported chain id.
func (f *Fake) SetChainID(id int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chainID = id
}

// SetAccounts replaces the node's accounts (nil = none unlocked).
func (f *Fake) SetAccounts(accounts []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts = accounts
}

// FailNext makes the next n calls of method return a JSON-RPC error.
func (f *Fake) FailNext(method string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[method] = n
}

// InstallCode places runtime code at an address, as if deployed elsewhere.
func (f *Fake) InstallCode(address string, code []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.code[strings.ToLower(address)] = code
	f.publisher[strings.ToLower(address)] = DefaultAccount
}

// Requests returns every recorded call, in order.
func (f *Fake) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.requests...)
}

// RequestsFor returns the recorded calls of one method.
func (f *Fake) RequestsFor(method string) []Request {
	var out []Request
	for _, r := range f.Requests() {
		if r.Method == method {
			out = append(out, r)
		}
	}
	return out
}

// Deployed lists contract addresses created through eth_sendTransaction.
func (f *Fake) Deployed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for addr := range f.code {
		if _, ok := f.publisher[addr]; ok {
			out = append(out, addr)
		}
	}
	return out
}

// Roots returns the anchored history for a day at a contract.
func (f *Fake) Roots(contract string, day [32]byte) [][32]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][32]byte(nil), f.history[strings.ToLower(contract)][day]...)
}

type rpcReq struct {
	ID     json.RawMessage   `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

func (f *Fake) handle(w http.ResponseWriter, r *http.Request) {
	var req rpcReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, _ := json.Marshal(req.Params)
	f.requests = append(f.requests, Request{Method: req.Method, Params: raw})

	if n := f.failures[req.Method]; n > 0 {
		f.failures[req.Method] = n - 1
		f.reply(w, req.ID, nil, &evm.RPCError{Code: -32000, Message: "injected failure"})
		return
	}
	result, rpcErr := f.dispatch(req)
	f.reply(w, req.ID, result, rpcErr)
}

func (f *Fake) reply(w http.ResponseWriter, id json.RawMessage, result any, rpcErr *evm.RPCError) {
	w.Header().Set("Content-Type", "application/json")
	body := map[string]any{"jsonrpc": "2.0", "id": id}
	if rpcErr != nil {
		body["error"] = rpcErr
	} else {
		body["result"] = result
	}
	_ = json.NewEncoder(w).Encode(body)
}

type txParam struct {
	From string `json:"from"`
	To   string `json:"to"`
	Data string `json:"data"`
}

func (f *Fake) dispatch(req rpcReq) (any, *evm.RPCError) {
	switch req.Method {
	case "eth_chainId":
		return evm.Quantity(f.chainID), nil
	case "eth_accounts":
		if f.accounts == nil {
			return []string{}, nil
		}
		return f.accounts, nil
	case "eth_blockNumber":
		return evm.Quantity(f.block), nil
	case "eth_getCode":
		var addr string
		if err := json.Unmarshal(req.Params[0], &addr); err != nil {
			return nil, &evm.RPCError{Code: -32602, Message: "bad address"}
		}
		return evm.EncodeHex(f.code[strings.ToLower(addr)]), nil
	case "eth_sendTransaction":
		var tx txParam
		if err := json.Unmarshal(req.Params[0], &tx); err != nil {
			return nil, &evm.RPCError{Code: -32602, Message: "bad transaction"}
		}
		return f.sendTransaction(tx)
	case "eth_getTransactionReceipt":
		var hash string
		if err := json.Unmarshal(req.Params[0], &hash); err != nil {
			return nil, &evm.RPCError{Code: -32602, Message: "bad hash"}
		}
		if f.pending[hash] > 0 {
			f.pending[hash]--
			return nil, nil
		}
		rcpt, ok := f.receipts[hash]
		if !ok {
			return nil, nil
		}
		return rcpt, nil
	case "eth_call":
		var call txParam
		if err := json.Unmarshal(req.Params[0], &call); err != nil {
			return nil, &evm.RPCError{Code: -32602, Message: "bad call"}
		}
		return f.call(call)
	case "eth_getLogs":
		var filter evm.LogFilter
		if err := json.Unmarshal(req.Params[0], &filter); err != nil {
			return nil, &evm.RPCError{Code: -32602, Message: "bad filter"}
		}
		out := []evm.Log{}
		for _, l := range f.logs {
			if filter.Address != "" && !strings.EqualFold(filter.Address, l.Address) {
				continue
			}
			if len(filter.Topics) > 0 && len(filter.Topics[0]) > 0 && !strings.EqualFold(filter.Topics[0][0], l.Topics[0]) {
				continue
			}
			out = append(out, l)
		}
		return out, nil
	}
	return nil, &evm.RPCError{Code: -32601, Message: "method not found: " + req.Method}
}

func (f *Fake) sendTransaction(tx txParam) (any, *evm.RPCError) {
	if !f.hasAccount(tx.From) {
		return nil, &evm.RPCError{Code: -32000, Message: "unknown account " + tx.From}
	}
	data, err := evm.DecodeHex(tx.Data)
	if err != nil {
		return nil, &evm.RPCError{Code: -32602, Message: "bad data"}
	}
	f.nonce++
	f.block++
	hash := fmt.Sprintf("0x%064x", f.nonce)
	rcpt := &evm.TxReceipt{TransactionHash: hash, Status: "0x1", BlockNumber: evm.Quantity(f.block), Logs: []evm.Log{}}
	if f.RevertNext {
		rcpt.Status = "0x0"
		f.RevertNext = false
	} else if tx.To == "" {
		addr := fmt.Sprintf("0x%040x", 0xc0ffee00+f.nonce)
		// Deploying the committed creation bytecode installs the committed
		// runtime; anything else is installed verbatim so tests can plant
		// wrong code.
		installed := data
		if string(data) == string(evm.Bytecode()) {
			installed = evm.RuntimeBytecode()
		}
		f.code[addr] = installed
		f.publisher[addr] = strings.ToLower(tx.From)
		rcpt.ContractAddress = addr
	} else {
		rcpt.Status = f.execute(strings.ToLower(tx.To), strings.ToLower(tx.From), data, hash, rcpt)
	}
	f.receipts[hash] = rcpt
	if f.PendingPolls > 0 {
		f.pending[hash] = f.PendingPolls
	}
	return hash, nil
}

func (f *Fake) hasAccount(addr string) bool {
	for _, a := range f.accounts {
		if strings.EqualFold(a, addr) {
			return true
		}
	}
	return false
}

// execute mimics RootAnchor.anchor: publisher-only, non-zero root, idempotent
// on the current root, otherwise append and emit Anchored.
func (f *Fake) execute(to, from string, data []byte, hash string, rcpt *evm.TxReceipt) string {
	if _, ok := f.code[to]; !ok || len(data) != 4+64 || string(data[:4]) != string(evm.SelectorAnchor) {
		return "0x0"
	}
	if f.publisher[to] != from {
		return "0x0"
	}
	var day, root [32]byte
	copy(day[:], data[4:36])
	copy(root[:], data[36:68])
	if root == [32]byte{} {
		return "0x0"
	}
	if f.history[to] == nil {
		f.history[to] = map[[32]byte][][32]byte{}
	}
	h := f.history[to][day]
	if len(h) > 0 && h[len(h)-1] == root {
		return "0x1"
	}
	h = append(h, root)
	f.history[to][day] = h
	version := make([]byte, 32)
	version[31] = byte(len(h))
	l := evm.Log{
		Address:         to,
		Topics:          []string{evm.EncodeHex(evm.AnchoredTopic), evm.EncodeHex(day[:])},
		Data:            evm.EncodeHex(append(append([]byte{}, root[:]...), version...)),
		BlockNumber:     rcpt.BlockNumber,
		TransactionHash: hash,
	}
	f.logs = append(f.logs, l)
	rcpt.Logs = append(rcpt.Logs, l)
	return "0x1"
}

func (f *Fake) call(c txParam) (any, *evm.RPCError) {
	to := strings.ToLower(c.To)
	data, err := evm.DecodeHex(c.Data)
	if err != nil || len(data) < 4 {
		return nil, &evm.RPCError{Code: -32602, Message: "bad calldata"}
	}
	if _, ok := f.code[to]; !ok {
		return "0x", nil
	}
	var day [32]byte
	if len(data) >= 36 {
		copy(day[:], data[4:36])
	}
	switch string(data[:4]) {
	case string(evm.SelectorRootOf):
		if f.ReadBackOverride != nil {
			return evm.EncodeHex(f.ReadBackOverride[:]), nil
		}
		h := f.history[to][day]
		if len(h) == 0 {
			return evm.EncodeHex(make([]byte, 32)), nil
		}
		return evm.EncodeHex(h[len(h)-1][:]), nil
	case string(evm.SelectorVersions):
		out := make([]byte, 32)
		out[31] = byte(len(f.history[to][day]))
		return evm.EncodeHex(out), nil
	case string(evm.SelectorPublisher):
		pub, _ := evm.DecodeHex(f.publisher[to])
		out := make([]byte, 32)
		copy(out[32-len(pub):], pub)
		return evm.EncodeHex(out), nil
	}
	return nil, &evm.RPCError{Code: 3, Message: "execution reverted"}
}
