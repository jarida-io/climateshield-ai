// SPDX-License-Identifier: Apache-2.0

import { publicClient } from "../api";
import { diseaseName, levelName } from "../map";
import { brand, levelColor, space, text } from "../ui";
import { dataOf, useApi } from "../useApi";

/**
 * The opening image of the whole dashboard: the fourteen days of rain that a
 * county is actually being scored on, with the cutoff published in the funding
 * proposal drawn across them.
 *
 * This is the system in one picture — weather crosses a line somebody wrote
 * down in advance, and a child's guardian is prompted. It is drawn entirely
 * from live responses: the window comes from GET /v1/climate/series, the
 * cutoff from GET /v1/model, the level from GET /v1/risk/current. Nothing here
 * is illustrative, and if any of the three does not answer the component
 * renders nothing rather than a plausible-looking shape.
 */
export function ForecastHero() {
  const risk = useApi(() => publicClient.getCurrentRisk({}), []);
  const model = useApi(() => publicClient.getModelInfo({}), []);
  const climate = useApi(() => publicClient.getClimateSeries({}), []);

  const scores = dataOf(risk)?.scores ?? [];
  const rules = dataOf(model)?.rules ?? [];
  const series = dataOf(climate)?.series ?? [];

  // The county and disease this opens on is whichever rainfall-driven pair is
  // worst right now — not a county chosen in advance. On a quiet day it will
  // honestly open on a LOW one.
  const rainScores = scores
    .filter((s) => s.driver === "peak_rainfall_mm_14d")
    .sort((a, b) => b.level - a.level || b.driverValue - a.driverValue);
  const lead = rainScores[0];
  if (lead === undefined) return null;

  const rule = rules.find((r) => r.disease === diseaseName(lead.disease));
  const county = series.find((s) => s.area === lead.area);
  if (rule === undefined || county === undefined || county.days.length === 0) return null;

  const cutoff = rule.high;
  const days = county.days;
  const peak = Math.max(...days.map((d) => d.precipitationMm));
  // The scale must clear both the tallest bar and the cutoff, or a line the
  // rain never approaches would be drawn off the top of the plot and a reader
  // would see rain "reaching" a cutoff that is nowhere near it.
  const scaleMax = Math.max(peak, cutoff) * 1.18;
  const peakDay = days.find((d) => d.precipitationMm === peak);
  const level = levelName(lead.level);
  const crossed = peak >= cutoff;

  const PLOT = 132;

  return (
    <section
      aria-label={`The 14-day rainfall window ${lead.area} is being scored on`}
      style={{
        background: brand.surface,
        border: `1px solid ${brand.line}`,
        borderRadius: 4,
        padding: `${space(5)} ${space(5)} ${space(4)}`,
        marginBottom: space(6),
      }}
    >
      <div style={{ display: "flex", gap: space(6), flexWrap: "wrap", alignItems: "flex-start" }}>
        <div style={{ flex: "1 1 320px", minWidth: 0 }}>
          <div style={{ ...text.micro, color: brand.muted }}>
            {county.source === "fixture"
              ? "Committed demo scenario, not live weather"
              : `Live forecast, ingested from ${county.source}`}
          </div>
          <h2 style={{ ...text.display, color: brand.ink, margin: `${space(2)} 0 0` }}>
            {peak.toFixed(1)} mm of rain in one day
          </h2>
          <p className="prose" style={{ ...text.body, color: brand.muted, margin: `${space(2)} 0 0`, maxWidth: "54ch" }}>
            {crossed ? (
              <>
                {lead.area}’s heaviest day in this fourteen-day window is at or above the{" "}
                {cutoff} mm cutoff published for {diseaseName(lead.disease)}, so the county is
                scored <strong style={{ color: levelColor[level] }}>{level}</strong> and every
                guardian there with a child due a dose enters the alert path.
              </>
            ) : (
              <>
                {lead.area}’s heaviest day in this fourteen-day window stays below the {cutoff} mm
                cutoff published for {diseaseName(lead.disease)}, so the county is scored{" "}
                <strong style={{ color: levelColor[level] }}>{level}</strong> and no alert is
                raised for it.
              </>
            )}
          </p>
        </div>

        <div style={{ flex: "1 1 380px", minWidth: 280 }}>
          <div style={{ position: "relative", height: PLOT }}>
            {/* The published cutoff, drawn where it actually falls against the
                rain rather than as a decorative rule at a fixed height. */}
            <div
              style={{
                position: "absolute",
                left: 0,
                right: 0,
                bottom: `${(cutoff / scaleMax) * 100}%`,
                borderTop: `1.5px dashed ${brand.navy}`,
                pointerEvents: "none",
              }}
            >
              <span
                style={{
                  ...text.micro,
                  position: "absolute",
                  right: 0,
                  bottom: 3,
                  color: brand.navy,
                  background: brand.surface,
                  paddingLeft: space(2),
                }}
              >
                HIGH for {diseaseName(lead.disease)} at {cutoff} mm
              </span>
            </div>

            <div
              style={{
                display: "flex",
                alignItems: "flex-end",
                gap: 3,
                height: "100%",
                borderBottom: `1.5px solid ${brand.lineStrong}`,
              }}
            >
              {days.map((d) => {
                const over = d.precipitationMm >= cutoff;
                return (
                  <div
                    key={d.date}
                    title={`${d.date}: ${d.precipitationMm.toFixed(1)} mm`}
                    style={{
                      flex: 1,
                      height: `${Math.max((d.precipitationMm / scaleMax) * 100, d.precipitationMm > 0 ? 1.5 : 0)}%`,
                      background: over ? levelColor[level] : brand.navySoft,
                      opacity: over ? 1 : 0.42,
                      borderRadius: "1px 1px 0 0",
                      minHeight: d.precipitationMm > 0 ? 2 : 0,
                    }}
                  />
                );
              })}
            </div>
          </div>
          <div
            style={{
              display: "flex",
              justifyContent: "space-between",
              ...text.micro,
              color: brand.muted,
              marginTop: space(2),
            }}
          >
            <span>{days[0]?.date}</span>
            {peakDay !== undefined && <span>heaviest {peakDay.date}</span>}
            <span>{days[days.length - 1]?.date}</span>
          </div>
        </div>
      </div>
    </section>
  );
}
