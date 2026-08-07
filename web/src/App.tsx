// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from "react";

import { Logo } from "./Logo";
import { brand, space, text } from "./ui";
import { AlertsView } from "./views/AlertsView";
import { ClimateView } from "./views/ClimateView";
import { DataView } from "./views/DataView";
import { LedgerView } from "./views/LedgerView";
import { MapView } from "./views/MapView";
import { ModelView } from "./views/ModelView";
import { PipelineView } from "./views/PipelineView";

// Each view exists to let a reviewer verify one claim against live data.
// Hash routing keeps every view linkable without adding a router dependency.
const VIEWS = [
  { id: "map", label: "Risk map", proves: "Current risk by county", render: () => <MapView /> },
  { id: "model", label: "Model", proves: "There is a scoring model, and its thresholds were checked", render: () => <ModelView /> },
  { id: "climate", label: "Weather", proves: "Risk reacts to the forecast", render: () => <ClimateView /> },
  { id: "alerts", label: "Messaging", proves: "Guardians are reachable by SMS", render: () => <AlertsView /> },
  { id: "ledger", label: "History", proves: "Immunization history is tamper-evident", render: () => <LedgerView /> },
  { id: "pipeline", label: "Automation", proves: "It runs unattended", render: () => <PipelineView /> },
  { id: "data", label: "Open data", proves: "Anyone can replicate this", render: () => <DataView /> },
] as const;

type ViewId = (typeof VIEWS)[number]["id"];

function currentView(): ViewId {
  const id = window.location.hash.replace(/^#\/?/, "");
  return (VIEWS.find((v) => v.id === id)?.id ?? "map") as ViewId;
}

export default function App(): React.JSX.Element {
  const [view, setView] = useState<ViewId>(currentView);

  useEffect(() => {
    const onHash = () => setView(currentView());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  const active = VIEWS.find((v) => v.id === view) ?? VIEWS[0];

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
        <Logo size={22} />
        <strong style={{ ...text.h1, fontSize: 19 }}>ClimateShield</strong>
        <span style={{ ...text.small, opacity: 0.85 }}>
          Climate-linked outbreak risk — public county aggregates
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

      <main style={{ flex: "1 1 auto", minHeight: 0, overflowY: "auto" }}>{active.render()}</main>

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
