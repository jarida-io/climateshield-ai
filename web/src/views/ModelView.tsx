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
 * Proves: there are two scorers, both open to inspection; the reference data
 * behind them can be re-derived by anyone; and the published thresholds have
 * been checked against that data — including the two that fail the check.
 *
 * Everything on this page is read from GET /v1/model, /v1/risk/current and
 * /v1/climatology. No number here is typed into the page.
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
      lede="A fitted statistical baseline over a decade of reanalysis, used to flag climatological extremes. No disease model has been trained or validated."
    >
      <TileRow>
        <StatTile label="Scoring right now" value={m.activePredictor} hint={`version ${m.activeVersion}`} />
        {m.referencePeriod !== "" && (
          <StatTile
            label="Measured against"
            value={m.referenceWindows.toLocaleString()}
            hint={`${m.windowDays}-day windows, ${m.referencePeriod}`}
          />
        )}
        {m.quantileSteps > 0 && (
          <StatTile
            label="Quantile ladder"
            value={`${m.quantileSteps} points`}
            hint="per county, per month, per driver"
          />
        )}
        <StatTile
          label="Published cutoffs that can never fire"
          value={String(unreachable.length)}
          hint={unreachable.length > 0 ? "found by checking our own proposal" : "all cutoffs reachable"}
        />
      </TileRow>

      <TwoPredictors info={m} />

      <Card title="How we checked our own proposal">
        <p style={{ ...text.body, color: brand.muted, marginTop: 0 }}>
          The four cutoffs below come from the funding proposal and are unchanged in code. Each was
          then compared against every county-month in the reference record. Two of them cannot be
          reached in any monitored county, so they can never fire — a finding about the proposal,
          reported rather than quietly patched. The last column names the value each verdict was
          measured against.
        </p>
        <Table head={["Disease", "Driver", "HIGH", "MEDIUM", "Can it ever fire?", "Measured against"]}>
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
        <p style={{ ...text.small, color: brand.muted, marginBottom: 0 }}>
          Reachable is not the same as correct. The full method, the per-county firing rates and
          the recommendation are in <Code>docs/threshold-validation.md</Code>; the same finding
          fails a test in CI if it ever stops being true.
        </p>
      </Card>

      <SameWeatherBothScorers info={m} risk={risk} />

      <ClimatologyExplorer />

      <ReferenceProvenance info={m} />

      <Caveat>
        <strong>What this page does not prove.</strong> This system holds no outbreak surveillance
        data, so nothing here has been validated against disease outcomes, and no accuracy,
        sensitivity or specificity figure is reported anywhere — none exists. The exceedance number
        describes the <em>weather</em>: how unusual a forecast window is for that county and month.
        It is not the probability that an outbreak will occur. Neither scorer is machine learning
        and neither has been trained on health data.
      </Caveat>
    </Page>
  );
}

type ModelInfo = Awaited<ReturnType<typeof publicClient.getModelInfo>>;
type RiskState = ReturnType<typeof useApi<Awaited<ReturnType<typeof publicClient.getCurrentRisk>>>>;

/**
 * The two scorers, side by side, so a reader can see what each one is before
 * comparing their output. Which one is live comes from the API.
 */
function TwoPredictors({ info }: { info: ModelInfo }) {
  const active = info.activePredictor;
  return (
    <Card title="Two ways to score the same weather">
      <div style={{ display: "flex", gap: space(5), flexWrap: "wrap" }}>
        <PredictorPanel
          name="rules"
          heading="Published thresholds"
          active={active === "rules"}
          summary="The four cutoffs from the funding proposal, applied exactly as written. No fitted parameters: a value is compared with a number, and the comparison is the score."
          strength="Traceable to a published document, and identical everywhere."
          limit="A fixed cutoff calibrated for one climate does not transfer to another — two of these four cannot fire here at all."
        />
        <PredictorPanel
          name="climatology"
          heading="Reference climatology"
          active={active === "climatology"}
          summary={`Empirical distributions measured from ${info.referenceWindows.toLocaleString()} historical ${info.windowDays}-day windows, per county and per calendar month. A score is how unusual the window is for that place at that time of year.`}
          strength="Defined in every climate, and it adapts to each county's own season."
          limit="It describes weather only. Nothing was fitted to health outcomes, because this system holds none."
        />
      </div>
      {info.exceedanceRole !== "" && (
        <p style={{ ...text.body, color: brand.ink, margin: `${space(5)} 0 0` }}>
          <strong>What the exceedance figure did: </strong>
          {info.exceedanceRole}
        </p>
      )}
    </Card>
  );
}

function PredictorPanel({
  name, heading, active, summary, strength, limit,
}: {
  name: string; heading: string; active: boolean; summary: string; strength: string; limit: string;
}) {
  return (
    <div
      style={{
        flex: "1 1 320px",
        border: `1px solid ${active ? brand.purple : brand.line}`,
        background: active ? brand.purpleDim : brand.canvas,
        borderRadius: 10,
        padding: space(4),
      }}
    >
      <div style={{ display: "flex", alignItems: "baseline", gap: space(2), flexWrap: "wrap" }}>
        <span style={{ ...text.h2, color: brand.ink }}>{heading}</span>
        <span style={{ ...text.mono, color: brand.muted }}>PREDICTOR={name}</span>
        {active && (
          <span style={{ ...text.small, color: brand.purple, fontWeight: 700 }}>· scoring now</span>
        )}
      </div>
      <p style={{ ...text.body, color: brand.ink, marginBottom: space(2) }}>{summary}</p>
      <p style={{ ...text.small, color: brand.muted, margin: 0 }}><strong>Strength:</strong> {strength}</p>
      <p style={{ ...text.small, color: brand.muted, margin: 0 }}><strong>Limit:</strong> {limit}</p>
    </div>
  );
}

/**
 * The live scores, with the reference record's view of each driver value
 * beside them. Under the published thresholds the exceedance is an annotation
 * and decided nothing; the API says which case is live and that sentence is
 * rendered above the table rather than assumed here.
 */
function SameWeatherBothScorers({ info, risk }: { info: ModelInfo; risk: RiskState }) {
  if (risk.kind !== "ready") {
    return (
      <Card title="Same weather, scored and explained">
        {risk.kind === "error"
          ? <Failed what="current scores" error={risk.message} />
          : <Loading what="current scores" />}
      </Card>
    );
  }
  const scores = risk.data.scores;
  const withExceedance = scores.filter((s) => s.exceedance !== undefined);

  return (
    <Card title="Same weather, scored and explained">
      <p style={{ ...text.body, color: brand.muted, marginTop: 0 }}>
        Every county-disease pair currently scored, the number that drove it, how unusual that
        number is for the county and month, and the sentence a health officer can dispute.
        {info.exceedanceRole !== "" && <> {info.exceedanceRole}</>}
      </p>

      {withExceedance.length > 0 && (
        <div style={{ marginBottom: space(5) }}>
          <Columns
            height={170}
            unit="%"
            labelEvery={2}
            data={withExceedance.map((s) => ({
              label: `${s.area.slice(0, 3)}·${diseaseName(s.disease).slice(0, 4)}`,
              value: Number(((s.exceedance ?? 0) * 100).toFixed(2)),
              color: diseaseColor[diseaseName(s.disease)] ?? brand.purple,
              detail: `${s.area} ${diseaseName(s.disease)}: ${((s.exceedance ?? 0) * 100).toFixed(1)}% of reference windows are at least this extreme`,
            }))}
          />
          <p style={{ ...text.small, color: brand.muted, marginTop: space(2) }}>
            Shorter is rarer. Rarity is measured against the reference record, not against a
            forecast of disease.
          </p>
          <TableView
            head={["County", "Disease", "Exceedance %", "Level"]}
            rows={withExceedance.map((s) => [
              s.area, diseaseName(s.disease), ((s.exceedance ?? 0) * 100).toFixed(2), levelName(s.level),
            ])}
          />
        </div>
      )}

      <Table head={["County", "Disease", "Level", "Driver value", "Rarity here", "Why", "Scored by"]}>
        {scores.map((s, i) => (
          <tr key={`${s.area}-${s.disease}-${i}`}>
            <Td>{s.area}</Td>
            <Td>{diseaseName(s.disease)}</Td>
            <Td><Pill level={levelName(s.level)} /></Td>
            <Td mono>{s.driverValue.toFixed(1)}</Td>
            <Td mono>{s.exceedance === undefined ? "not covered" : `top ${(s.exceedance * 100).toFixed(1)}%`}</Td>
            <Td>{s.explanation === "" ? "—" : s.explanation}</Td>
            <Td mono>{s.predictor} v{s.predictorVersion}</Td>
          </tr>
        ))}
      </Table>
      <p style={{ ...text.small, color: brand.muted, marginBottom: 0 }}>
        “Not covered” means the reference record holds no distribution for that county and month,
        so no rarity is shown. Nothing is guessed to fill the gap.
      </p>
    </Card>
  );
}

/**
 * The provenance of the reference data, in enough detail to re-derive it.
 * This is the answer to "where did those quantiles come from, and how would I
 * check?" — a file, a digest, a generator and two commands.
 */
function ReferenceProvenance({ info }: { info: ModelInfo }) {
  if (info.referenceSha256 === "") return null;
  return (
    <Card title="Where the reference numbers come from">
      <Table head={["", ""]}>
        <tr><Td>Source</Td><Td>{info.referenceSource}</Td></tr>
        <tr><Td>Licence</Td><Td>{info.referenceLicence}</Td></tr>
        <tr><Td>Period</Td><Td mono>{info.referencePeriod}</Td></tr>
        <tr><Td>Windows measured</Td><Td mono>{info.referenceWindows.toLocaleString()} × {info.windowDays} days</Td></tr>
        <tr><Td>Quantile ladder</Td><Td mono>{info.quantileSteps} points per county, month and driver</Td></tr>
        <tr><Td>File</Td><Td mono>{info.referenceFile}</Td></tr>
        <tr><Td>SHA-256</Td><Td mono>{info.referenceSha256}</Td></tr>
        <tr><Td>Built by</Td><Td mono>{info.referenceGenerator}</Td></tr>
      </Table>
      <p style={{ ...text.body, color: brand.muted, marginTop: space(4) }}>
        Check the digest of the file in the repository, or rebuild it from the archive and compare:
      </p>
      <Code>make climatology-digest</Code>
      <Code>make climatology</Code>
      <p style={{ ...text.small, color: brand.muted, marginBottom: 0 }}>
        The rebuild is the only command in the repository that makes an outbound request, and it
        needs no account or key. The tier cut-points are declared choices, not fitted ones: HIGH at
        exceedance ≤ {info.highExceedance}, MEDIUM at ≤ {info.mediumExceedance}. Method, operating
        points and limitations: <Code>docs/model-card.md</Code>.
      </p>
      {info.interpretation !== "" && (
        <p style={{ ...text.small, color: brand.muted, marginBottom: 0, marginTop: space(4) }}>
          {info.interpretation}
        </p>
      )}
    </Card>
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
