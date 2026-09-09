// SPDX-License-Identifier: Apache-2.0

import type maplibregl from "maplibre-gl";
import { useEffect, useRef, useState } from "react";
import "maplibre-gl/dist/maplibre-gl.css";

import { publicClient } from "../api";
import { Select } from "../forms";
import type { RiskScore } from "../gen/climateshield/v1/public_pb";
import { createMap, diseaseName, fitToCounties, groupByCounty, levelName, renderMarkers } from "../map";
import { Disclosure, Failed, Loading, Pill, brand, levelColor, space, text } from "../ui";
import { useApi } from "../useApi";

const DISEASES = ["cholera", "malaria", "pneumonia", "meningitis"];

/**
 * Plain words for the two drivers the scorers use, matching the wording the Go
 * explainers already produce. Only a label is chosen here — the number and the
 * driver name both come from the score row.
 */
const DRIVER_LABEL: Record<string, { name: string; unit: string }> = {
  peak_rainfall_mm_14d: { name: "peak 14-day rainfall", unit: " mm" },
  mean_max_temp_c_14d: { name: "mean 14-day maximum temperature", unit: " °C" },
};

function driverPhrase(driver: string, value: number): string {
  const d = DRIVER_LABEL[driver];
  return d === undefined ? `${driver} = ${String(value)}` : `${d.name} ${value.toFixed(1)}${d.unit}`;
}

/** The worst-scoring row for a county — the one that colours its marker. */
function worstScore(scores: RiskScore[]): RiskScore | undefined {
  let best: RiskScore | undefined;
  for (const s of scores) {
    if (best === undefined || s.level > best.level) best = s;
  }
  return best;
}

/** Proves: current risk, geographically, at a glance. */
export function MapView() {
  const container = useRef<HTMLDivElement>(null);
  const mapRef = useRef<maplibregl.Map | null>(null);
  const [disease, setDisease] = useState("");
  const [basemap, setBasemap] = useState<"loading" | "ready" | "unavailable">("loading");
  const risk = useApi(() => publicClient.getCurrentRisk({}), []);

  const scores =
    risk.kind === "ready"
      ? risk.data.scores.filter((s) => disease === "" || diseaseName(s.disease) === disease)
      : [];

  // Created once so the canvas is never rebuilt mid-layout.
  useEffect(() => {
    if (container.current === null) return;
    const map = createMap(container.current);
    mapRef.current = map;

    // Attached at creation, because a listener registered in a later effect
    // can miss an event the map has already fired.
    //
    // The basemap comes from MapLibre's public demo tile server. On a
    // restricted or offline network it can stall with no error event at all,
    // so a timeout is the only way to distinguish "slow" from "never". Say
    // which one it is rather than leaving a blank rectangle: the county
    // markers are the data and remain accurate either way.
    const ready = () => setBasemap("ready");
    map.once("idle", ready);
    const giveUp = setTimeout(
      () => setBasemap((s) => (s === "ready" ? s : "unavailable")),
      10000,
    );

    return () => {
      clearTimeout(giveUp);
      map.off("idle", ready);
      mapRef.current = null;
      map.remove();
    };
  }, []);

  useEffect(() => {
    const map = mapRef.current;
    if (map === null || risk.kind !== "ready") return;
    const groups = groupByCounty(scores);
    const markers = renderMarkers(map, groups);
    fitToCounties(map, groups);
    return () => markers.forEach((m) => m.remove());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [risk, disease]);

  const counties = groupByCounty(scores);
  // Worst county first, so the eye lands where the attention is needed.
  const ranked = [...counties].sort(
    (a, b) => (worstScore(b.scores)?.level ?? 0) - (worstScore(a.scores)?.level ?? 0),
  );

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", minHeight: 0 }}>
      <div style={{ padding: `${space(5)} ${space(6)} 0`, maxWidth: 1180, margin: "0 auto", width: "100%" }}>
        <h1 style={{ ...text.h1, margin: 0, color: brand.ink }}>Where risk is elevated today</h1>
        <p style={{ ...text.body, color: brand.muted, margin: `${space(2)} 0 ${space(4)}`, lineHeight: 1.6 }}>
          Current risk by county and disease, with the weather figure that produced each level.
          Pick a disease to recolour the markers, or read the counties underneath.
        </p>
        <form
          onSubmit={(e) => e.preventDefault()}
          style={{ display: "flex", gap: space(4), flexWrap: "wrap", marginBottom: space(4) }}
        >
          <Select
            label="Disease"
            value={disease}
            onChange={setDisease}
            options={[
              { value: "", label: "All diseases (worst per county)" },
              ...DISEASES.map((d) => ({ value: d, label: d })),
            ]}
            hint="markers recolour to the worst level among the selection"
          />
        </form>
        <Disclosure>
          Everything here is a county-level aggregate, computed from the forecast window and the
          published thresholds. Markers sit on county centroids — they are not the location of any
          person, and nothing on this map is derived from an identifiable child.
        </Disclosure>
      </div>

      <div style={{ flex: "0 0 auto", position: "relative", height: 420, minHeight: 300 }}>
        <div ref={container} style={{ position: "absolute", inset: 0 }} />
        {basemap !== "ready" && (
          <div
            style={{
              position: "absolute", top: space(3), left: space(3),
              background: brand.surface, border: `1px solid ${brand.line}`,
              borderRadius: 999, padding: `${space(1)} ${space(3)}`,
              ...text.small,
              color: basemap === "unavailable" ? brand.warn : brand.muted,
              pointerEvents: "none",
              boxShadow: "0 1px 3px rgba(0,0,0,0.08)",
              maxWidth: "min(90%, 520px)",
              lineHeight: 1.5,
            }}
          >
            {basemap === "loading"
              ? "Loading basemap…"
              : "Basemap unavailable — county markers and risk levels below are still accurate"}
          </div>
        )}
      </div>

      <div
        style={{
          borderTop: `1px solid ${brand.line}`,
          background: brand.surface,
          padding: `${space(3)} ${space(6)}`,
          display: "flex",
          gap: space(5),
          flexWrap: "wrap",
          alignItems: "center",
        }}
      >
        {(["HIGH", "MEDIUM", "LOW"] as const).map((lvl) => (
          <span key={lvl} style={{ ...text.small, display: "inline-flex", alignItems: "center", gap: space(2) }}>
            <span
              style={{
                width: 12, height: 12, borderRadius: "50%",
                display: "inline-block", background: levelColor[lvl],
              }}
            />
            {lvl}
          </span>
        ))}
        <span style={{ ...text.small, color: brand.muted, marginLeft: "auto" }}>
          {risk.kind === "ready" && `${risk.data.scores.length} scores across ${counties.length} counties`}
        </span>
      </div>

      {risk.kind === "loading" && (
        <div style={{ padding: space(6) }}>
          <Loading what="current risk" />
        </div>
      )}
      {risk.kind === "error" && (
        <div style={{ padding: space(6) }}>
          <Failed what="current risk" error={risk.message} />
        </div>
      )}
      {risk.kind === "ready" && (
        <div style={{ padding: `${space(4)} ${space(6)} ${space(10)}`, maxWidth: 1180, margin: "0 auto", width: "100%" }}>
          <div style={{ ...text.h2, color: brand.ink, marginBottom: space(3) }}>
            Every county, worst level first — and why
          </div>
          <div style={{ display: "flex", gap: space(3), flexWrap: "wrap" }}>
            {ranked.map((c) => {
              const worst = worstScore(c.scores);
              return (
                <div
                  key={c.area}
                  style={{
                    border: `1px solid ${brand.line}`, borderRadius: 10,
                    padding: space(4), flex: "1 1 280px", minWidth: 0, background: brand.surface,
                  }}
                >
                  <div style={{ display: "flex", alignItems: "center", gap: space(2), marginBottom: space(2) }}>
                    <span style={{ ...text.h2, color: brand.ink }}>{c.area}</span>
                    {worst !== undefined && (
                      <span style={{ marginLeft: "auto" }}>
                        <Pill level={levelName(worst.level)} />
                      </span>
                    )}
                  </div>
                  {/* The explanation is written by the predictor that produced
                      the row, so this card can never invent a reason the
                      scorer did not give. */}
                  {worst !== undefined && (
                    <p style={{ ...text.small, color: brand.ink, margin: `0 0 ${space(3)}`, lineHeight: 1.6 }}>
                      {worst.explanation === ""
                        ? `${levelName(worst.level)} for ${diseaseName(worst.disease)} — ${driverPhrase(worst.driver, worst.driverValue)} in the scored window.`
                        : `${diseaseName(worst.disease)}: ${worst.explanation}`}
                    </p>
                  )}
                  {c.scores.map((s, i) => (
                    <div
                      key={`${s.disease}-${i}`}
                      style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: space(3), marginTop: space(1) }}
                    >
                      <span style={{ ...text.small, color: brand.muted }}>{diseaseName(s.disease)}</span>
                      <Pill level={levelName(s.level)} />
                    </div>
                  ))}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
