// SPDX-License-Identifier: Apache-2.0

import type maplibregl from "maplibre-gl";
import { useEffect, useRef, useState } from "react";
import "maplibre-gl/dist/maplibre-gl.css";

import { publicClient } from "../api";
import { Columns } from "../charts";
import { Select } from "../forms";
import { createMap, diseaseName, fitToCounties, groupByCounty, levelName, renderMarkers } from "../map";
import { Caveat, Failed, Loading, Pill, brand, levelColor, space, text } from "../ui";
import { useApi } from "../useApi";

const DISEASES = ["cholera", "malaria", "pneumonia", "meningitis"];

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
