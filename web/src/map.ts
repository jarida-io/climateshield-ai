// SPDX-License-Identifier: Apache-2.0

// County risk markers on a MapLibre map. MapLibre GL JS is the mandated map
// stack (open source; Mapbox GL v2+ is a forbidden dependency).
import maplibregl from "maplibre-gl";

import { Disease, RiskLevel } from "./gen/climateshield/v1/common_pb";
import type { RiskScore } from "./gen/climateshield/v1/public_pb";

// Imported from the design system so the map and the charts can never drift
// apart on what HIGH looks like.
import { levelColor } from "./ui";

export const levelColors = levelColor;

export function levelName(l: RiskLevel): "HIGH" | "MEDIUM" | "LOW" | "NONE" {
  switch (l) {
    case RiskLevel.HIGH:
      return "HIGH";
    case RiskLevel.MEDIUM:
      return "MEDIUM";
    case RiskLevel.LOW:
      return "LOW";
    default:
      return "NONE";
  }
}

export function diseaseName(d: Disease): string {
  switch (d) {
    case Disease.CHOLERA:
      return "cholera";
    case Disease.MALARIA:
      return "malaria";
    case Disease.PNEUMONIA:
      return "pneumonia";
    case Disease.MENINGITIS:
      return "meningitis";
    default:
      return "unknown";
  }
}

/** Worst level wins the marker color for a county. */
export function worstLevel(scores: RiskScore[]): RiskLevel {
  let worst = RiskLevel.UNSPECIFIED;
  for (const s of scores) {
    if (s.level > worst) worst = s.level;
  }
  return worst;
}

export interface CountyGroup {
  area: string;
  latitude: number;
  longitude: number;
  scores: RiskScore[];
}

/** Group flat score rows by county. */
export function groupByCounty(scores: RiskScore[]): CountyGroup[] {
  const byArea = new Map<string, CountyGroup>();
  for (const s of scores) {
    const existing = byArea.get(s.area);
    if (existing) {
      existing.scores.push(s);
    } else {
      byArea.set(s.area, {
        area: s.area,
        latitude: s.latitude,
        longitude: s.longitude,
        scores: [s],
      });
    }
  }
  return [...byArea.values()];
}

/** Create the base map centered on Kenya. */
export function createMap(container: HTMLElement): maplibregl.Map {
  const map = new maplibregl.Map({
    container,
    // Openly-licensed demo style, no API key. If tiles are unreachable the
    // markers still render on a plain background.
    style: "https://demotiles.maplibre.org/style.json",
    center: [37.0, -0.5],
    zoom: 5.6,
  });
  // The map is created inside a flex child, so its size is not final on the
  // first frame; without this the canvas keeps its initial (tiny) dimensions
  // and markers land outside the visible area.
  map.once("load", () => map.resize());
  const observer = new ResizeObserver(() => map.resize());
  observer.observe(container);
  map.once("remove", () => observer.disconnect());
  return map;
}

/** Render one marker per county, colored by its worst risk level. */
export function renderMarkers(map: maplibregl.Map, groups: CountyGroup[]): maplibregl.Marker[] {
  return groups.map((g) => {
    const level = levelName(worstLevel(g.scores));
    const el = document.createElement("div");
    el.style.width = "22px";
    el.style.height = "22px";
    el.style.borderRadius = "50%";
    el.style.border = "2px solid #ffffff";
    el.style.boxShadow = "0 1px 4px rgba(0,0,0,0.4)";
    el.style.backgroundColor = levelColors[level] ?? levelColors["NONE"] ?? "#8d99ae";
    el.title = `${g.area}: ${level}`;

    const rows = g.scores
      .map((s) => `<tr><td>${diseaseName(s.disease)}</td><td><b>${levelName(s.level)}</b></td></tr>`)
      .join("");
    const popup = new maplibregl.Popup({ offset: 14 }).setHTML(
      `<strong>${g.area}</strong><table>${rows}</table>`,
    );

    return new maplibregl.Marker({ element: el })
      .setLngLat([g.longitude, g.latitude])
      .setPopup(popup)
      .addTo(map);
  });
}
