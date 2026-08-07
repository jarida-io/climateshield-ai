// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";

import { publicClient } from "../api";
import { Columns, DistributionStrip, TableView, diseaseColor } from "../charts";
import { Select } from "../forms";
import { diseaseName, levelName } from "../map";
import { Caveat, Card, Code, Failed, Loading, Page, Pill, StatTile, Table, Td, TileRow, brand, space, text } from "../ui";
import { useApi } from "../useApi";

const COUNTIES = ["Nairobi", "Kisumu", "Mombasa", "Nakuru", "Eldoret"];
const MONTHS = [
  "January", "February", "March", "April", "May", "June",
  "July", "August", "September", "October", "November", "December",
];

/**
 * Proves: there is a scoring model, its provenance is recorded, and its
 * published thresholds have been checked against the climate record.
 */
export function ModelView() {
  const model = useApi(() => publicClient.getModelInfo({}), []);
  const risk = useApi(() => publicClient.getCurrentRisk({}), []);

  if (model.kind === "loading") return <Loading what="model information" />;
  if (model.kind === "error") return <Failed what="model information" error={model.message} />;
  const m = model.data;

  const unreachable = m.rules.filter((r) => !r.reachableInReferencePeriod);

  return (
    <Page
      title="Prediction model"
      lede="Which predictor is scoring right now, what it was measured against, and what its numbers do and do not mean."
    >
      <Caveat>
        <strong>No accuracy claim is made.</strong> This system holds no outbreak surveillance
        data, so no model here has been validated against disease outcomes, and none reports
        sensitivity, specificity or an accuracy figure. The exceedance number below describes the{" "}
        <em>weather</em> — how unusual a forecast window is for that county and month — not the
        probability that an outbreak will occur.
      </Caveat>

      <TileRow>
        <StatTile label="Active predictor" value={m.activePredictor} hint={`version ${m.activeVersion}`} />
        <StatTile label="Available" value={String(m.availablePredictors.length)} hint={m.availablePredictors.join(", ")} />
        {m.referencePeriod !== "" && (
          <StatTile label="Reference record" value={m.referencePeriod} hint={`${m.windowDays}-day windows`} />
        )}
        <StatTile
          label="Unreachable thresholds"
          value={String(unreachable.length)}
          hint={unreachable.length > 0 ? "cannot fire — see below" : "all cutoffs reachable"}
        />
      </TileRow>

      <Card title="Published thresholds, checked against the climate record">
        <p style={{ ...text.body, color: brand.muted, marginTop: 0 }}>
          These cutoffs come from the funding proposal and are unchanged. The final column is the
          result of comparing each one against every county-month in the reference record.
        </p>
        <Table head={["Disease", "Driver", "HIGH", "MEDIUM", "Reachable?", "Finding"]}>
          {m.rules.map((r) => (
            <tr key={r.disease}>
              <Td>{r.disease}</Td>
              <Td mono>{r.driver}</Td>
              <Td>{r.higherIsWorse ? "≥" : "≤"} {r.high}</Td>
              <Td>{r.higherIsWorse ? "≥" : "≤"} {r.medium}</Td>
              <Td>
                {r.reachableInReferencePeriod ? (
                  <span style={{ color: brand.low, fontWeight: 700 }}>yes</span>
                ) : (
                  <span style={{ color: brand.high, fontWeight: 700 }}>never fires</span>
                )}
              </Td>
              <Td>{r.note === "" ? "—" : r.note}</Td>
            </tr>
          ))}
        </Table>
      </Card>

      {m.interpretation !== "" && (
        <Card title="How to read the exceedance number">
          <Code>{m.interpretation}</Code>
          <p style={{ ...text.small, color: brand.muted, marginBottom: 0 }}>
            HIGH at exceedance ≤ {m.highExceedance}, MEDIUM at ≤ {m.mediumExceedance}. Reference
            data: {m.referenceSource} ({m.referenceLicence}).
          </p>
        </Card>
      )}

      <ClimatologyExplorer />

      <Card title="Current scores, with the reason for each">
        {risk.kind === "ready" && risk.data.scores.some((s) => s.exceedance !== undefined) && (
          <div style={{ marginBottom: space(5) }}>
            <p style={{ ...text.body, color: brand.muted, marginTop: 0 }}>
              How unusual each county-disease window is. Shorter is rarer, and
              rarer is what raises the tier.
            </p>
            <Columns
              height={170}
              unit="%"
              labelEvery={2}
              data={risk.data.scores
                .filter((s) => s.exceedance !== undefined)
                .map((s) => ({
                  label: `${s.area.slice(0, 3)}·${diseaseName(s.disease).slice(0, 4)}`,
                  value: Number(((s.exceedance ?? 0) * 100).toFixed(2)),
                  color: diseaseColor[diseaseName(s.disease)] ?? brand.purple,
                  detail: `${s.area} ${diseaseName(s.disease)}: ${((s.exceedance ?? 0) * 100).toFixed(1)}% of reference windows are at least this extreme`,
                }))}
            />
            <TableView
              head={["County", "Disease", "Exceedance %", "Level"]}
              rows={risk.data.scores
                .filter((s) => s.exceedance !== undefined)
                .map((s) => [s.area, diseaseName(s.disease), ((s.exceedance ?? 0) * 100).toFixed(2), levelName(s.level)])}
            />
          </div>
        )}
        {risk.kind === "ready" ? (
          <Table head={["County", "Disease", "Level", "Driver value", "Exceedance", "Why", "Scored by"]}>
            {risk.data.scores.map((s, i) => (
              <tr key={`${s.area}-${s.disease}-${i}`}>
                <Td>{s.area}</Td>
                <Td>{diseaseName(s.disease)}</Td>
                <Td><Pill level={levelName(s.level)} /></Td>
                <Td mono>{s.driverValue.toFixed(1)}</Td>
                <Td mono>{s.exceedance === undefined ? "—" : `${(s.exceedance * 100).toFixed(1)}%`}</Td>
                <Td>{s.explanation === "" ? "—" : s.explanation}</Td>
                <Td mono>{s.predictor} v{s.predictorVersion}</Td>
              </tr>
            ))}
          </Table>
        ) : (
          <Loading what="current scores" />
        )}
      </Card>
    </Page>
  );
}

/**
 * Lets a reviewer pick any county and month and see the decade of history the
 * model measures against, with the current forecast window marked on it. This
 * is the screen that answers "where does that number come from?".
 */
function ClimatologyExplorer() {
  const [area, setArea] = useState("Kisumu");
  const [month, setMonth] = useState(String(new Date().getMonth() + 1));

  const clim = useApi(
    () => publicClient.getClimatology({ area, month: Number(month) }),
    [area, month],
  );

  return (
    <Card title="Reference climatology explorer">
      <p style={{ ...text.body, color: brand.muted, marginTop: 0 }}>
        The distribution the model scores against, for any county and month.
        The bar is a decade of history; the marker is the window being scored
        now; the shaded end is the tail treated as hazardous.
      </p>

      <form
        onSubmit={(e) => e.preventDefault()}
        style={{ display: "flex", gap: space(4), flexWrap: "wrap", marginBottom: space(5) }}
      >
        <Select
          label="County" value={area} onChange={setArea}
          options={COUNTIES.map((c) => ({ value: c, label: c }))}
        />
        <Select
          label="Month" value={month} onChange={setMonth}
          options={MONTHS.map((m, i) => ({ value: String(i + 1), label: m }))}
        />
      </form>

      {clim.kind === "loading" && <Loading what="reference distribution" />}
      {clim.kind === "error" && (
        <p style={{ ...text.small, color: brand.high }}>
          No reference data for that county and month.
        </p>
      )}
      {clim.kind === "ready" && (
        <>
          <p style={{ ...text.small, color: brand.muted, marginTop: 0 }}>
            {clim.data.samples.toLocaleString()} reference windows · {clim.data.referencePeriod}
          </p>
          {clim.data.distributions.map((d) => (
            <div key={d.driver} style={{ marginBottom: space(5) }}>
              <div style={{ ...text.h2, color: brand.ink, marginBottom: space(1) }}>{d.driver}</div>
              {d.observed === undefined ? (
                <p style={{ ...text.small, color: brand.muted }}>
                  No current forecast window ingested for this county yet.
                </p>
              ) : (
                <>
                  <DistributionStrip
                    quantiles={d.quantiles}
                    value={d.observed}
                    unit={d.unit}
                    lowerTailIsHazard={d.lowerTailIsHazard}
                  />
                  {d.observedExceedance !== undefined && (
                    <p style={{ ...text.small, color: brand.muted, marginTop: space(2) }}>
                      {(d.observedExceedance * 100).toFixed(1)}% of reference windows are at
                      least this extreme.
                    </p>
                  )}
                </>
              )}
              <TableView
                head={["Percentile", `${d.driver} (${d.unit})`]}
                rows={d.quantiles.map((q, i) => [`p${d.percentileSteps[i] ?? i}`, q.toFixed(2)])}
              />
            </div>
          ))}
        </>
      )}
    </Card>
  );
}
