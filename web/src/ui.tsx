// SPDX-License-Identifier: Apache-2.0

// Shared design system. Kept deliberately small: plain objects and a handful
// of components, no CSS framework, so the whole visual language is auditable
// in one file.

import type { CSSProperties, ReactNode } from "react";

export const brand = {
  // The ground is the colour of an overcast day over the lake: a cool, damp
  // paper rather than the warm cream or the blue-grey every dashboard ships
  // with. It is the one surface the reader sees on every screen, so it is
  // where the design decides what kind of instrument this is.
  canvas: "#EDF1F0",
  surface: "#FFFFFF",
  // Lake Victoria at depth. Structure, headings and the header band. It
  // replaces a generic navy with a colour that belongs to the five counties
  // this system actually watches.
  navy: "#0E3733",
  navySoft: "#1A4A45",
  ink: "#12211F",
  muted: "#4A6360",
  line: "#D6DEDC",
  lineStrong: "#B9C6C3",

  // Jarida's mark is purple, so purple stays — on the mark. It no longer
  // colours buttons, links and furniture that have nothing to do with the
  // brand, which is what made it read as decoration.
  purple: "#7C4DFF",
  purpleDim: "#EFEAFF",
  purpleInk: "#4B2AC2",

  // Status palette, unchanged: these three cleared 4.5:1 against white label
  // text after an earlier amber failed at 2.07:1, and this is a public health
  // tool. Re-tuning them for a new ground would be re-running a solved
  // accessibility problem for the sake of taste.
  high: "#C1121F",
  medium: "#B45309",
  low: "#12715F",
  warn: "#8A6100",
  warnBg: "#FFF6E0",
  lowBg: "#E3F1EC",
  highBg: "#FCE8EA",
} as const;

export const space = (n: number) => `${n * 4}px`;

export const text = {
  // A modular scale, roughly 1.25, from a 15px body. Plex Sans has enough
  // x-height to read at 15 where the previous rounded display face needed
  // more, so the page carries more information at the same apparent size.
  display: { fontSize: 30, fontWeight: 600, letterSpacing: "-0.02em", lineHeight: 1.15 },
  h1: { fontSize: 24, fontWeight: 600, letterSpacing: "-0.015em", lineHeight: 1.25 },
  h2: { fontSize: 19, fontWeight: 600, letterSpacing: "-0.01em", lineHeight: 1.3 },
  h3: { fontSize: 15, fontWeight: 600, letterSpacing: "0" },
  body: { fontSize: 15, fontWeight: 400, lineHeight: 1.55 },
  small: { fontSize: 13, fontWeight: 400, lineHeight: 1.5 },
  micro: { fontSize: 12, fontWeight: 500, lineHeight: 1.4 },
  // Machine output: Merkle roots, endpoint paths, driver values. Mono is used
  // for things that ARE machine output, never as decoration on a label.
  mono: { fontFamily: '"IBM Plex Mono", ui-monospace, SFMono-Regular, Menlo, monospace', fontSize: 13 },
  // A figure meant to be read at a glance and compared with its neighbour.
  figure: { fontSize: 28, fontWeight: 600, letterSpacing: "-0.02em", lineHeight: 1.1 },
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
    <div style={{ padding: `${space(7)} ${space(6)} ${space(10)}`, maxWidth: 1180, margin: "0 auto" }}>
      <h1 style={{ ...text.h1, margin: 0, color: brand.ink }}>{title}</h1>
      {/* A lede is read as a sentence, so it is held to a comfortable measure
          rather than run to the full width of a 1180px page. */}
      <p
        className="prose"
        style={{ ...text.body, color: brand.muted, margin: `${space(3)} 0 ${space(6)}`, maxWidth: "62ch" }}
      >
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
        // A rule on the edge, not a box. The boundary of a view is an aside to
        // the evidence beside it, and a full border would give it the same
        // weight as the data — which is how a permanent notice gets skipped.
        borderLeft: `2px solid ${brand.lineStrong}`,
        padding: `${space(1)} 0 ${space(1)} ${space(4)}`,
        margin: `0 0 ${space(6)}`,
        ...text.small,
        color: brand.muted,
        maxWidth: "72ch",
      }}
    >
      <div style={{ ...text.micro, color: brand.ink, marginBottom: space(1) }}>{caption}</div>
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

export function Card({
  title,
  children,
  plain = false,
}: {
  title?: string;
  children: ReactNode;
  /** Prose rather than data: a titled passage with no surface under it. */
  plain?: boolean;
}) {
  if (plain) {
    return (
      <section style={{ marginBottom: space(6), paddingTop: space(2) }}>
        {title !== undefined && (
          <h2
            style={{
              ...text.h2,
              margin: `0 0 ${space(3)}`,
              color: brand.ink,
              paddingBottom: space(2),
              borderBottom: `1px solid ${brand.lineStrong}`,
            }}
          >
            {title}
          </h2>
        )}
        {children}
      </section>
    );
  }
  return (
    <section
      style={{
        background: brand.surface,
        border: `1px solid ${brand.line}`,
        // 4px, not 10. A large radius on every surface regardless of what it
        // holds is the tell of a component kit; a small one reads as a sheet
        // of paper, which is what these are.
        borderRadius: 4,
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
        // A rule over a figure, not a box around it. The number is the thing
        // worth looking at; four identical bordered rectangles make four
        // numbers look like furniture.
        borderTop: `2px solid ${brand.navy}`,
        padding: `${space(3)} ${space(4)} ${space(2)} 0`,
        minWidth: 150,
        flex: "1 1 150px",
      }}
    >
      <div style={{ ...text.micro, color: brand.muted }}>{label}</div>
      {/* A tile holds either a figure or a short string, and the two want
          different sizes: "20" wants to be read across the room, "Aug 7, 2026,
          9:00 AM" wants to fit on one line. The scale follows the content
          rather than forcing every value through one size and letting the long
          ones wrap into the tile below. */}
      <div
        style={{
          ...text.figure,
          fontSize: value.length <= 7 ? 28 : value.length <= 14 ? 21 : 17,
          color: brand.ink,
          marginTop: space(2),
        }}
      >
        {value}
      </div>
      {hint !== undefined && (
        <div style={{ ...text.small, color: brand.muted, marginTop: space(2), maxWidth: "34ch" }}>
          {hint}
        </div>
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
      <table style={{ borderCollapse: "collapse", width: "100%", ...text.small }}>
        <thead>
          <tr>
            {head.map((h, i) => (
              <th
                // Index, not the label: a header row may legitimately carry two
                // blank columns (an actions column beside a spacer), and two
                // identical keys make React drop one of them.
                key={`${h}-${i}`}
                style={{
                  textAlign: "left",
                  padding: `${space(2)} ${space(3)}`,
                  borderBottom: `1.5px solid ${brand.navy}`,
                  color: brand.ink,
                  ...text.micro,
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

/**
 * A file path, flag or identifier named inside a sentence. It is an inline
 * <code>, not the block <pre> that Code renders: a <pre> inside a <p> is
 * invalid HTML, and React reports it as a hydration error.
 */
export function InlineCode({ children }: { children: ReactNode }) {
  return (
    <code
      style={{
        ...text.mono,
        fontSize: "0.92em",
        background: brand.canvas,
        border: `1px solid ${brand.line}`,
        borderRadius: 3,
        padding: "0.05em 0.35em",
        whiteSpace: "nowrap",
      }}
    >
      {children}
    </code>
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
