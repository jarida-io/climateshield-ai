// SPDX-License-Identifier: Apache-2.0

// Hand-built SVG charts. No charting dependency: these forms are simple, and
// a library would add ~150KB to a page that must stay cheap to serve.
//
// Specs held constant across every chart here: bars capped at 24px with a 4px
// rounded data-end and a square baseline, 2px surface gaps between touching
// marks, 2px lines with round caps, markers at least 8px across carrying a 2px
// surface ring, hairline recessive gridlines, a legend whenever there are two
// or more series, labels only on the extremes, and a table view behind every
// chart so nothing is gated behind colour.

import { useEffect, useId, useRef, useState } from "react";

import { brand, space, text } from "./ui";

/**
 * Charts are drawn in real pixel coordinates, measured from the container.
 *
 * The tempting shortcut — a 0..100 viewBox with preserveAspectRatio="none" —
 * stretches the coordinate system horizontally, which turns every circle into
 * an ellipse and every rounded corner into a smear. Measuring costs one
 * ResizeObserver and keeps circles circular.
 */
function useWidth<T extends HTMLElement>(): [React.RefObject<T | null>, number] {
  const ref = useRef<T>(null);
  const [w, setW] = useState(600);
  useEffect(() => {
    const el = ref.current;
    if (el === null) return;
    const update = () => setW(Math.max(120, el.clientWidth));
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return [ref, w];
}

/**
 * Categorical slots, in fixed order — assigned by entity, never cycled and
 * never reassigned when a filter changes the series count. Validated for
 * colour-vision deficiency separation and lightness band as a four-slot set.
 */
export const series = ["#2A78D6", "#EB6834", "#1BAF7A", "#EDA100"] as const;

/** Stable colour per disease, so a filter never repaints the survivors. */
export const diseaseColor: Record<string, string> = {
  cholera: series[0],
  malaria: series[1],
  pneumonia: series[2],
  meningitis: series[3],
};

const AXIS = "#C9CEDA";
const GRID = "#E8EBF1";

function niceTicks(max: number, count = 4): number[] {
  if (max <= 0) return [0];
  const raw = max / count;
  const mag = Math.pow(10, Math.floor(Math.log10(raw)));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= raw) ?? mag * 10;
  const out: number[] = [];
  for (let v = 0; v <= max + step * 0.001; v += step) out.push(Number(v.toFixed(6)));
  return out;
}

export interface Column {
  label: string;
  value: number;
  color?: string | undefined;
  /** Shown in the tooltip instead of the bare value. */
  detail?: string | undefined;
}

/**
 * Column chart for magnitude across a small set of labelled slots.
 * Hover gives an exact readout; the extreme is directly labelled.
 */
export function Columns({
  data,
  height = 180,
  unit = "",
  color = brand.purple,
  labelEvery = 1,
}: {
  data: Column[];
  height?: number;
  unit?: string;
  color?: string;
  labelEvery?: number;
}) {
  const [hover, setHover] = useState<number | null>(null);
  const [wrap, W] = useWidth<HTMLDivElement>();
  const titleId = useId();

  const max = Math.max(...data.map((d) => d.value), 0);
  const ticks = niceTicks(max);
  const top = ticks[ticks.length - 1] ?? 1;
  const plotH = height - 26;
  const slot = data.length === 0 ? W : W / data.length;
  // Bars are capped at 24px and never fill their slot: the leftover is the
  // 2px-plus surface gap that separates neighbours.
  const barW = Math.min(24, Math.max(3, slot - 6));
  const peak = data.reduce((best, d, i) => (d.value > (data[best]?.value ?? -Infinity) ? i : best), 0);

  return (
    <figure style={{ margin: 0 }}>
      <div ref={wrap} style={{ position: "relative" }}>
        {data.length === 0 ? (
          <Empty />
        ) : (
        <svg
          viewBox={`0 0 ${W} ${height}`}
          role="img"
          aria-labelledby={titleId}
          style={{ width: "100%", height, display: "block", overflow: "visible" }}
        >
          <title id={titleId}>Column chart, {data.length} values, peak {max.toFixed(1)}{unit}</title>

          {ticks.map((t) => {
            const y = plotH - (t / top) * plotH;
            return (
              <line
                key={t}
                x1="0"
                x2={W}
                y1={y}
                y2={y}
                stroke={t === 0 ? AXIS : GRID}
                strokeWidth="1"
              />
            );
          })}

          {data.map((d, i) => {
            const h = top === 0 ? 0 : (d.value / top) * plotH;
            // 2px surface gap between neighbours comes from the slot padding.
            const x = i * slot + (slot - barW) / 2;
            const isHot = hover === i;
            return (
              <g key={d.label}>
                <rect
                  x={x}
                  y={plotH - h}
                  width={barW}
                  height={Math.max(h, 0.5)}
                  fill={d.color ?? color}
                  opacity={hover === null || isHot ? 1 : 0.45}
                  rx="4"
                  ry="4"
                />
                {/* Square the baseline: the 4px radius belongs to the data-end
                    only, so the bar still sits flat on the axis. */}
                <rect
                  x={x}
                  y={plotH - Math.min(h, 4)}
                  width={barW}
                  height={Math.min(h, 4)}
                  fill={d.color ?? color}
                  opacity={hover === null || isHot ? 1 : 0.45}
                />
                {/* Hit target is wider than the mark. */}
                <rect
                  x={i * slot}
                  y={0}
                  width={slot}
                  height={plotH}
                  fill="transparent"
                  onMouseEnter={() => setHover(i)}
                  onMouseLeave={() => setHover(null)}
                />
              </g>
            );
          })}
        </svg>
        )}

        {/* Tick values sit in HTML so they inherit page typography. */}
        <div
          style={{
            position: "absolute",
            inset: 0,
            pointerEvents: "none",
            ...text.small,
            color: brand.muted,
          }}
        >
          <span style={{ position: "absolute", left: 0, top: -6 }}>
            {top.toLocaleString()}
            {unit}
          </span>
        </div>
      </div>

      <div style={{ display: "flex", marginTop: space(1) }}>
        {data.map((d, i) => (
          <div
            key={d.label}
            style={{
              flex: 1,
              textAlign: "center",
              ...text.small,
              color: hover === i || i === peak ? brand.ink : brand.muted,
              fontWeight: hover === i || i === peak ? 700 : 400,
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
          >
            {i % labelEvery === 0 || hover === i ? d.label : ""}
          </div>
        ))}
      </div>

      <figcaption style={{ ...text.small, color: brand.muted, marginTop: space(2), minHeight: 18 }}>
        {hover === null
          ? `Peak: ${data[peak]?.label} at ${data[peak]?.value.toLocaleString()}${unit}`
          : (data[hover]?.detail ??
            `${data[hover]?.label}: ${data[hover]?.value.toLocaleString()}${unit}`)}
      </figcaption>
    </figure>
  );
}

export interface LineSeries {
  name: string;
  color: string;
  points: { label: string; value: number }[];
}

/** Multi-series line chart for change over one shared axis. */
export function Lines({
  data,
  height = 190,
  unit = "",
}: {
  data: LineSeries[];
  height?: number;
  unit?: string;
}) {
  const [hover, setHover] = useState<number | null>(null);
  const [wrap, W] = useWidth<HTMLDivElement>();
  const titleId = useId();

  const n = data[0]?.points.length ?? 0;
  const all = data.flatMap((s) => s.points.map((p) => p.value));
  if (n === 0 || all.length === 0) return <Empty />;
  const lo = Math.min(...all);
  const hi = Math.max(...all);
  const pad = (hi - lo) * 0.15 || 1;
  const min = lo - pad;
  const max = hi + pad;
  const plotH = height - 24;
  // Inset so an end marker at either extreme is never clipped by the edge.
  const PAD = 8;
  const x = (i: number) => (n === 1 ? W / 2 : PAD + (i / (n - 1)) * (W - PAD * 2));
  const y = (v: number) => plotH - ((v - min) / (max - min)) * plotH;

  return (
    <figure style={{ margin: 0 }}>
      <div ref={wrap} style={{ position: "relative" }}>
        <svg
          viewBox={`0 0 ${W} ${height}`}
          role="img"
          aria-labelledby={titleId}
          style={{ width: "100%", height, display: "block", overflow: "visible" }}
        >
          <title id={titleId}>
            Line chart of {data.map((s) => s.name).join(" and ")} over {n} points
          </title>

          {[0, 0.5, 1].map((f) => (
            <line
              key={f}
              x1="0"
              x2={W}
              y1={plotH * f}
              y2={plotH * f}
              stroke={GRID}
              strokeWidth="1"
            />
          ))}

          {hover !== null && (
            <line
              x1={x(hover)}
              x2={x(hover)}
              y1="0"
              y2={plotH}
              stroke={AXIS}
              strokeWidth="1"
            />
          )}

          {data.map((s) => (
            <polyline
              key={s.name}
              points={s.points.map((p, i) => `${x(i)},${y(p.value)}`).join(" ")}
              fill="none"
              stroke={s.color}
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          ))}

          {/* End markers: 2px surface ring keeps them legible on crossings. */}
          {data.map((s) => {
            const last = s.points[s.points.length - 1];
            if (last === undefined) return null;
            return (
              <circle
                key={s.name}
                cx={x(n - 1)}
                cy={y(last.value)}
                r="4"
                fill={s.color}
                stroke={brand.surface}
                strokeWidth="2"
              />
            );
          })}

          {Array.from({ length: n }, (_, i) => (
            <rect
              key={i}
              x={x(i) - W / Math.max(n - 1, 1) / 2}
              y={0}
              width={W / Math.max(n - 1, 1)}
              height={plotH}
              fill="transparent"
              onMouseEnter={() => setHover(i)}
              onMouseLeave={() => setHover(null)}
            />
          ))}
        </svg>

        <div
          style={{
            position: "absolute",
            inset: 0,
            pointerEvents: "none",
            ...text.small,
            color: brand.muted,
          }}
        >
          <span style={{ position: "absolute", left: 0, top: -6 }}>
            {hi.toFixed(1)}
            {unit}
          </span>
          <span style={{ position: "absolute", left: 0, top: plotH - 8 }}>
            {lo.toFixed(1)}
            {unit}
          </span>
        </div>
      </div>

      <Legend items={data.map((s) => ({ name: s.name, color: s.color }))} />

      <figcaption style={{ ...text.small, color: brand.muted, marginTop: space(1), minHeight: 18 }}>
        {hover === null
          ? `${n} points · range ${lo.toFixed(1)}–${hi.toFixed(1)}${unit}`
          : `${data[0]?.points[hover]?.label}: ` +
            data
              .map((s) => `${s.name} ${s.points[hover]?.value.toFixed(1)}${unit}`)
              .join(" · ")}
      </figcaption>
    </figure>
  );
}

/**
 * Where one observed value falls inside a reference distribution. This is the
 * chart that makes the climatology model legible: the ladder is a decade of
 * history, the marker is the forecast window being scored.
 */
export function DistributionStrip({
  quantiles,
  value,
  unit,
  lowerTailIsHazard = false,
}: {
  quantiles: number[];
  value: number;
  unit: string;
  lowerTailIsHazard?: boolean;
}) {
  const titleId = useId();
  const [wrap, W] = useWidth<HTMLDivElement>();
  if (quantiles.length < 2) return <Empty />;

  const lo = quantiles[0] ?? 0;
  const hi = quantiles[quantiles.length - 1] ?? 1;
  const span = hi - lo || 1;
  const pos = Math.max(0, Math.min(1, (value - lo) / span));
  const inTail = lowerTailIsHazard ? pos <= 0.1 : pos >= 0.9;
  // Inset so a value at either extreme keeps its whole marker on screen.
  const PAD = 8;
  const markX = PAD + pos * (W - PAD * 2);
  const barW = W - PAD * 2;

  return (
    <figure ref={wrap} style={{ margin: 0 }}>
      <svg
        viewBox={`0 0 ${W} 34`}
        role="img"
        aria-labelledby={titleId}
        style={{ width: "100%", height: 34, display: "block", overflow: "visible" }}
      >
        <title id={titleId}>
          Observed {value}
          {unit} against the reference range {lo.toFixed(1)}–{hi.toFixed(1)}
          {unit}
        </title>

        {/* The reference range as a wash, with the hazardous tail picked out. */}
        <rect x={PAD} y="12" width={barW} height="8" fill={brand.purple} opacity="0.12" rx="4" />
        <rect
          x={lowerTailIsHazard ? PAD : PAD + barW * 0.9}
          y="12"
          width={barW * 0.1}
          height="8"
          fill={brand.high}
          opacity="0.22"
        />

        {quantiles.map((_, i) => (
          <line
            key={i}
            x1={PAD + (i / (quantiles.length - 1)) * barW}
            x2={PAD + (i / (quantiles.length - 1)) * barW}
            y1="12"
            y2="20"
            stroke={brand.surface}
            strokeWidth="1"
          />
        ))}

        <line
          x1={markX}
          x2={markX}
          y1="8"
          y2="26"
          stroke={inTail ? brand.high : brand.ink}
          strokeWidth="2"
        />
        <circle
          cx={markX}
          cy="8"
          r="4"
          fill={inTail ? brand.high : brand.ink}
          stroke={brand.surface}
          strokeWidth="2"
        />
      </svg>

      <div style={{ display: "flex", justifyContent: "space-between", ...text.small, color: brand.muted }}>
        <span>
          {lo.toFixed(1)}
          {unit} <span style={{ opacity: 0.7 }}>(lowest on record)</span>
        </span>
        <span style={{ color: inTail ? brand.high : brand.ink, fontWeight: 700 }}>
          now {value.toFixed(1)}
          {unit}
        </span>
        <span>
          {hi.toFixed(1)}
          {unit} <span style={{ opacity: 0.7 }}>(highest)</span>
        </span>
      </div>
    </figure>
  );
}

export function Legend({ items }: { items: { name: string; color: string }[] }) {
  if (items.length < 2) return null;
  return (
    <div style={{ display: "flex", gap: space(4), flexWrap: "wrap", marginTop: space(2) }}>
      {items.map((i) => (
        <span
          key={i.name}
          style={{ ...text.small, color: brand.muted, display: "inline-flex", alignItems: "center", gap: space(2) }}
        >
          <span
            style={{ width: 10, height: 10, borderRadius: 2, background: i.color, display: "inline-block" }}
          />
          {i.name}
        </span>
      ))}
    </div>
  );
}

/**
 * Every chart ships a table view. Three of the categorical slots sit below 3:1
 * against the page surface, so the numbers must be reachable without relying
 * on the colour at all.
 */
export function TableView({ head, rows }: { head: string[]; rows: (string | number)[][] }) {
  const [open, setOpen] = useState(false);
  return (
    <div style={{ marginTop: space(2) }}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        style={{
          ...text.small,
          background: "none",
          border: "none",
          padding: 0,
          color: brand.purple,
          cursor: "pointer",
          textDecoration: "underline",
        }}
      >
        {open ? "Hide" : "Show"} data table
      </button>
      {open && (
        <div style={{ overflowX: "auto", marginTop: space(2) }}>
          <table style={{ borderCollapse: "collapse", ...text.small }}>
            <thead>
              <tr>
                {head.map((h) => (
                  <th
                    key={h}
                    style={{
                      textAlign: "left",
                      padding: `${space(1)} ${space(3)}`,
                      borderBottom: `1px solid ${brand.line}`,
                      color: brand.muted,
                    }}
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={i}>
                  {r.map((c, j) => (
                    <td
                      key={j}
                      style={{
                        padding: `${space(1)} ${space(3)}`,
                        borderBottom: `1px solid ${brand.line}`,
                        color: brand.ink,
                      }}
                    >
                      {c}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function Empty() {
  return (
    <p style={{ ...text.small, color: brand.muted, margin: `${space(4)} 0` }}>
      No data to plot yet.
    </p>
  );
}
