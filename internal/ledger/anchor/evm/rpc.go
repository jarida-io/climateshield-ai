// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Client is a minimal Ethereum JSON-RPC client over HTTP. It speaks exactly
// the methods the anchor needs and nothing more.
type Client struct {
	url string
	hc  *http.Client
	seq atomic.Int64
}

// NewClient builds a client for the node at url. A nil http.Client gets a
// sensible default with a timeout, so a hung node cannot hang a caller.
func NewClient(url string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{url: url, hc: hc}
}

// URL returns the node endpoint.
func (c *Client) URL() string { return c.url }

// RPCError is a JSON-RPC error object returned by the node.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
}

const maxResponseBytes = 4 << 20

// Call invokes method with params and decodes the result into out (which may
// be nil to discard it). A JSON-RPC error is returned as *RPCError.
func (c *Client) Call(ctx context.Context, method string, out any, params ...any) error {
	if params == nil {
		params = []any{}
	}
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: c.seq.Add(1), Method: method, Params: params})
	if err != nil {
		return fmt.Errorf("evm: %s: encode: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("evm: %s: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("evm: %s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("evm: %s: read: %w", method, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("evm: %s: HTTP %d", method, resp.StatusCode)
	}
	var rr rpcResponse
	if err := json.Unmarshal(raw, &rr); err != nil {
		return fmt.Errorf("evm: %s: decode: %w", method, err)
	}
	if rr.Error != nil {
		return fmt.Errorf("evm: %s: %w", method, rr.Error)
	}
	if out == nil || string(rr.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(rr.Result, out); err != nil {
		return fmt.Errorf("evm: %s: decode result: %w", method, err)
	}
	return nil
}

// ChainID returns the node's chain id (eth_chainId).
func (c *Client) ChainID(ctx context.Context) (int64, error) {
	var q string
	if err := c.Call(ctx, "eth_chainId", &q); err != nil {
		return 0, err
	}
	return ParseQuantity(q)
}

// Accounts lists the accounts the node itself holds (eth_accounts). On the
// development chain these are anvil's unlocked accounts.
func (c *Client) Accounts(ctx context.Context) ([]string, error) {
	var out []string
	if err := c.Call(ctx, "eth_accounts", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// BlockNumber returns the latest block height (eth_blockNumber).
func (c *Client) BlockNumber(ctx context.Context) (int64, error) {
	var q string
	if err := c.Call(ctx, "eth_blockNumber", &q); err != nil {
		return 0, err
	}
	return ParseQuantity(q)
}

// GetCode returns the runtime bytecode at address (eth_getCode, latest).
func (c *Client) GetCode(ctx context.Context, address string) ([]byte, error) {
	var h string
	if err := c.Call(ctx, "eth_getCode", &h, address, "latest"); err != nil {
		return nil, err
	}
	return DecodeHex(h)
}

// Tx is an eth_sendTransaction request. An empty To deploys Data as a
// contract. Gas and price are left to the node.
type Tx struct {
	From string
	To   string
	Data []byte
}

// SendTransaction asks the node to sign and submit tx from one of its own
// accounts (eth_sendTransaction) and returns the transaction hash.
func (c *Client) SendTransaction(ctx context.Context, tx Tx) (string, error) {
	param := map[string]string{"from": tx.From, "data": EncodeHex(tx.Data)}
	if tx.To != "" {
		param["to"] = tx.To
	}
	var hash string
	if err := c.Call(ctx, "eth_sendTransaction", &hash, param); err != nil {
		return "", err
	}
	if hash == "" {
		return "", errors.New("evm: eth_sendTransaction returned no hash")
	}
	return hash, nil
}

// TxReceipt is the subset of eth_getTransactionReceipt the anchor reads.
type TxReceipt struct {
	TransactionHash string `json:"transactionHash"`
	Status          string `json:"status"`
	BlockNumber     string `json:"blockNumber"`
	ContractAddress string `json:"contractAddress"`
	Logs            []Log  `json:"logs"`
}

// Succeeded reports whether the transaction executed without reverting.
func (r TxReceipt) Succeeded() bool { return r.Status == "0x1" }

// Block returns the receipt's block number.
func (r TxReceipt) Block() (int64, error) { return ParseQuantity(r.BlockNumber) }

// TransactionReceipt fetches a receipt; (nil, nil) means not yet mined.
func (c *Client) TransactionReceipt(ctx context.Context, hash string) (*TxReceipt, error) {
	var r *TxReceipt
	if err := c.Call(ctx, "eth_getTransactionReceipt", &r, hash); err != nil {
		return nil, err
	}
	return r, nil
}

// EthCall executes a read-only call against a contract at the latest block.
func (c *Client) EthCall(ctx context.Context, to string, data []byte) ([]byte, error) {
	var h string
	if err := c.Call(ctx, "eth_call", &h, map[string]string{"to": to, "data": EncodeHex(data)}, "latest"); err != nil {
		return nil, err
	}
	return DecodeHex(h)
}

// LogFilter selects logs for eth_getLogs.
type LogFilter struct {
	FromBlock string     `json:"fromBlock,omitempty"`
	ToBlock   string     `json:"toBlock,omitempty"`
	Address   string     `json:"address,omitempty"`
	Topics    [][]string `json:"topics,omitempty"`
}

// Log is one emitted event.
type Log struct {
	Address         string   `json:"address"`
	Topics          []string `json:"topics"`
	Data            string   `json:"data"`
	BlockNumber     string   `json:"blockNumber"`
	TransactionHash string   `json:"transactionHash"`
}

// GetLogs fetches logs matching f (eth_getLogs).
func (c *Client) GetLogs(ctx context.Context, f LogFilter) ([]Log, error) {
	var out []Log
	if err := c.Call(ctx, "eth_getLogs", &out, f); err != nil {
		return nil, err
	}
	return out, nil
}

// ParseQuantity decodes a JSON-RPC hex quantity ("0x1a") to an integer.
func ParseQuantity(s string) (int64, error) {
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return 0, fmt.Errorf("evm: quantity %q lacks 0x prefix", s)
	}
	if len(s) == 2 {
		return 0, fmt.Errorf("evm: empty quantity %q", s)
	}
	v, err := strconv.ParseInt(s[2:], 16, 64)
	if err != nil {
		return 0, fmt.Errorf("evm: quantity %q: %w", s, err)
	}
	return v, nil
}

// DecodeHex decodes 0x-prefixed hex data. "0x" decodes to an empty slice.
func DecodeHex(s string) ([]byte, error) {
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return nil, fmt.Errorf("evm: hex %q lacks 0x prefix", truncate(s))
	}
	b, err := hex.DecodeString(s[2:])
	if err != nil {
		return nil, fmt.Errorf("evm: hex %q: %w", truncate(s), err)
	}
	return b, nil
}

// EncodeHex encodes data as 0x-prefixed lowercase hex.
func EncodeHex(b []byte) string { return "0x" + hex.EncodeToString(b) }

// Quantity encodes an integer as a JSON-RPC hex quantity.
func Quantity(v int64) string { return "0x" + strconv.FormatInt(v, 16) }

func truncate(s string) string {
	if len(s) > 20 {
		return s[:20] + "…"
	}
	return s
}
