// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";

import { publicClient } from "../api";
import { Columns, Lines, TableView, series } from "../charts";
import { Select, Toggle } from "../forms";
import { Caveat, Card, Failed, Loading, Page, StatTile, Table, Td, TileRow, brand, space, text } from "../ui";
import { ts, useApi } from "../useApi";

/**
 * Proves: risk reacts to weather. Shows the exact forecast window the current
 * scores were computed from, labelled with the source it was ingested from.
 */
export function ClimateView() {
  const [county, setCounty] = useState("");
  const [showTable, setShowTable] = useState(false);
  const climate = useApi(() => publicClient.getClimateSeries({}), []);

  if (climate.kind === "loading") return <Loading what="climate series" />;
  if (climate.kind === "error") return <Failed what="climate series" error={climate.message} />;
  const everything = climate.data.series;
  const all = county === "" ? everything : everything.filter((s) => s.area === county);

  const isFixture = all.some((s) => s.source === "fixture");
  const maxRain = Math.max(1, ...all.flatMap((s) => s.days.map((d) => d.precipitationMm)));

  return (
    <Page
      title="Weather driving the scores"
      lede="The 14-day forecast window each county was scored from, as ingested — not a redrawn illustration."
    >
      <Caveat>
        {isFixture ? (
          <>
            <strong>Source: committed fixtures, not live weather.</strong> This deployment is
            running <code>CLIMATE_SOURCE=fixture</code>, so the series below is the committed demo
            scenario and is identical on every machine. Set{" "}
            <code>CLIMATE_SOURCE=openmeteo</code> for live forecasts. The source label on each card
            is read back from the stored data, so this screen cannot show fixture data while
            claiming it is live.
          </>
        ) : (
          <>
            <strong>Source: live Open-Meteo forecasts.</strong> Values change with the actual
            weather, so this screen will not match the documented demo scenario.
          </>
        )}
      </Caveat>

      <TileRow>
        <StatTile label="Counties" value={String(all.length)} />
        <StatTile label="Window" value={`${all[0]?.days.length ?? 0} days`} />
        <StatTile label="Peak rainfall" value={`${maxRain.toFixed(1)} mm`} hint="highest single day across all counties" />
      </TileRow>

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
        const peak = Math.max(...s.days.map((d) => d.precipitationMm));
        return (
          <Card key={s.area} title={s.area}>
            <p style={{ ...text.small, color: brand.muted, marginTop: 0 }}>
              source <strong>{s.source}</strong> · issued {ts(s.issuedAt)} · peak {peak.toFixed(1)} mm
            </p>

            <Columns
              unit=" mm"
              height={150}
              labelEvery={3}
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
