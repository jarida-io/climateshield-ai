// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from "react";

import { publicClient } from "./api";
import { Logo } from "./Logo";
import { Caveat, Chip, brand, space, text } from "./ui";
import { dataOf, useApi, useStaleData, useTitle } from "./useApi";
import { AlertsView } from "./views/AlertsView";
import { BriefingView } from "./views/BriefingView";
import { ClimateView } from "./views/ClimateView";
import { DataView } from "./views/DataView";
import { LedgerView } from "./views/LedgerView";
import { MapView } from "./views/MapView";
import { ModelView } from "./views/ModelView";
import { OverviewView } from "./views/OverviewView";
import { PipelineView } from "./views/PipelineView";

// Each view exists to let a reviewer verify one claim against live data.
// Hash routing keeps every view linkable without adding a router dependency.
//
// Overview comes first and is the default route. The dashboard used to open on
// the risk map behind a disease filter and an amber box, which meant the first
// thing a new reader met was a control they could not yet use and a warning
// about something they had not yet seen. The work underneath is strong; the
// front door should say what it is for.
const VIEWS = [
  { id: "overview", label: "Overview", proves: "What this system does, and what runs today", render: () => <OverviewView /> },
  { id: "map", label: "Risk map", proves: "Current risk by county", render: () => <MapView /> },
  { id: "model", label: "Model", proves: "There is a scoring model, and its thresholds were checked", render: () => <ModelView /> },
  { id: "climate", label: "Weather", proves: "Risk reacts to the forecast", render: () => <ClimateView /> },
  { id: "briefing", label: "Briefing", proves: "A plain-language county summary built from the published aggregates", render: () => <BriefingView /> },
  { id: "alerts", label: "Messaging", proves: "The alert path renders, gates and records messages", render: () => <AlertsView /> },
  { id: "ledger", label: "History", proves: "Immunization history is tamper-evident", render: () => <LedgerView /> },
  { id: "pipeline", label: "Automation", proves: "It runs unattended", render: () => <PipelineView /> },
  { id: "data", label: "Open data", proves: "Anyone can replicate this", render: () => <DataView /> },
] as const;

type ViewId = (typeof VIEWS)[number]["id"];

function currentView(): ViewId {
  const id = window.location.hash.replace(/^#\/?/, "");
  return (VIEWS.find((v) => v.id === id)?.id ?? "overview") as ViewId;
}

export default function App(): React.JSX.Element {
  const [view, setView] = useState<ViewId>(currentView);

  useEffect(() => {
    const onHash = () => setView(currentView());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  const active = VIEWS.find((v) => v.id === view) ?? VIEWS[0];
  useTitle(`${active.label} · ClimateShield`);

  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        height: "100%",
        background: brand.canvas,
        color: brand.ink,
      }}
    >
      <a
        href="#main"
        style={{
          position: "absolute",
          left: -9999,
          top: 0,
          background: brand.surface,
          color: brand.ink,
          padding: space(3),
          zIndex: 10,
        }}
        onFocus={(e) => {
          e.currentTarget.style.left = "8px";
        }}
        onBlur={(e) => {
          e.currentTarget.style.left = "-9999px";
        }}
      >
        Skip to content
      </a>

      <header
        style={{
          padding: `${space(3)} ${space(5)}`,
          background: brand.navy,
          color: "#fff",
          display: "flex",
          alignItems: "center",
          gap: space(3),
          flexWrap: "wrap",
          flex: "none",
        }}
      >
        <a
          href="#/overview"
          style={{
            display: "flex",
            alignItems: "center",
            gap: space(3),
            color: "inherit",
            textDecoration: "none",
          }}
        >
          <Logo size={22} />
          <strong style={{ ...text.h1, fontSize: 19 }}>ClimateShield</strong>
        </a>
        <span style={{ ...text.small, opacity: 0.85, lineHeight: 1.5 }}>
          Climate-responsive early warning for child immunization in Kenya
        </span>
      </header>

      <nav
        aria-label="Views"
        style={{
          display: "flex",
          gap: space(1),
          padding: `0 ${space(5)}`,
          background: brand.navySoft,
          overflowX: "auto",
          flex: "none",
          alignItems: "stretch",
        }}
      >
        {VIEWS.map((v) => {
          const isActive = v.id === active.id;
          return (
            <a
              key={v.id}
              href={`#/${v.id}`}
              title={v.proves}
              aria-current={isActive ? "page" : undefined}
              style={{
                ...text.small,
                color: isActive ? "#fff" : "rgba(255,255,255,0.72)",
                fontWeight: isActive ? 700 : 400,
                textDecoration: "none",
                padding: `${space(3)} ${space(4)}`,
                borderBottom: `3px solid ${isActive ? brand.purple : "transparent"}`,
                whiteSpace: "nowrap",
              }}
            >
              {v.label}
            </a>
          );
        })}
      </nav>

      <StatusStrip />

      <main id="main" style={{ flex: "1 1 auto", minHeight: 0, overflowY: "auto" }}>
        <StaleNotice />
        {active.render()}
      </main>

      <footer
        style={{
          ...text.small,
          flex: "none",
          borderTop: `1px solid ${brand.line}`,
          background: brand.surface,
          color: brand.muted,
          padding: `${space(2)} ${space(5)}`,
          display: "flex",
          gap: space(4),
          flexWrap: "wrap",
        }}
      >
        <span>Aggregate data only — no personal information is served.</span>
        <span style={{ marginLeft: "auto" }}>Apache-2.0 · open data · no credentials required</span>
      </footer>
    </div>
  );
}

/**
 * The three facts that decide how to read every number on every screen —
 * where the weather came from, whether messages are actually delivered, and
 * which scorer produced the levels — read from the live API and shown under
 * the nav on all views.
 *
 * It exists mostly for the second one. The mock channel used to be disclosed
 * only on the messaging view, so a reader who opened the risk map and left
 * could reasonably have believed alerts were going out. Now the disclosure
 * follows them.
 */
function StatusStrip() {
  const climate = useApi(() => publicClient.getClimateSeries({}), []);
  const alerts = useApi(() => publicClient.getAlertSummary({}), []);
  const model = useApi(() => publicClient.getModelInfo({}), []);

  const sources = [...new Set((dataOf(climate)?.series ?? []).map((s) => s.source))];
  const a = dataOf(alerts);
  const m = dataOf(model);

  return (
    <div
      style={{
        flex: "none",
        background: brand.surface,
        borderBottom: `1px solid ${brand.line}`,
        padding: `${space(2)} ${space(5)}`,
        display: "flex",
        gap: space(2),
        flexWrap: "wrap",
        alignItems: "center",
      }}
    >
      <span style={{ ...text.small, color: brand.muted, fontWeight: 700, marginRight: space(1) }}>
        This deployment
      </span>

      <Chip
        tone={sources.length === 0 ? "neutral" : sources.includes("fixture") ? "info" : "good"}
        title="Read back from the stored observations, not from configuration."
      >
        {climate.kind === "loading"
          ? "weather: checking…"
          : sources.length === 0
            ? "weather: unavailable"
            : `weather: ${sources.join(" + ")}`}
      </Chip>

      <Chip
        tone={a === undefined ? "neutral" : a.channelSends ? "warn" : "info"}
        title={a?.channelNote ?? ""}
      >
        {alerts.kind === "loading"
          ? "messaging: checking…"
          : a === undefined
            ? "messaging: unavailable"
            : a.channelSends
              ? `messaging: ${a.channel} — delivers messages`
              : `messaging: ${a.channel} — no SMS is sent`}
      </Chip>

      <Chip tone="neutral" title="The predictor that produced the risk levels on these screens.">
        {model.kind === "loading"
          ? "scorer: checking…"
          : m === undefined
            ? "scorer: unavailable"
            : `scorer: ${m.activePredictor} v${m.activeVersion}`}
      </Chip>

      <Chip
        tone="neutral"
        title="Children, guardians and phone numbers in this deployment are invented for the demonstration."
      >
        demo population: fictional
      </Chip>
    </div>
  );
}

/**
 * The public API answers reads from its last good response when the database
 * is unreachable, and marks those responses stale. Say so, once, rather than
 * letting old numbers pass for current ones.
 */
function StaleNotice() {
  const stale = useStaleData();
  if (!stale) return null;
  return (
    <div style={{ padding: `${space(4)} ${space(6)} 0`, maxWidth: 1180, margin: "0 auto" }}>
      <Caveat>
        <strong>Showing the last good response.</strong> The database is currently unreachable from
        the API, so at least one endpoint on this page answered from its cache. The figures were
        true when they were cached; they are not being refreshed right now.
      </Caveat>
    </div>
  );
}
