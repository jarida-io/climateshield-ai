// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";

import { publicClient } from "../api";
import type { Threshold } from "../charts";
import { Columns, Lines, TableView, series } from "../charts";
import { Select, Toggle } from "../forms";
import type { GetModelInfoResponse } from "../gen/climateshield/v1/public_pb";
import { Card, Disclosure, Failed, Loading, Page, StatTile, Table, Td, TileRow, brand, space, text } from "../ui";
import { dataOf, ts, useApi } from "../useApi";

/** The driver name the rules use for rainfall, as published on /v1/model. */
const RAIN_DRIVER = "peak_rainfall_mm_14d";

/**
 * The rainfall cutoffs to draw, read from the API's published rules.
 *
 * The numbers are contractual — they live in one place in the Go code and are
 * served from there. Typing 60 into this file would create a second copy that
 * nothing keeps in step, so if the rules do not load, no line is drawn.
 */
function rainThresholds(model: GetModelInfoResponse | undefined): Threshold[] {
  return (model?.rules ?? [])
    .filter((r) => r.driver === RAIN_DRIVER && r.higherIsWorse)
    .map((r) => ({ value: r.high, label: `${r.high} mm ${r.disease} HIGH` }))
    .sort((a, b) => a.value - b.value);
}

/**
 * Proves: risk reacts to weather. Shows the exact forecast window the current
 * scores were computed from, labelled with the source it was ingested from.
 */
export function ClimateView() {
  const [county, setCounty] = useState("");
  const [showTable, setShowTable] = useState(false);
  const climate = useApi(() => publicClient.getClimateSeries({}), []);
  const model = useApi(() => publicClient.getModelInfo({}), []);

  if (climate.kind === "loading") return <Loading what="climate series" />;
  if (climate.kind === "error") return <Failed what="climate series" error={climate.message} />;
  const everything = climate.data.series;
  const all = county === "" ? everything : everything.filter((s) => s.area === county);

  const isFixture = all.some((s) => s.source === "fixture");
  const cutoffs = rainThresholds(dataOf(model));

  // The wettest single day in view, and where and when it fell — so the peak
  // tile can say what it means instead of showing a bare number.
  let peak: { area: string; date: string; mm: number } | undefined;
  for (const s of all) {
    for (const d of s.days) {
      if (peak === undefined || d.precipitationMm > peak.mm) {
        peak = { area: s.area, date: d.date, mm: d.precipitationMm };
      }
    }
  }
  const crossed = peak === undefined ? [] : cutoffs.filter((c) => (peak?.mm ?? 0) >= c.value);
  const highest = crossed[crossed.length - 1];

  return (
    <Page
      title="The weather these scores came from"
      lede="Risk here reacts to the forecast, not to the calendar. This is the 14-day window each county was scored on, exactly as it was ingested — not a redrawn illustration."
    >
      <TileRow>
        <StatTile label="Counties" value={String(all.length)} />
        <StatTile label="Window" value={`${all[0]?.days.length ?? 0} days`} />
        <StatTile
          label="Wettest day in view"
          value={peak === undefined ? "—" : `${peak.mm.toFixed(1)} mm`}
          hint={
            peak === undefined
              ? "no daily values in this selection"
              : highest === undefined
                ? `${peak.area} on ${peak.date}${cutoffs.length === 0 ? "" : ` — below every published rainfall cutoff`}`
                : `${peak.area} on ${peak.date} — at or above the ${highest.label} cutoff`
          }
        />
      </TileRow>

      <Disclosure>
        {isFixture ? (
          <>
            <strong>These are committed demo fixtures, not live weather.</strong> This deployment
            runs <code>CLIMATE_SOURCE=fixture</code>, so the series below is the committed demo
            scenario and is identical on every machine — which is what makes the demonstration
            reproducible, and also means it is not today’s forecast. Set{" "}
            <code>CLIMATE_SOURCE=openmeteo</code> for live forecasts. The source label on each card
            is read back from the stored data, so this screen cannot show fixture data while
            claiming it is live.
          </>
        ) : (
          <>
            <strong>These are live Open-Meteo forecasts.</strong> Values change with the actual
            weather, so this screen will not match the documented demo scenario.
          </>
        )}{" "}
        {cutoffs.length > 0 && (
          <>
            The dashed lines on the rainfall charts are the published HIGH cutoffs, read from{" "}
            <code>/v1/model</code> rather than typed into this page.
          </>
        )}
      </Disclosure>

      <form
        onSubmit={(e) => e.preventDefault()}
        style={{
          display: "flex", gap: space(4), flexWrap: "wrap", alignItems: "flex-end",
          padding: space(4), background: brand.surface,
          border: `1px solid ${brand.line}`, borderRadius: 10, marginBottom: space(5),
        }}
      >
        <Select
          label="County"
          value={county}
          onChange={setCounty}
          options={[
            { value: "", label: "All counties" },
            ...everything.map((s) => ({ value: s.area, label: s.area })),
          ]}
        />
        <Toggle label="Show daily values as a table" checked={showTable} onChange={setShowTable} />
      </form>

      {all.map((s) => {
        const countyPeak = Math.max(...s.days.map((d) => d.precipitationMm));
        return (
          <Card key={s.area} title={s.area}>
            <p style={{ ...text.small, color: brand.muted, marginTop: 0 }}>
              source <strong>{s.source}</strong> · issued {ts(s.issuedAt)} · peak{" "}
              {countyPeak.toFixed(1)} mm
            </p>

            <Columns
              unit=" mm"
              height={150}
              labelEvery={3}
              threshold={cutoffs}
              data={s.days.map((d) => ({
                label: d.date.slice(5),
                value: d.precipitationMm,
                detail: `${d.date}: ${d.precipitationMm.toFixed(1)} mm of rain`,
              }))}
            />
            <TableView
              head={["Date", "Rain (mm)", "Max °C", "Min °C"]}
              rows={s.days.map((d) => [d.date, d.precipitationMm.toFixed(1), d.tempMaxC.toFixed(1), d.tempMinC.toFixed(1)])}
            />

            <div style={{ marginTop: space(4) }}>
              <div style={{ ...text.small, color: brand.muted, marginBottom: space(2) }}>
                Daily temperature range — the minimum line is what cold-stress
                risk is measured on.
              </div>
              <Lines
                unit="°C"
                data={[
                  {
                    name: "Daily maximum",
                    color: series[1],
                    points: s.days.map((d) => ({ label: d.date, value: d.tempMaxC })),
                  },
                  {
                    name: "Daily minimum",
                    color: series[0],
                    points: s.days.map((d) => ({ label: d.date, value: d.tempMinC })),
                  },
                ]}
              />
            </div>

            {showTable && (
              <div style={{ marginTop: space(4) }}>
                <Table head={["Date", "Rain (mm)", "Max °C", "Min °C"]}>
                  {s.days.map((d) => (
                    <tr key={d.date}>
                      <Td mono>{d.date}</Td>
                      <Td mono>{d.precipitationMm.toFixed(1)}</Td>
                      <Td mono>{d.tempMaxC.toFixed(1)}</Td>
                      <Td mono>{d.tempMinC.toFixed(1)}</Td>
                    </tr>
                  ))}
                </Table>
              </div>
            )}
          </Card>
        );
      })}
    </Page>
  );
}
