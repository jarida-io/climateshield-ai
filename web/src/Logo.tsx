// SPDX-License-Identifier: Apache-2.0

/**
 * ClimateShield mark: two overlapping planes forming a shield silhouette.
 * Drawn as vector geometry rather than a raster asset so it stays crisp at
 * any size, carries a transparent background, and adds no network request.
 */
export const brandPurple = "#7C4DFF";

export function Logo({ size = 26, title = "ClimateShield" }: { size?: number; title?: string }) {
  return (
    <svg
      viewBox="0 0 120 100"
      width={(size * 120) / 100}
      height={size}
      role="img"
      aria-label={title}
      style={{ display: "block", flex: "none" }}
    >
      {/* Right plane, then the left plane over it. The hairline on the
          overlap is what reads as two separate surfaces rather than one. */}
      <path d="M53 25 L120 0 L120 100 L53 75 Z" fill={brandPurple} />
      <path
        d="M0 0 L67 25 L67 75 L0 100 Z"
        fill={brandPurple}
        stroke="rgba(255,255,255,0.45)"
        strokeWidth="1.4"
        strokeLinejoin="round"
      />
    </svg>
  );
}
