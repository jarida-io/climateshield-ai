// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";

import { publicClient } from "../api";
import { Columns, TableView } from "../charts";
import { TextInput } from "../forms";
import { Caveat, Card, Failed, Loading, Page, StatTile, Table, Td, TileRow, brand, space, text } from "../ui";
import { ts, useApi } from "../useApi";

/**
 * Proves: immunization history is tamper-evident and independently checkable.
 * Deliberately does NOT say "blockchain" — none exists in this system.
 */
export function LedgerView() {
  const [filter, setFilter] = useState("");
  const ledger = useApi(() => publicClient.getLedgerSummary({}), []);

  if (ledger.kind === "loading") return <Loading what="ledger summary" />;
  if (ledger.kind === "error") return <Failed what="ledger summary" error={ledger.message} />;
  const l = ledger.data;

  return (
    <Page
      title="Tamper-evident history"
      lede="Daily commitments over the immunization record, published so they can be checked without trusting us."
    >
      <Caveat>
        <strong>There is no blockchain in this system.</strong> {l.anchorNote} The design keeps
        chain anchoring one implementation away, but nothing here has been written to a chain and
        this screen does not claim otherwise. Separately: individual leaves are{" "}
        <strong>never published</strong>. A leaf is a per-child HMAC, and putting one on a public
        page would be a per-child artifact on a public surface. Only whole-day roots appear below.
      </Caveat>

      <TileRow>
        <StatTile label="Days committed" value={String(l.totalDays)} />
        <StatTile label="Anchor" value={l.roots[0]?.anchorType === "" ? "pending" : (l.roots[0]?.anchorType ?? "—")} hint="local table, not a chain" />
        <StatTile label="Construction" value="Merkle" hint="RFC 6962" />
      </TileRow>

      <Card title="How this is tamper-evident">
        <ol style={{ ...text.body, color: brand.ink, margin: 0, paddingLeft: space(5), lineHeight: 1.7 }}>
          <li>Each immunization event is serialised deterministically — fixed field order, UTC.</li>
          <li>
            It is hashed with <strong>HMAC-SHA256 under a key unique to that child</strong>, held
            in a separate database schema from the event data.
          </li>
          <li>Each day’s leaves are folded into a Merkle tree and the root is stored and anchored.</li>
          <li>
            Anyone holding a leaf and its audit path can verify inclusion against a published root
            without seeing any other record. Change one byte of one event and the root changes.
          </li>
          <li>
            Erasure still works: destroying a child’s key makes their leaves permanently
            unlinkable, while previously published roots continue to verify.
          </li>
        </ol>
      </Card>

      {l.roots.some((r) => !r.leafCountSuppressed) && (
        <Card title="Events committed per day">
          <p style={{ ...text.body, color: brand.muted, marginTop: 0 }}>
            Days where the count is below the suppression threshold are omitted
            from this chart rather than shown as zero — absence here means
            withheld, not empty.
          </p>
          <Columns
            height={150}
            unit=" leaves"
            data={l.roots
              .filter((r) => !r.leafCountSuppressed)
              .slice(0, 30)
              .reverse()
              .map((r) => ({
                label: r.day.slice(5),
                value: Number(r.leafCount ?? 0n),
                detail: `${r.day}: ${String(r.leafCount ?? 0n)} leaves committed`,
              }))}
          />
          <TableView
            head={["Day", "Leaves"]}
            rows={l.roots
              .filter((r) => !r.leafCountSuppressed)
              .map((r) => [r.day, String(r.leafCount ?? 0n)])}
          />
        </Card>
      )}

      <Card title={`Daily roots (${l.algorithm})`}>
        <form
          onSubmit={(e) => e.preventDefault()}
          style={{ display: "flex", gap: space(4), flexWrap: "wrap", marginBottom: space(4) }}
        >
          <TextInput
            label="Find a day"
            value={filter}
            onChange={setFilter}
            placeholder="2026-08-07"
            hint="filters the roots below by date"
          />
        </form>
        <Table head={["Day", "Merkle root", "Leaves", "Anchor", "Anchored at"]}>
          {l.roots.filter((r) => filter === "" || r.day.includes(filter)).map((r) => (
            <tr key={r.day}>
              <Td mono>{r.day}</Td>
              <Td mono>
                <span title={r.rootHex}>{r.rootHex.slice(0, 32)}…</span>
              </Td>
              <Td mono>{r.leafCountSuppressed ? "withheld (k<10)" : String(r.leafCount ?? 0n)}</Td>
              <Td mono>{r.anchorType === "" ? "—" : r.anchorType}</Td>
              <Td mono>{ts(r.anchoredAt)}</Td>
            </tr>
          ))}
        </Table>
        <p style={{ ...text.small, color: brand.muted, marginBottom: 0, marginTop: space(3) }}>
          Leaf counts are suppressed below 10 because on a quiet day the count would say how many
          identifiable families attended a clinic. The root itself is safe to publish: it is a
          commitment over the whole day and discloses nothing about any individual.
        </p>
      </Card>
    </Page>
  );
}
