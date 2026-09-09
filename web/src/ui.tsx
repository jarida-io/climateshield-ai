// SPDX-License-Identifier: Apache-2.0

// Shared design system. Kept deliberately small: plain objects and a handful
// of components, no CSS framework, so the whole visual language is auditable
// in one file.

import type { CSSProperties, ReactNode } from "react";

export const brand = {
  purple: "#7C4DFF",
  purpleDim: "#EDE7FF",
  navy: "#14213D",
  navySoft: "#26315A",
  ink: "#1B1F2A",
  muted: "#5C6478",
  line: "#E3E6ED",
  surface: "#FFFFFF",
  canvas: "#F7F8FB",
  // Status palette, darkened from the original so white label text clears
  // 4.5:1 on every tier. The previous amber sat at 2.07:1 — unreadable, and
  // this is a public health tool.
  high: "#C1121F",
  medium: "#B45309",
  low: "#12715F",
  warn: "#8A6100",
  warnBg: "#FFF6E0",
  // Tinted chip grounds. Each is paired below with ink dark enough to clear
  // 4.5:1 on it, because a chip is often the only word carrying a caveat.
  lowBg: "#E3F1EC",
  highBg: "#FCE8EA",
  purpleInk: "#4B2AC2",
} as const;

export const space = (n: number) => `${n * 4}px`;

export const text = {
  h1: { fontSize: 22, fontWeight: 700, letterSpacing: "0.01em" },
  h2: { fontSize: 16, fontWeight: 700 },
  body: { fontSize: 14, fontWeight: 400 },
  small: { fontSize: 12.5, fontWeight: 400 },
  mono: { fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace", fontSize: 12.5 },
} satisfies Record<string, CSSProperties>;

export const levelColor: Record<string, string> = {
  HIGH: brand.high,
  MEDIUM: brand.medium,
  LOW: brand.low,
  NONE: "#5C6478",
};

/** Risk order, worst first — used to sort and to lay out legends. */
export const LEVELS = ["HIGH", "MEDIUM", "LOW"] as const;

/** Page shell: a title, a one-line purpose, and the body. */
export function Page({
  title,
  lede,
  children,
}: {
  title: string;
  lede: ReactNode;
  children: ReactNode;
}) {
  return (
    <div style={{ padding: `${space(6)} ${space(6)} ${space(10)}`, maxWidth: 1180, margin: "0 auto" }}>
      <h1 style={{ ...text.h1, margin: 0, color: brand.ink }}>{title}</h1>
      <p style={{ ...text.body, color: brand.muted, margin: `${space(2)} 0 ${space(5)}`, lineHeight: 1.6 }}>
        {lede}
      </p>
      {children}
    </div>
  );
}

/**
 * Disclosure carries the standing boundary of a view: what it shows, and what
 * it does not prove. Every sentence that used to sit in an amber box at the
 * top of the page lives here instead — none were dropped, they were reworded
 * to lead with the affirmative fact and end with the limit.
 *
 * It is deliberately NOT a warning colour. A permanent condition of the design
 * ("these are county aggregates") is not an alarm; painting it like one taught
 * readers to skip the box, which is the opposite of what it is for. Amber is
 * reserved for Caveat: something true right now that may not be true later.
 */
export function Disclosure({
  caption = "What this view shows — and does not",
  children,
}: {
  caption?: string;
  children: ReactNode;
}) {
  return (
    <section
      role="note"
      style={{
        background: brand.canvas,
        border: `1px solid ${brand.line}`,
        borderRadius: 10,
        padding: `${space(3)} ${space(4)}`,
        margin: `0 0 ${space(5)}`,
        ...text.small,
        color: brand.ink,
        lineHeight: 1.6,
      }}
    >
      <div
        style={{
          fontSize: 11,
          fontWeight: 700,
          letterSpacing: "0.06em",
          textTransform: "uppercase",
          color: brand.muted,
          marginBottom: space(2),
        }}
      >
        {caption}
      </div>
      {children}
    </section>
  );
}

export type Tone = "neutral" | "good" | "warn" | "bad" | "info";

const toneStyle: Record<Tone, { bg: string; fg: string; border: string }> = {
  neutral: { bg: brand.canvas, fg: brand.muted, border: brand.line },
  good: { bg: brand.lowBg, fg: brand.low, border: "#BEDED4" },
  warn: { bg: brand.warnBg, fg: brand.warn, border: "#E8D08A" },
  bad: { bg: brand.highBg, fg: brand.high, border: "#F0C2C7" },
  info: { bg: brand.purpleDim, fg: brand.purpleInk, border: "#D7CCFF" },
};

/** A short standing fact — a channel, a source, a mode. Never decorative. */
export function Chip({
  children,
  tone = "neutral",
  title,
}: {
  children: ReactNode;
  tone?: Tone;
  title?: string;
}) {
  const t = toneStyle[tone];
  return (
    <span
      {...(title === undefined ? {} : { title })}
      style={{
        ...text.small,
        fontWeight: 700,
        color: t.fg,
        background: t.bg,
        border: `1px solid ${t.border}`,
        borderRadius: 999,
        padding: "2px 10px",
        display: "inline-block",
        lineHeight: 1.6,
      }}
    >
      {children}
    </span>
  );
}

/**
 * Caveat is for a RUNTIME condition — something true of this deployment at
 * this moment and possibly not true an hour from now: the basemap did not
 * load, the API is serving a cached response, jobs are failing, a channel that
 * really sends is active. Standing statements about what a view does and does
 * not prove belong in Disclosure, which does not shout.
 */
export function Caveat({ children }: { children: ReactNode }) {
  return (
    <div
      role="note"
      style={{
        ...text.small,
        background: brand.warnBg,
        border: `1px solid #E8D08A`,
        color: brand.warn,
        borderRadius: 8,
        padding: `${space(3)} ${space(4)}`,
        margin: `0 0 ${space(5)}`,
        lineHeight: 1.5,
      }}
    >
      {children}
    </div>
  );
}

export function Card({ title, children }: { title?: string; children: ReactNode }) {
  return (
    <section
      style={{
        background: brand.surface,
        border: `1px solid ${brand.line}`,
        borderRadius: 10,
        padding: space(5),
        marginBottom: space(5),
      }}
    >
      {title !== undefined && (
        <h2 style={{ ...text.h2, margin: `0 0 ${space(4)}`, color: brand.ink }}>{title}</h2>
      )}
      {children}
    </section>
  );
}

export function StatTile({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div
      style={{
        border: `1px solid ${brand.line}`,
        borderRadius: 10,
        padding: space(4),
        minWidth: 150,
        flex: "1 1 150px",
        background: brand.surface,
      }}
    >
      <div style={{ ...text.small, color: brand.muted }}>{label}</div>
      <div style={{ fontSize: 24, fontWeight: 700, color: brand.ink, marginTop: space(1) }}>
        {value}
      </div>
      {hint !== undefined && (
        <div style={{ ...text.small, color: brand.muted, marginTop: space(1) }}>{hint}</div>
      )}
    </div>
  );
}

export function TileRow({ children }: { children: ReactNode }) {
  return (
    <div style={{ display: "flex", gap: space(3), flexWrap: "wrap", marginBottom: space(5) }}>
      {children}
    </div>
  );
}

/**
 * Responsive grid whose columns are decided by the available width, not by a
 * breakpoint list: one column on a phone, two on a tablet, three on a laptop,
 * with no horizontal scrolling at any width in between.
 */
export function Grid({ min = 260, children }: { min?: number; children: ReactNode }) {
  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: `repeat(auto-fit, minmax(min(100%, ${min}px), 1fr))`,
        gap: space(3),
        marginBottom: space(5),
      }}
    >
      {children}
    </div>
  );
}

export function Pill({ level }: { level: string }) {
  return (
    <span
      style={{
        ...text.small,
        fontWeight: 700,
        color: "#fff",
        background: levelColor[level] ?? levelColor["NONE"],
        borderRadius: 999,
        padding: "2px 10px",
        display: "inline-block",
      }}
    >
      {level}
    </span>
  );
}

/** Table wrapper: wide content scrolls inside itself, never the page. */
export function Table({ head, children }: { head: string[]; children: ReactNode }) {
  return (
    <div style={{ overflowX: "auto" }}>
      <table style={{ borderCollapse: "collapse", width: "100%", ...text.body }}>
        <thead>
          <tr>
            {head.map((h) => (
              <th
                key={h}
                style={{
                  textAlign: "left",
                  padding: `${space(2)} ${space(3)}`,
                  borderBottom: `2px solid ${brand.line}`,
                  color: brand.muted,
                  ...text.small,
                  fontWeight: 700,
                  whiteSpace: "nowrap",
                }}
              >
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  );
}

export function Td({ children, mono = false }: { children: ReactNode; mono?: boolean }) {
  return (
    <td
      style={{
        padding: `${space(2)} ${space(3)}`,
        borderBottom: `1px solid ${brand.line}`,
        color: brand.ink,
        ...(mono ? text.mono : {}),
      }}
    >
      {children}
    </td>
  );
}

export function Code({ children }: { children: ReactNode }) {
  return (
    <pre
      style={{
        ...text.mono,
        background: brand.canvas,
        border: `1px solid ${brand.line}`,
        borderRadius: 8,
        padding: space(3),
        overflowX: "auto",
        margin: 0,
        lineHeight: 1.6,
      }}
    >
      {children}
    </pre>
  );
}

export function Loading({ what }: { what: string }) {
  return (
    <p
      style={{
        ...text.body,
        color: brand.muted,
        padding: `${space(6)} ${space(6)}`,
        maxWidth: 1180,
        margin: "0 auto",
      }}
    >
      Loading {what}…
    </p>
  );
}

export function Failed({ what, error }: { what: string; error: string }) {
  return (
    <div style={{ padding: `${space(6)} ${space(6)} 0`, maxWidth: 1180, margin: "0 auto" }}>
      <Card>
        <p style={{ ...text.body, color: brand.high, margin: 0 }}>
          Could not load {what}. The API is unreachable from this browser.
        </p>
        <p style={{ ...text.small, color: brand.muted, margin: `${space(2)} 0` }}>
          Nothing is shown in its place: an empty chart here would read as “no risk”, and this page
          does not guess. Start the stack with <code>make up</code> and reload.
        </p>
        <p style={{ ...text.small, color: brand.muted, marginBottom: 0 }}>{error}</p>
      </Card>
    </div>
  );
}
