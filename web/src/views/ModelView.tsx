// SPDX-License-Identifier: Apache-2.0

import { publicClient } from "../api";
import { diseaseName, levelName } from "../map";
import { Caveat, Card, Code, Failed, Loading, Page, Pill, StatTile, Table, Td, TileRow, brand, text } from "../ui";
import { useApi } from "../useApi";

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

      <Card title="Current scores, with the reason for each">
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
