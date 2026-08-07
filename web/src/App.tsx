// SPDX-License-Identifier: Apache-2.0
import type maplibregl from "maplibre-gl";
import { useEffect, useRef, useState } from "react";
import "maplibre-gl/dist/maplibre-gl.css";

import { publicClient } from "./api";
import type { GetCurrentRiskResponse } from "./gen/climateshield/v1/public_pb";
import { createMap, groupByCounty, levelColors, renderMarkers } from "./map";

type LoadState =
  | { kind: "loading" }
  | { kind: "ready"; data: GetCurrentRiskResponse }
  | { kind: "error"; message: string };

export default function App(): React.JSX.Element {
  const mapContainer = useRef<HTMLDivElement>(null);
  const mapRef = useRef<maplibregl.Map | null>(null);
  const [state, setState] = useState<LoadState>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    publicClient
      .getCurrentRisk({})
      .then((data) => {
        if (!cancelled) setState({ kind: "ready", data });
      })
      .catch((err: unknown) => {
        if (!cancelled) setState({ kind: "error", message: String(err) });
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // The map is created once, on mount, and outlives every data update.
  // Recreating it per render would rebuild the canvas while the container is
  // mid-layout, which leaves MapLibre stuck at its 400x300 fallback size.
  useEffect(() => {
    if (mapContainer.current === null) return;
    const map = createMap(mapContainer.current);
    mapRef.current = map;
    return () => {
      mapRef.current = null;
      map.remove();
    };
  }, []);

  useEffect(() => {
    const map = mapRef.current;
    if (map === null || state.kind !== "ready") return;
    const markers = renderMarkers(map, groupByCounty(state.data.scores));
    return () => markers.forEach((m) => m.remove());
  }, [state]);

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100vh", margin: 0 }}>
      <header
        style={{
          padding: "10px 16px",
          background: "#14213d",
          color: "#ffffff",
          display: "flex",
          alignItems: "baseline",
          gap: 12,
        }}
      >
        <strong style={{ fontSize: 18 }}>ClimateShield AI</strong>
        <span style={{ opacity: 0.85, fontSize: 13 }}>
          Climate-linked outbreak risk — public county aggregates
        </span>
        <span style={{ marginLeft: "auto", fontSize: 12, opacity: 0.85 }}>
          {state.kind === "loading" && "loading…"}
          {state.kind === "error" && `unavailable: ${state.message}`}
          {state.kind === "ready" &&
            `${state.data.scores.length} scores · generated ${
              state.data.generatedAt
                ? new Date(Number(state.data.generatedAt.seconds) * 1000).toLocaleString()
                : "n/a"
            }`}
        </span>
      </header>
      {/* position:relative + a definite flex basis so MapLibre can measure
          the element on the first frame. */}
      <div ref={mapContainer} style={{ flex: "1 1 auto", position: "relative", minHeight: 0 }} />
      <footer style={{ padding: "6px 16px", fontSize: 12, background: "#f5f5f5", display: "flex", gap: 16 }}>
        {(["HIGH", "MEDIUM", "LOW"] as const).map((lvl) => (
          <span key={lvl} style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
            <span
              style={{
                width: 12,
                height: 12,
                borderRadius: "50%",
                display: "inline-block",
                backgroundColor: levelColors[lvl],
              }}
            />
            {lvl}
          </span>
        ))}
        <span style={{ marginLeft: "auto", opacity: 0.7 }}>
          Aggregate data only — no personal information is served.
        </span>
      </footer>
    </div>
  );
}
