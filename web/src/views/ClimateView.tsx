// SPDX-License-Identifier: Apache-2.0

import { publicClient } from "../api";
import { Caveat, Card, Failed, Loading, Page, StatTile, Table, Td, TileRow, brand, space, text } from "../ui";
import { ts, useApi } from "../useApi";

/**
 * Proves: risk reacts to weather. Shows the exact forecast window the current
 * scores were computed from, labelled with the source it was ingested from.
 */
export function ClimateView() {
  const series = useApi(() => publicClient.getClimateSeries({}), []);

  if (series.kind === "loading") return <Loading what="climate series" />;
  if (series.kind === "error") return <Failed what="climate series" error={series.message} />;
  const all = series.data.series;

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

      {all.map((s) => {
        const peak = Math.max(...s.days.map((d) => d.precipitationMm));
        return (
          <Card key={s.area} title={s.area}>
            <p style={{ ...text.small, color: brand.muted, marginTop: 0 }}>
              source <strong>{s.source}</strong> · issued {ts(s.issuedAt)} · peak {peak.toFixed(1)} mm
            </p>

            {/* Bar chart of daily rainfall: the driver a reviewer most wants
                to see next to a cholera or malaria score. */}
            <div
              style={{ display: "flex", alignItems: "flex-end", gap: 3, height: 90, marginBottom: space(3) }}
              role="img"
              aria-label={`Daily rainfall for ${s.area}, peak ${peak.toFixed(1)} millimetres`}
            >
              {s.days.map((d) => (
                <div
                  key={d.date}
                  title={`${d.date}: ${d.precipitationMm.toFixed(1)} mm`}
                  style={{
                    flex: 1,
                    height: `${Math.max(2, (d.precipitationMm / maxRain) * 100)}%`,
                    background: brand.purple,
                    borderRadius: "2px 2px 0 0",
                    minWidth: 6,
                  }}
                />
              ))}
            </div>

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
          </Card>
        );
      })}
    </Page>
  );
}
