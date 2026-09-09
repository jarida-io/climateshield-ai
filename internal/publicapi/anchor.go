// SPDX-License-Identifier: Apache-2.0

package publicapi

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	climateshieldv1 "github.com/jarida-io/climateshield/internal/gen/climateshield/v1"
	"github.com/jarida-io/climateshield/internal/ledger/anchor"
	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// This file answers one question on a public surface: does the chain still
// hold the root this system published for a given day?
//
// Two rules shape all of it. First, only whole-day roots are involved — a
// root is a commitment over a day and discloses nothing about any child, so
// it is safe to publish and safe to re-read. Second, an unanswerable question
// is answered with "unavailable" and the reason, never with a fabricated
// match: if no chain is configured, no root reached one, or the node does not
// reply, the response says exactly that.

// WithAnchor records how the ledger publishes its roots and where this API
// can independently read them back from. Both come from the running
// configuration, so the response describes the deployment rather than a
// default. An empty rpcURL means this API cannot ask a chain anything, which
// it then reports rather than glossing over.
func (s *Server) WithAnchor(mode, rpcURL string) *Server {
	if mode != "" {
		s.anchorMode = mode
	}
	s.anchorRPCURL = rpcURL
	return s
}

// verification statuses. Anything other than verified/mismatch is
// unavailable with a reason; there is no fourth, hopeful state.
const (
	statusVerified    = "verified"
	statusMismatch    = "mismatch"
	statusUnavailable = "unavailable"
)

// buildAnchorVerification re-reads one day's root from the chain and compares
// it with the root in the database. A database failure is returned as an
// error so the read falls back to the stale cache; a chain failure is part of
// the answer.
func (s *Server) buildAnchorVerification(ctx context.Context, day string) (*climateshieldv1.GetAnchorVerificationResponse, error) {
	leafDay, resolved, err := s.resolveAnchorDay(ctx, day)
	if err != nil {
		return nil, err
	}
	resp := &climateshieldv1.GetAnchorVerificationResponse{
		AnchorMode: s.anchorMode,
		CheckedAt:  timestamppb.Now(),
	}
	if !resolved {
		resp.Status = statusUnavailable
		resp.Reason = "no daily root exists yet: no immunization event has been committed to the ledger, so there is nothing to check against a chain."
		return resp, nil
	}

	resp.Day = leafDay.Time.Format(evm.DayLayout)
	dayWord := evm.DayBytes32(leafDay.Time)
	resp.DayBytes32 = evm.EncodeHex(dayWord[:])

	stored, err := s.q.GetDailyRoot(ctx, leafDay)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		resp.Status = statusUnavailable
		resp.Reason = fmt.Sprintf("no daily root for %s: no immunization event was committed on that day.", resp.GetDay())
		return resp, nil
	case err != nil:
		return nil, fmt.Errorf("publicapi: anchor verification: %w", err)
	}
	resp.DbRootHex = hex.EncodeToString(stored.Root)

	row, err := s.q.LatestAnchorForDay(ctx, db.LatestAnchorForDayParams{
		LeafDay: leafDay, AnchorType: anchor.TypeEVM,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		resp.Status = statusUnavailable
		if s.anchorMode == anchor.TypeLocal {
			resp.Reason = "no chain anchor for this day: this deployment runs ANCHOR_MODE=local, so roots are recorded in the anchors table of its own database and there is no independent chain copy to compare against."
			return resp, nil
		}
		resp.Reason = fmt.Sprintf(
			"no chain anchor has been recorded for %s yet: the ledger publishes locally first and then to the chain, so a root can exist here before it reaches one.", resp.GetDay())
		return resp, nil
	}
	if err != nil {
		return nil, fmt.Errorf("publicapi: anchor verification: %w", err)
	}

	// Report what the anchors row says even when the live check cannot run:
	// the recorded facts are true whether or not this API can reach a node.
	if row.ChainID != nil {
		resp.ChainId = *row.ChainID
	}
	if row.ChainLabel != nil {
		resp.ChainLabel = *row.ChainLabel
	}
	if row.ContractAddress != nil {
		resp.ContractAddress = *row.ContractAddress
	}
	if row.TxHash != nil {
		resp.TxHash = *row.TxHash
	}

	if s.anchorRPCURL == "" {
		resp.Status = statusUnavailable
		resp.Reason = fmt.Sprintf(
			"no chain RPC configured for this API: the root was anchored to the RootAnchor contract at %s on chain id %d, but this process has no ANCHOR_RPC_URL and cannot re-read it. The recorded anchor is reported above; the live check is not.",
			resp.GetContractAddress(), resp.GetChainId())
		return resp, nil
	}
	if resp.GetContractAddress() == "" {
		resp.Status = statusUnavailable
		resp.Reason = "the recorded chain anchor has no contract address, so there is nothing to call."
		return resp, nil
	}

	s.checkChain(ctx, resp, stored.Root, dayWord)
	return resp, nil
}

// checkChain performs the live read. Every failure path sets an unavailable
// status with the reason; none of them invents a root or a verdict.
func (s *Server) checkChain(
	ctx context.Context,
	resp *climateshieldv1.GetAnchorVerificationResponse,
	dbRoot []byte,
	dayWord [32]byte,
) {
	client := evm.NewClient(s.anchorRPCURL, nil)

	chainID, err := client.ChainID(ctx)
	if err != nil {
		resp.Status = statusUnavailable
		resp.Reason = "the chain node did not answer eth_chainId, so its root could not be read: " + err.Error()
		return
	}
	if chainID != resp.GetChainId() {
		resp.Status = statusUnavailable
		resp.Reason = fmt.Sprintf(
			"the node at the configured RPC reports chain id %d, but this root was anchored on chain id %d. Refusing to compare roots across two different chains.",
			chainID, resp.GetChainId())
		return
	}

	ret, err := client.EthCall(ctx, resp.GetContractAddress(), evm.PackRootOf(dayWord))
	if err != nil {
		resp.Status = statusUnavailable
		resp.Reason = "the eth_call to rootOf(day) failed, so the chain's root could not be read: " + err.Error()
		return
	}
	onChain, err := evm.UnpackBytes32(ret)
	if err != nil {
		resp.Status = statusUnavailable
		resp.Reason = "the chain answered rootOf(day) with something that is not a 32-byte root: " + err.Error()
		return
	}

	empty := [32]byte{}
	if onChain == empty {
		resp.Status = statusMismatch
		resp.ChainRootHex = hex.EncodeToString(onChain[:])
		resp.Reason = fmt.Sprintf(
			"the contract holds no root for %s. A root was anchored and recorded here, so either the chain was reset or the contract at that address is not the one that was anchored to.", resp.GetDay())
		return
	}

	resp.ChainRootHex = hex.EncodeToString(onChain[:])
	if resp.GetChainRootHex() == hex.EncodeToString(dbRoot) {
		resp.Status = statusVerified
		resp.Reason = fmt.Sprintf(
			"the RootAnchor contract at %s on chain id %d holds exactly the root this system published for %s.",
			resp.GetContractAddress(), resp.GetChainId(), resp.GetDay())
		return
	}
	resp.Status = statusMismatch
	resp.Reason = fmt.Sprintf(
		"the chain holds a different root for %s than the database does. One of the two has changed since it was anchored; the chain copy is the one this system cannot rewrite.", resp.GetDay())
}

// resolveAnchorDay turns the request's day parameter into a date. An empty
// value means the newest day that has a root; a malformed value is a client
// error, not an empty answer.
func (s *Server) resolveAnchorDay(ctx context.Context, day string) (pgtype.Date, bool, error) {
	if day != "" {
		parsed, err := time.Parse(evm.DayLayout, day)
		if err != nil {
			return pgtype.Date{}, false, errBadRequest{fmt.Sprintf("bad day %q (want YYYY-MM-DD)", day)}
		}
		return pgtype.Date{Time: parsed, Valid: true}, true, nil
	}
	days, err := s.q.ListLeafDays(ctx)
	if err != nil {
		return pgtype.Date{}, false, fmt.Errorf("publicapi: anchor verification: %w", err)
	}
	if len(days) == 0 {
		return pgtype.Date{}, false, nil
	}
	return days[len(days)-1], true, nil
}

// anchorNote states where roots were actually published, computed from the
// newest anchor rows rather than hardcoded. It is the sentence that must
// never drift from reality: when a chain is involved it names the contract,
// the chain id and what kind of chain it is; when none is, it says so.
func anchorNote(mode string, rows []db.LedgerRootSummaryRow) string {
	const localSentence = "Daily roots are recorded in the anchors table of this system's own database."
	const noChain = "No blockchain is written to by this system"

	for _, r := range rows {
		if r.AnchorType != anchor.TypeEVM {
			continue
		}
		chainID := int64(0)
		if r.ChainID != nil {
			chainID = *r.ChainID
		}
		address := ""
		if r.ContractAddress != nil {
			address = *r.ContractAddress
		}
		note := fmt.Sprintf(
			"Daily roots are published to the RootAnchor contract at %s on chain id %d (%s), and read back from it before this system reports them as anchored.",
			address, chainID, evm.Label(chainID))
		if evm.IsDevChain(chainID) {
			note += " That chain is started by this deployment's own docker compose: its history does not outlive `make down -v`, and nothing here is written to any public network."
		}
		return note
	}

	if len(rows) == 0 {
		if mode == anchor.TypeEVM {
			return "No day has been committed to the ledger yet, so nothing has been written to any chain so far. " + localSentence
		}
		return localSentence + " " + noChain + "."
	}
	if mode == anchor.TypeEVM {
		return localSentence + " " + noChain + " yet: chain anchoring is configured (ANCHOR_MODE=evm) but no root has reached the chain yet."
	}
	return localSentence + " " + noChain + "."
}
