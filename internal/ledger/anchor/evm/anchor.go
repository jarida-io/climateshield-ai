// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jarida-io/climateshield/internal/ledger/anchor"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// Sentinel errors. Each names a condition the ledger must not paper over.
var (
	// ErrNoAccount: the node holds no unlocked account and none was configured.
	ErrNoAccount = errors.New("evm: no account to send from (node returned no eth_accounts and ANCHOR_FROM is empty)")
	// ErrCodeMismatch: the code at the contract address is not RootAnchor.
	ErrCodeMismatch = errors.New("evm: code at the contract address does not match the committed RootAnchor runtime bytecode")
	// ErrReverted: a transaction was mined but failed.
	ErrReverted = errors.New("evm: transaction reverted")
	// ErrConfirmTimeout: no receipt arrived within ANCHOR_CONFIRM_TIMEOUT.
	ErrConfirmTimeout = errors.New("evm: transaction not confirmed within the timeout")
	// ErrReadBackMismatch: rootOf(day) after anchoring is not the root sent.
	ErrReadBackMismatch = errors.New("evm: read-back mismatch: the chain does not hold the root that was anchored")
)

// Config configures the chain anchor. Every field maps to an ANCHOR_*
// environment variable of the ledger service.
type Config struct {
	// RPCURL is the node's JSON-RPC endpoint.
	RPCURL string
	// From is the sending account; empty selects the node's first account.
	From string
	// ContractAddress pins an existing RootAnchor; empty deploys once per
	// chain and records the address in anchor_contracts.
	ContractAddress string
	// ConfirmTimeout bounds the wait for a transaction receipt.
	ConfirmTimeout time.Duration
	// PollInterval is how often the receipt is polled for. Zero means 250ms.
	PollInterval time.Duration
}

// ContractStore remembers which RootAnchor was deployed on which chain.
// *db.Queries satisfies it.
type ContractStore interface {
	GetAnchorContract(ctx context.Context, chainID int64) (db.AnchorContract, error)
	UpsertAnchorContract(ctx context.Context, arg db.UpsertAnchorContractParams) error
}

// Deployment is the resolved chain, sender and contract the anchor uses.
type Deployment struct {
	ChainID         int64
	ChainLabel      string
	From            string
	ContractAddress string
	// Deployed is true when Ensure deployed the contract in this process.
	Deployed bool
	DeployTx string
}

// Anchor publishes daily roots to RootAnchor and reads each one back.
type Anchor struct {
	client *Client
	cfg    Config
	store  ContractStore
	log    *slog.Logger

	mu  sync.Mutex
	dep *Deployment
}

// New builds a chain anchor. Nothing is contacted until Ensure or AnchorRoot.
func New(cfg Config, store ContractStore, log *slog.Logger) *Anchor {
	if cfg.ConfirmTimeout <= 0 {
		cfg.ConfirmTimeout = 30 * time.Second
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 250 * time.Millisecond
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Anchor{client: NewClient(cfg.RPCURL, nil), cfg: cfg, store: store, log: log}
}

// Client exposes the underlying RPC client (read-only callers reuse it).
func (a *Anchor) Client() *Client { return a.client }

// Type implements anchor.Anchor.
func (a *Anchor) Type() string { return anchor.TypeEVM }

// Ensure resolves the chain id, the sending account and a verified contract
// address, deploying RootAnchor once if this chain has none. It is safe to
// call repeatedly; the result is cached after the first success.
func (a *Anchor) Ensure(ctx context.Context) (Deployment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dep != nil {
		return *a.dep, nil
	}
	dep, err := a.resolve(ctx)
	if err != nil {
		return Deployment{}, err
	}
	a.dep = &dep
	return dep, nil
}

func (a *Anchor) resolve(ctx context.Context) (Deployment, error) {
	chainID, err := a.client.ChainID(ctx)
	if err != nil {
		return Deployment{}, err
	}
	dep := Deployment{ChainID: chainID, ChainLabel: Label(chainID)}

	dep.From = a.cfg.From
	if dep.From == "" {
		accounts, err := a.client.Accounts(ctx)
		if err != nil {
			return Deployment{}, err
		}
		if len(accounts) == 0 {
			return Deployment{}, ErrNoAccount
		}
		dep.From = accounts[0]
	}

	if a.cfg.ContractAddress != "" {
		// An operator-pinned address must be exactly RootAnchor; anything else
		// is a misconfiguration, never something to deploy over or fall back
		// from.
		if err := a.verifyCode(ctx, a.cfg.ContractAddress); err != nil {
			return Deployment{}, fmt.Errorf("ANCHOR_CONTRACT_ADDRESS %s: %w", a.cfg.ContractAddress, err)
		}
		dep.ContractAddress = a.cfg.ContractAddress
		return dep, nil
	}

	row, err := a.store.GetAnchorContract(ctx, chainID)
	switch {
	case err == nil:
		if verr := a.verifyCode(ctx, row.Address); verr == nil {
			dep.ContractAddress = row.Address
			return dep, nil
		}
		// The database remembers a contract the chain no longer has (for
		// example a development chain restarted without its state). Deploy
		// again and say so; earlier anchors on the old address stay recorded
		// and are reported as unverifiable by the public API.
		a.log.Warn("recorded RootAnchor is not on the chain; deploying a new one",
			"chain_id", chainID, "recorded_address", row.Address)
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return Deployment{}, fmt.Errorf("evm: anchor_contracts lookup: %w", err)
	}

	addr, txHash, err := a.deploy(ctx, dep.From)
	if err != nil {
		return Deployment{}, err
	}
	if err := a.store.UpsertAnchorContract(ctx, db.UpsertAnchorContractParams{
		ChainID: chainID, Address: addr, DeployTx: &txHash,
	}); err != nil {
		return Deployment{}, fmt.Errorf("evm: record contract: %w", err)
	}
	dep.ContractAddress, dep.Deployed, dep.DeployTx = addr, true, txHash
	a.log.Info("RootAnchor deployed", "chain_id", chainID, "chain_label", dep.ChainLabel,
		"contract", addr, "tx", txHash)
	return dep, nil
}

// verifyCode checks that the code at address is exactly the committed
// RootAnchor runtime bytecode.
func (a *Anchor) verifyCode(ctx context.Context, address string) error {
	code, err := a.client.GetCode(ctx, address)
	if err != nil {
		return err
	}
	if !bytes.Equal(code, RuntimeBytecode()) {
		return ErrCodeMismatch
	}
	return nil
}

// deploy sends the creation bytecode from `from`, waits for the receipt and
// verifies the code that landed.
func (a *Anchor) deploy(ctx context.Context, from string) (address, txHash string, err error) {
	txHash, err = a.client.SendTransaction(ctx, Tx{From: from, Data: Bytecode()})
	if err != nil {
		return "", "", fmt.Errorf("evm: deploy: %w", err)
	}
	rcpt, err := a.waitReceipt(ctx, txHash)
	if err != nil {
		return "", "", fmt.Errorf("evm: deploy %s: %w", txHash, err)
	}
	if rcpt.ContractAddress == "" {
		return "", "", fmt.Errorf("evm: deploy %s: receipt carries no contract address", txHash)
	}
	if err := a.verifyCode(ctx, rcpt.ContractAddress); err != nil {
		return "", "", fmt.Errorf("evm: deploy %s: %w", txHash, err)
	}
	return strings.ToLower(rcpt.ContractAddress), txHash, nil
}

// waitReceipt polls for a mined receipt and requires success.
func (a *Anchor) waitReceipt(ctx context.Context, txHash string) (*TxReceipt, error) {
	deadline := time.Now().Add(a.cfg.ConfirmTimeout)
	for {
		rcpt, err := a.client.TransactionReceipt(ctx, txHash)
		if err != nil {
			return nil, err
		}
		if rcpt != nil {
			if !rcpt.Succeeded() {
				return nil, fmt.Errorf("%w (status %s)", ErrReverted, rcpt.Status)
			}
			return rcpt, nil
		}
		if time.Now().After(deadline) {
			return nil, ErrConfirmTimeout
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(a.cfg.PollInterval):
		}
	}
}

// AnchorRoot implements anchor.Anchor: send anchor(day, root), wait for the
// receipt, then read rootOf(day) back and require it to equal root. A
// mismatch is an error — a receipt alone is not proof the chain holds the
// root, and this anchor never reports what it has not read back.
func (a *Anchor) AnchorRoot(ctx context.Context, day time.Time, root []byte) (anchor.Receipt, error) {
	root32, err := Root32(root)
	if err != nil {
		return anchor.Receipt{}, err
	}
	dep, err := a.Ensure(ctx)
	if err != nil {
		return anchor.Receipt{}, err
	}
	day32 := DayBytes32(day)

	txHash, err := a.client.SendTransaction(ctx, Tx{From: dep.From, To: dep.ContractAddress, Data: PackAnchor(day32, root32)})
	if err != nil {
		return anchor.Receipt{}, fmt.Errorf("evm: anchor %s: %w", DayString(day32), err)
	}
	rcpt, err := a.waitReceipt(ctx, txHash)
	if err != nil {
		return anchor.Receipt{}, fmt.Errorf("evm: anchor %s tx %s: %w", DayString(day32), txHash, err)
	}
	block, err := rcpt.Block()
	if err != nil {
		return anchor.Receipt{}, fmt.Errorf("evm: anchor %s tx %s: %w", DayString(day32), txHash, err)
	}

	ret, err := a.client.EthCall(ctx, dep.ContractAddress, PackRootOf(day32))
	if err != nil {
		return anchor.Receipt{}, fmt.Errorf("evm: read back %s: %w", DayString(day32), err)
	}
	got, err := UnpackBytes32(ret)
	if err != nil {
		return anchor.Receipt{}, fmt.Errorf("evm: read back %s: %w", DayString(day32), err)
	}
	out := anchor.Receipt{
		Type:            anchor.TypeEVM,
		Reference:       txHash,
		ChainID:         dep.ChainID,
		ChainLabel:      dep.ChainLabel,
		ContractAddress: dep.ContractAddress,
		TxHash:          txHash,
		BlockNumber:     block,
		ReadBack:        got[:],
		ReadBackOK:      got == root32,
	}
	if !out.ReadBackOK {
		return out, fmt.Errorf("evm: %s tx %s: %w", DayString(day32), txHash, ErrReadBackMismatch)
	}
	a.log.Info("daily root anchored on chain", "day", DayString(day32), "chain_id", dep.ChainID,
		"contract", dep.ContractAddress, "tx", txHash, "block", block)
	return out, nil
}
