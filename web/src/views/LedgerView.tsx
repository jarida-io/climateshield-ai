// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";

import { publicClient } from "../api";
import { Columns, TableView } from "../charts";
import { Button, Select, TextInput } from "../forms";
import type {
  DailyRoot,
  GetAnchorVerificationResponse,
} from "../gen/climateshield/v1/public_pb";
import { Caveat, Card, Code, Failed, Loading, Page, StatTile, Table, Td, TileRow, brand, space, text } from "../ui";
import { ts, useApi } from "../useApi";

/** Where a reader of this page can reach the development chain from the host. */
const HOST_RPC_URL = "http://127.0.0.1:8545";

/** Abbreviates a 0x hex string for a tile; the full value stays in the title. */
function shortHex(h: string, keep = 8): string {
  return h.length > 2 * keep + 4 ? `${h.slice(0, keep + 2)}…${h.slice(-keep)}` : h;
}

/**
 * The day as the contract takes it: ASCII "YYYY-MM-DD" right-padded with
 * zero bytes to 32 bytes — the same encoding the ledger service uses and
 * `cast format-bytes32-string` produces. Computed here only to build the
 * copyable command; the API returns the authoritative value with a check.
 */
function dayBytes32(day: string): string {
  let hex = "";
  for (const ch of day) hex += ch.charCodeAt(0).toString(16).padStart(2, "0");
  return `0x${hex.padEnd(64, "0")}`;
}

function castCommand(contract: string, dayHex: string): string {
  return `docker compose exec anvil cast call ${contract} "rootOf(bytes32)(bytes32)" ${dayHex} --rpc-url ${HOST_RPC_URL}`;
}

/** The newest anchor across all days decides how the page describes anchoring. */
function newestAnchor(roots: DailyRoot[]): DailyRoot | undefined {
  let best: DailyRoot | undefined;
  for (const r of roots) {
    if (r.anchoredAt === undefined) continue;
    if (best?.anchoredAt === undefined || r.anchoredAt.seconds > best.anchoredAt.seconds) best = r;
  }
  return best;
}

function isDevChain(chainId: bigint): boolean {
  return chainId === 31337n || chainId === 1337n;
}

/**
 * Proves: immunization history is tamper-evident and independently checkable.
 * Every sentence about anchoring is derived from the API's data — anchor type,
 * chain id, label and note — never from a literal, so the page cannot say
 * "chain" when there is only a table, or "table" when a chain was written to.
 */
export function LedgerView() {
  const [filter, setFilter] = useState("");
  const ledger = useApi(() => publicClient.getLedgerSummary({}), []);

  if (ledger.kind === "loading") return <Loading what="ledger summary" />;
  if (ledger.kind === "error") return <Failed what="ledger summary" error={ledger.message} />;
  const l = ledger.data;
  const newest = newestAnchor(l.roots);
  const onChain = newest !== undefined && newest.anchorType === "evm";

  return (
    <Page
      title="Tamper-evident history"
      lede="Every dose recorded can be proven recorded and cannot be quietly changed — and that claim can be checked here against the running system rather than taken on trust."
    >
      <Caveat>
        <strong>Every dose recorded can be proven recorded and cannot be quietly changed.</strong> Each
        immunization event becomes a leaf; each day’s leaves fold into one Merkle root; change one byte of
        one record and the root changes. {l.anchorNote}{" "}
        {onChain && isDevChain(newest.chainId) && (
          <>
            A second copy on a development chain makes tampering detectable across two systems that this
            stack itself runs; it does not make the record public or immutable, and this page does not claim
            that.{" "}
          </>
        )}
        Individual leaves are <strong>never published</strong>: a leaf is a per-child HMAC, and putting one on
        a public page would be a per-child artifact on a public surface. Only whole-day roots appear below.
      </Caveat>

      <TileRow>
        <StatTile label="Days committed" value={String(l.totalDays)} />
        <StatTile
          label="Chain"
          value={onChain ? `id ${String(newest.chainId)}` : "none"}
          hint={
            onChain
              ? newest.chainLabel
              : l.anchorMode === "evm"
                ? "ANCHOR_MODE=evm is configured; no root has reached the chain yet"
                : "database table only — no chain configured"
          }
        />
        <StatTile
          label="Contract"
          value={onChain ? shortHex(newest.contractAddress) : "—"}
          hint={onChain ? "RootAnchor, bytecode verified with eth_getCode before use" : `anchor mode: ${l.anchorMode}`}
        />
        <StatTile
          label="Latest anchor tx"
          value={onChain ? shortHex(newest.txHash) : "—"}
          hint={onChain ? `block ${String(newest.blockNumber)} · ${newest.day}` : "local anchors have no transaction"}
        />
      </TileRow>

      <VerifyCard roots={l.roots} anchorMode={l.anchorMode} />

      <Card title="How this is tamper-evident">
        <ol style={{ ...text.body, color: brand.ink, margin: 0, paddingLeft: space(5), lineHeight: 1.7 }}>
          <li>Each immunization event is serialised deterministically — fixed field order, UTC.</li>
          <li>
            It is hashed with <strong>HMAC-SHA256 under a key unique to that child</strong>, held
            in a separate database schema from the event data.
          </li>
          <li>
            Each day’s leaves are folded into a Merkle tree. The root is stored in the anchors table and,
            when a chain is configured, sent to the RootAnchor contract and <strong>read back</strong> before
            the anchor is reported — a receipt alone is not treated as proof.
          </li>
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
        <Table head={["Day", "Merkle root", "Leaves", "Anchor", "Chain tx", "Read-back", "Anchored at"]}>
          {l.roots.filter((r) => filter === "" || r.day.includes(filter)).map((r) => (
            <tr key={r.day}>
              <Td mono>{r.day}</Td>
              <Td mono>
                <span title={r.rootHex}>{r.rootHex.slice(0, 32)}…</span>
              </Td>
              <Td mono>{r.leafCountSuppressed ? "withheld (k<10)" : String(r.leafCount ?? 0n)}</Td>
              <Td mono>{r.anchorType === "" ? "—" : r.anchorType}</Td>
              <Td mono>
                {r.txHash === "" ? "—" : <span title={`${r.txHash} · block ${String(r.blockNumber)}`}>{shortHex(r.txHash, 6)}</span>}
              </Td>
              <Td mono>{r.anchorType === "evm" ? (r.readbackMatches ? "matches" : "no match") : "—"}</Td>
              <Td mono>{ts(r.anchoredAt)}</Td>
            </tr>
          ))}
        </Table>
        <p style={{ ...text.small, color: brand.muted, marginBottom: 0, marginTop: space(3) }}>
          Leaf counts are suppressed below 10 because on a quiet day the count would say how many
          identifiable families attended a clinic. The root itself is safe to publish: it is a
          commitment over the whole day and discloses nothing about any individual. Only the newest
          anchor per day is shown; how many times a day was re-anchored would track late records.
        </p>
      </Card>
    </Page>
  );
}

type VerifyState =
  | { kind: "idle" }
  | { kind: "running" }
  | { kind: "done"; result: GetAnchorVerificationResponse }
  | { kind: "error"; message: string };

const statusColor: Record<string, string> = {
  verified: brand.low,
  mismatch: brand.high,
  unavailable: brand.medium,
};

/**
 * Asks the API to check one day's root against the chain LIVE and shows both
 * roots side by side. The result is whatever the chain answered — verified,
 * mismatch, or unavailable with the reason — never a decoration.
 */
function VerifyCard({ roots, anchorMode }: { roots: DailyRoot[]; anchorMode: string }) {
  const [day, setDay] = useState(roots[0]?.day ?? "");
  const [state, setState] = useState<VerifyState>({ kind: "idle" });

  const selected = roots.find((r) => r.day === day);
  const contractForCommand =
    state.kind === "done" && state.result.contractAddress !== ""
      ? state.result.contractAddress
      : selected?.anchorType === "evm"
        ? selected.contractAddress
        : "";
  const dayHex = state.kind === "done" && state.result.dayBytes32 !== "" ? state.result.dayBytes32 : dayBytes32(day);

  const run = () => {
    setState({ kind: "running" });
    publicClient
      .getAnchorVerification({ day })
      .then((result) => setState({ kind: "done", result }))
      .catch((err: unknown) => setState({ kind: "error", message: String(err) }));
  };

  return (
    <Card title="Verify on chain now">
      <p style={{ ...text.body, color: brand.muted, marginTop: 0 }}>
        The API calls <code>rootOf(day)</code> on the RootAnchor contract at the moment you click and
        compares the answer with the root in its database. Nothing is cached and nothing is assumed:
        if the chain is not configured or does not answer, the result says so.
        {anchorMode !== "evm" && (
          <>
            {" "}
            This deployment runs <code>ANCHOR_MODE={anchorMode}</code>, so the check will report
            “unavailable” and explain why.
          </>
        )}
      </p>
      <form
        onSubmit={(e) => e.preventDefault()}
        style={{ display: "flex", gap: space(4), flexWrap: "wrap", alignItems: "flex-end", marginBottom: space(4) }}
      >
        <Select
          label="Day"
          value={day}
          onChange={(v) => {
            setDay(v);
            setState({ kind: "idle" });
          }}
          options={roots.map((r) => ({ value: r.day, label: `${r.day}${r.anchorType === "" ? "" : ` · ${r.anchorType}`}` }))}
        />
        <Button kind="primary" onClick={run}>
          {state.kind === "running" ? "Checking…" : "Verify on chain now"}
        </Button>
      </form>

      {state.kind === "error" && (
        <p style={{ ...text.body, color: brand.high }}>The request failed: {state.message}</p>
      )}

      {state.kind === "done" && (
        <div style={{ display: "flex", flexDirection: "column", gap: space(3) }}>
          <div style={{ display: "flex", gap: space(3), alignItems: "center", flexWrap: "wrap" }}>
            <span
              style={{
                ...text.small,
                fontWeight: 700,
                color: "#fff",
                background: statusColor[state.result.status] ?? brand.muted,
                borderRadius: 999,
                padding: "2px 10px",
              }}
            >
              {state.result.status}
            </span>
            <span style={{ ...text.body, color: brand.ink }}>{state.result.reason}</span>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))", gap: space(3) }}>
            <RootBox label="Root in the database" value={state.result.dbRootHex} />
            <RootBox label="Root read from the chain" value={state.result.chainRootHex} />
          </div>
          <dl style={{ ...text.small, color: brand.muted, margin: 0, display: "grid", gridTemplateColumns: "max-content 1fr", gap: `${space(1)} ${space(4)}` }}>
            <dt>Chain</dt>
            <dd style={{ margin: 0 }}>
              {state.result.chainId === 0n ? "—" : `id ${String(state.result.chainId)} — ${state.result.chainLabel}`}
            </dd>
            <dt>Contract</dt>
            <dd style={{ margin: 0, ...text.mono }}>{state.result.contractAddress === "" ? "—" : state.result.contractAddress}</dd>
            <dt>Anchor tx</dt>
            <dd style={{ margin: 0, ...text.mono }}>{state.result.txHash === "" ? "—" : state.result.txHash}</dd>
            <dt>Checked at</dt>
            <dd style={{ margin: 0 }}>{ts(state.result.checkedAt)}</dd>
          </dl>
        </div>
      )}

      {contractForCommand !== "" && (
        <div style={{ marginTop: space(4) }}>
          <p style={{ ...text.small, color: brand.muted, marginBottom: space(2) }}>
            Repeat the check yourself from the host running this stack. The answer should equal the
            database root above.
          </p>
          <div style={{ display: "flex", gap: space(3), alignItems: "flex-start", flexWrap: "wrap" }}>
            <div style={{ flex: "1 1 400px", minWidth: 0 }}>
              <Code>{castCommand(contractForCommand, dayHex)}</Code>
            </div>
            <CopyButton value={castCommand(contractForCommand, dayHex)} />
          </div>
        </div>
      )}
    </Card>
  );
}

function RootBox({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ border: `1px solid ${brand.line}`, borderRadius: 8, padding: space(3), minWidth: 0 }}>
      <div style={{ ...text.small, color: brand.muted, marginBottom: space(1) }}>{label}</div>
      <div style={{ ...text.mono, color: brand.ink, wordBreak: "break-all" }}>{value === "" ? "—" : value}</div>
    </div>
  );
}

function CopyButton({ value }: { value: string }) {
  const [done, setDone] = useState<"idle" | "copied" | "failed">("idle");
  const copy = () => {
    try {
      const clipboard = typeof navigator === "undefined" ? undefined : navigator.clipboard;
      if (clipboard === undefined) {
        setDone("failed");
        return;
      }
      clipboard
        .writeText(value)
        .then(() => setDone("copied"))
        .catch(() => setDone("failed"));
    } catch {
      setDone("failed");
    }
  };
  return (
    <Button onClick={copy}>
      {done === "copied" ? "Copied" : done === "failed" ? "Copy failed — select the text" : "Copy command"}
    </Button>
  );
}
