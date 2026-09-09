// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from "react";

import { publicClient } from "../api";
import { Columns, TableView } from "../charts";
import { Toggle } from "../forms";
import { Caveat, Card, Disclosure, Failed, Loading, Page, StatTile, Table, Td, TileRow, brand, levelColor, space, text } from "../ui";
import { ts, useApi } from "../useApi";

/**
 * Colour by what the state means, not by whether it says "completed".
 *
 * Painting every non-completed state red made a queue of ten *scheduled*
 * alert jobs look like ten failures — a red bar on a health dashboard is a
 * claim, and that one was not true. Only retryable and discarded are trouble.
 */
function jobColor(state: string): string {
  if (state === "completed") return levelColor["LOW"] ?? brand.low;
  if (state === "retryable" || state === "discarded") return levelColor["HIGH"] ?? brand.high;
  return brand.purple;
}

/**
 * Proves: the system runs unattended. Every row here is a job the scheduler
 * ran on its own — nobody pressed a button.
 */
export function PipelineView() {
  const [live, setLive] = useState(false);
  const [tick, setTick] = useState(0);

  // Auto-refresh makes the "it runs on its own" claim checkable in the room:
  // leave it on and the counts move without anyone touching anything.
  useEffect(() => {
    if (!live) return;
    const id = setInterval(() => setTick((t) => t + 1), 5000);
    return () => clearInterval(id);
  }, [live]);

  const pipeline = useApi(() => publicClient.getPipelineStatus({}), [tick]);

  if (pipeline.kind === "loading") return <Loading what="pipeline status" />;
  if (pipeline.kind === "error") return <Failed what="pipeline status" error={pipeline.message} />;
  const p = pipeline.data;

  const completed = p.jobs.filter((j) => j.state === "completed").reduce((n, j) => n + Number(j.count), 0);
  const failing = p.jobs.filter((j) => j.state === "retryable" || j.state === "discarded");

  return (
    <Page
      title="It keeps running when nobody is watching"
      lede="Nobody presses a button here. Every row below is a job the scheduler ran on its own — what ran, how often, and when it last finished."
    >
      {failing.length > 0 && (
        <Caveat>
          <strong>
            {failing.reduce((n, j) => n + Number(j.count), 0)} job
            {failing.reduce((n, j) => n + Number(j.count), 0) === 1 ? "" : "s"} in this deployment
            are retrying or were discarded.
          </strong>{" "}
          They are listed below with their kind and state. Figures on other views may be behind
          until those jobs succeed.
        </Caveat>
      )}

      <Disclosure>
        These are Postgres-backed jobs with real retry history, not a cron script assumed to have
        worked. Ingestion runs every {p.ingestInterval} and fans out to scoring and alerting on its
        own. <strong>What this does not show</strong> is uptime over time: this deployment has been
        running for minutes, not months, and no availability figure is claimed anywhere.
      </Disclosure>

      <TileRow>
        <StatTile label="Jobs completed" value={String(completed)} />
        <StatTile label="Climate observations" value={String(p.climateObservations)} />
        <StatTile label="Risk scores" value={String(p.riskScores)} />
        <StatTile label="Ingest interval" value={p.ingestInterval} />
      </TileRow>

      {failing.length > 0 && (
        <Card title="Jobs needing attention">
          <Table head={["Kind", "State", "Count"]}>
            {failing.map((j) => (
              <tr key={`${j.kind}-${j.state}`}>
                <Td mono>{j.kind}</Td>
                <Td mono>
                  <span style={{ color: brand.high, fontWeight: 700 }}>{j.state}</span>
                </Td>
                <Td mono>{String(j.count)}</Td>
              </tr>
            ))}
          </Table>
        </Card>
      )}

      <Card title="Jobs run, by kind">
        <Columns
          height={160}
          data={p.jobs.map((j) => ({
            label: j.kind.replace(/_/g, " "),
            value: Number(j.count),
            color: jobColor(j.state),
            detail: `${j.kind} (${j.state}): ${j.count} run${Number(j.count) === 1 ? "" : "s"}`,
          }))}
        />
        <TableView
          head={["Kind", "State", "Count"]}
          rows={p.jobs.map((j) => [j.kind, j.state, String(j.count)])}
        />
      </Card>

      <Card title="Job history">
        <div style={{ marginBottom: space(3) }}>
          <Toggle label="Auto-refresh every 5 seconds" checked={live} onChange={setLive} />
        </div>
        {p.jobs.length === 0 ? (
          <p style={{ ...text.body, color: brand.muted, margin: 0 }}>
            No jobs have run yet in this deployment.
          </p>
        ) : (
          <Table head={["Kind", "State", "Count", "Last finished"]}>
            {p.jobs.map((j) => (
              <tr key={`${j.kind}-${j.state}`}>
                <Td mono>{j.kind}</Td>
                <Td mono>{j.state}</Td>
                <Td mono>{String(j.count)}</Td>
                <Td mono>{ts(j.lastFinishedAt)}</Td>
              </tr>
            ))}
          </Table>
        )}
        <p style={{ ...text.small, color: brand.muted, marginTop: space(3), marginBottom: 0 }}>
          Latest forecast ingested: {ts(p.latestObservationAt)}. The chain is{" "}
          <code>climate_ingest → risk_predict → alert_dispatch</code>, plus a periodic ledger
          sweep.
        </p>
      </Card>
    </Page>
  );
}
