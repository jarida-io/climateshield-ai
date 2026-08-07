// SPDX-License-Identifier: Apache-2.0

import { publicClient } from "../api";
import { Caveat, Card, Failed, Loading, Page, StatTile, Table, Td, TileRow, brand, space, text } from "../ui";
import { ts, useApi } from "../useApi";

/**
 * Proves: the system runs unattended. Every row here is a job the scheduler
 * ran on its own — nobody pressed a button.
 */
export function PipelineView() {
  const pipeline = useApi(() => publicClient.getPipelineStatus({}), []);

  if (pipeline.kind === "loading") return <Loading what="pipeline status" />;
  if (pipeline.kind === "error") return <Failed what="pipeline status" error={pipeline.message} />;
  const p = pipeline.data;

  const completed = p.jobs.filter((j) => j.state === "completed").reduce((n, j) => n + Number(j.count), 0);
  const failing = p.jobs.filter((j) => j.state === "retryable" || j.state === "discarded");

  return (
    <Page
      title="Continuous operation"
      lede="Durable job history from the queue: what ran, how often, and when it last finished."
    >
      <Caveat>
        These are Postgres-backed jobs with real retry history, not a cron script assumed to have
        worked. Ingestion runs every {p.ingestInterval} and fans out to scoring and alerting on its
        own. <strong>What this does not show</strong> is uptime over time: this deployment has been
        running for minutes, not months, and no availability figure is claimed anywhere.
      </Caveat>

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

      <Card title="Job history">
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
