// SPDX-License-Identifier: Apache-2.0

import type maplibregl from "maplibre-gl";
import { useEffect, useRef, useState } from "react";
import "maplibre-gl/dist/maplibre-gl.css";

import { publicClient } from "../api";
import { Columns } from "../charts";
import { Select } from "../forms";
import { createMap, diseaseName, groupByCounty, levelName, renderMarkers } from "../map";
import { Caveat, Failed, Loading, Pill, brand, levelColor, space, text } from "../ui";
import { useApi } from "../useApi";

const DISEASES = ["cholera", "malaria", "pneumonia", "meningitis"];

/** Proves: current risk, geographically, at a glance. */
export function MapView() {
  const container = useRef<HTMLDivElement>(null);
  const mapRef = useRef<maplibregl.Map | null>(null);
  const [disease, setDisease] = useState("");
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
    return () => {
      mapRef.current = null;
      map.remove();
    };
  }, []);

  useEffect(() => {
    const map = mapRef.current;
    if (map === null || risk.kind !== "ready") return;
    const markers = renderMarkers(map, groupByCounty(scores));
    return () => markers.forEach((m) => m.remove());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [risk, disease]);

  const counties = groupByCounty(scores);
  const byLevel = (["HIGH", "MEDIUM", "LOW"] as const).map((lvl) => ({
    label: lvl,
    value: scores.filter((s) => levelName(s.level) === lvl).length,
    color: levelColor[lvl],
    detail: `${scores.filter((s) => levelName(s.level) === lvl).length} county-disease pairs at ${lvl}`,
  }));

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", minHeight: 0 }}>
      <div style={{ padding: `${space(4)} ${space(6)} 0`, maxWidth: 1180, margin: "0 auto", width: "100%" }}>
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
        <Caveat>
          County-level aggregates only. Markers are county centroids, not the location of any
          person, and nothing on this map is derived from an identifiable child.
        </Caveat>
      </div>

      <div style={{ flex: "1 1 auto", position: "relative", minHeight: 320 }}>
        <div ref={container} style={{ position: "absolute", inset: 0 }} />
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
        <div style={{ padding: `${space(4)} ${space(6)}`, maxWidth: 1180, margin: "0 auto", width: "100%" }}>
          <div style={{ marginBottom: space(5) }}>
            <div style={{ ...text.h2, color: brand.ink, marginBottom: space(2) }}>
              Risk distribution
            </div>
            <Columns data={byLevel} height={140} />
          </div>
          <div style={{ display: "flex", gap: space(3), flexWrap: "wrap" }}>
            {counties.map((c) => (
              <div
                key={c.area}
                style={{
                  border: `1px solid ${brand.line}`, borderRadius: 10,
                  padding: space(4), flex: "1 1 200px", background: brand.surface,
                }}
              >
                <div style={{ ...text.h2, color: brand.ink, marginBottom: space(2) }}>{c.area}</div>
                {c.scores.map((s, i) => (
                  <div
                    key={`${s.disease}-${i}`}
                    style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: space(1) }}
                  >
                    <span style={{ ...text.small, color: brand.muted }}>{diseaseName(s.disease)}</span>
                    <Pill level={levelName(s.level)} />
                  </div>
                ))}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
