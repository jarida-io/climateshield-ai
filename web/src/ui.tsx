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
  lede: string;
  children: ReactNode;
}) {
  return (
    <div style={{ padding: `${space(6)} ${space(6)} ${space(10)}`, maxWidth: 1180, margin: "0 auto" }}>
      <h1 style={{ ...text.h1, margin: 0, color: brand.ink }}>{title}</h1>
      <p style={{ ...text.body, color: brand.muted, margin: `${space(2)} 0 ${space(5)}` }}>{lede}</p>
      {children}
    </div>
  );
}

/**
 * Caveat states, on the screen itself, what this view does NOT prove. Every
 * view carries one. A reviewer should never have to ask "but is that real?"
 * and get the answer only in conversation.
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
  return <p style={{ ...text.body, color: brand.muted }}>Loading {what}…</p>;
}

export function Failed({ what, error }: { what: string; error: string }) {
  return (
    <Card>
      <p style={{ ...text.body, color: brand.high, margin: 0 }}>
        Could not load {what}. The API is unreachable from this browser.
      </p>
      <p style={{ ...text.small, color: brand.muted, marginBottom: 0 }}>{error}</p>
    </Card>
  );
}
