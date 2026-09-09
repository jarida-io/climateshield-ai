-- SPDX-License-Identifier: Apache-2.0
-- Chain anchoring details. An anchor row records where one daily Merkle root
-- was published. The local anchor keeps using anchor_type = 'local' with the
-- root in `reference`; the EVM anchor adds the chain it wrote to, the
-- contract, the transaction and — the part that makes the row checkable —
-- the root it read back from the chain immediately afterwards.
--
-- Only whole-day roots ever reach a chain. No leaf, leaf hash or leaf count
-- appears in these columns or in the contract.
ALTER TABLE anchors
    ADD COLUMN chain_id bigint,
    ADD COLUMN chain_label text,
    ADD COLUMN contract_address text,
    ADD COLUMN tx_hash text,
    ADD COLUMN block_number bigint,
    ADD COLUMN root bytea,
    ADD COLUMN readback_root bytea,
    ADD COLUMN verified_at timestamptz;

-- Existing local anchors stored the root as hex in `reference`. Backfill the
-- typed column so the sweep's "is the CURRENT root already anchored?" check
-- does not re-anchor every historical day once.
UPDATE anchors
SET root = decode(reference, 'hex')
WHERE root IS NULL
  AND anchor_type = 'local'
  AND reference ~ '^[0-9a-f]{64}$';

CREATE INDEX anchors_day_type_root_idx ON anchors (leaf_day, anchor_type, root);

-- One RootAnchor contract per chain, deployed once by the ledger service on
-- first start and reused afterwards. The service verifies eth_getCode against
-- the committed runtime bytecode before trusting an address recorded here.
CREATE TABLE anchor_contracts (
    chain_id bigint PRIMARY KEY,
    address text NOT NULL,
    deploy_tx text,
    deployed_at timestamptz NOT NULL DEFAULT now()
);
