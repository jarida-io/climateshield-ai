// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jarida-io/climateshield/internal/ledger/anchor"
	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm"
	"github.com/jarida-io/climateshield/internal/store/db"
)

// reportAnchor prints where a day's root was published, read back from the
// anchors row the ledger actually wrote — never from configuration. When a
// chain was involved it names the chain and says what kind of chain it is;
// when none was, it says that instead. The demo must not be able to imply an
// anchoring that did not happen, which is the same rule the mock channel and
// the climate source label already obey.
func reportAnchor(ctx context.Context, q *db.Queries, day pgtype.Date, root []byte) error {
	chainRow, chainErr := q.LatestAnchorForDay(ctx, db.LatestAnchorForDayParams{
		LeafDay: day, AnchorType: anchor.TypeEVM,
	})
	if chainErr != nil {
		if !isNoRows(chainErr) {
			return chainErr
		}
		// No chain anchor: say what did happen, which is the local row.
		if _, err := q.LatestAnchorForDay(ctx, db.LatestAnchorForDayParams{
			LeafDay: day, AnchorType: anchor.TypeLocal,
		}); err != nil {
			if isNoRows(err) {
				fmt.Println("    anchored: not yet — the sweep has not published this root anywhere")
				return nil
			}
			return err
		}
		fmt.Println("    anchored: local (a row in this system's own database — no chain was written to)")
		return nil
	}

	chainID := int64(0)
	if chainRow.ChainID != nil {
		chainID = *chainRow.ChainID
	}
	address, txHash := "", ""
	if chainRow.ContractAddress != nil {
		address = *chainRow.ContractAddress
	}
	if chainRow.TxHash != nil {
		txHash = *chainRow.TxHash
	}
	block := int64(0)
	if chainRow.BlockNumber != nil {
		block = *chainRow.BlockNumber
	}

	fmt.Printf("    anchored: chain id %d (%s)\n", chainID, evm.Label(chainID))
	fmt.Printf("              contract %s\n", address)
	fmt.Printf("              tx %s in block %d\n", txHash, block)

	// The read-back is the part that matters: the ledger asked the chain what
	// root it holds for this day and compared it with the one it published.
	switch {
	case len(chainRow.ReadbackRoot) == 0:
		fmt.Println("              read-back: not recorded")
	case hex.EncodeToString(chainRow.ReadbackRoot) == hex.EncodeToString(root):
		fmt.Println("              read-back rootOf(day) == database root: OK")
	default:
		fmt.Printf("              read-back rootOf(day) DIFFERS from the database root (%s… on chain)\n",
			hex.EncodeToString(chainRow.ReadbackRoot)[:16])
	}

	dayWord := evm.DayBytes32(day.Time)
	fmt.Println("    check it yourself, without trusting this program:")
	fmt.Printf("      docker compose exec anvil cast call %s \"rootOf(bytes32)(bytes32)\" %s --rpc-url http://127.0.0.1:8545\n",
		address, evm.EncodeHex(dayWord[:]))
	return nil
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
