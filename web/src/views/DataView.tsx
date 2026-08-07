// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";

import { Button, Select } from "../forms";
import { Caveat, Card, Code, Page, Table, Td, brand, space, text } from "../ui";

const ENDPOINTS: { path: string; what: string; formats: string }[] = [
  { path: "/v1/risk/current", what: "Latest risk per county × disease", formats: "JSON · CSV · GeoJSON" },
  { path: "/v1/risk/history", what: "Historical scores, filterable by county, disease and date", formats: "JSON · CSV · GeoJSON" },
  { path: "/v1/stats", what: "Per-county counts derived from people (k≥10 suppressed)", formats: "JSON · CSV" },
  { path: "/v1/model", what: "Active predictor, published thresholds, reference record", formats: "JSON" },
  { path: "/v1/climate/series", what: "The forecast window each score was computed from", formats: "JSON" },
  { path: "/v1/ledger/summary", what: "Daily Merkle roots and anchors", formats: "JSON" },
  { path: "/v1/alerts/summary", what: "Messaging outcomes and templates", formats: "JSON" },
  { path: "/v1/pipeline", what: "Job history and data volumes", formats: "JSON" },
  { path: "/health", what: "Readiness; 503 when the database is unreachable", formats: "JSON" },
  { path: "/metrics", what: "Prometheus metrics", formats: "text" },
];

/**
 * Proves: the data is genuinely open and the system is replicable — free
 * inputs, unauthenticated outputs, committed fixtures, Apache-2.0 code.
 */
export function DataView() {
  const origin = typeof window === "undefined" ? "" : window.location.origin;

  return (
    <Page
      title="Open data & replication"
      lede="Every number on this dashboard is available to anyone, unauthenticated, in machine-readable form."
    >
      <Caveat>
        <strong>These are derived outputs, not a training dataset.</strong> There is no training
        corpus to publish because no model has been trained on health outcomes. What is published
        is the climate reference record, the scores computed from it, and the aggregate counts —
        with any count that could identify a child withheld.
      </Caveat>

      <Card title="Endpoints">
        <p style={{ ...text.body, color: brand.muted, marginTop: 0 }}>
          No API key, no account, no rate-limit gate. Add <code>?format=csv</code> or{" "}
          <code>?format=geojson</code> where offered.
        </p>
        <Table head={["Endpoint", "What it returns", "Formats", ""]}>
          {ENDPOINTS.map((e) => (
            <tr key={e.path}>
              <Td mono>{e.path}</Td>
              <Td>{e.what}</Td>
              <Td>{e.formats}</Td>
              <Td>
                <a href={`${origin}${e.path}`} target="_blank" rel="noreferrer" style={{ color: brand.purple }}>
                  open
                </a>
              </Td>
            </tr>
          ))}
        </Table>
      </Card>

      <ApiExplorer origin={origin} />

      <Card title="Try it">
        <Code>{`curl -s ${origin}/v1/risk/current | jq .
curl -s "${origin}/v1/risk/current?format=csv"
curl -s "${origin}/v1/risk/current?format=geojson" | jq .type
curl -s ${origin}/v1/model | jq '.rules'`}</Code>
      </Card>

      <Card title="What makes this replicable">
        <ul style={{ ...text.body, color: brand.ink, margin: 0, paddingLeft: space(5), lineHeight: 1.7 }}>
          <li>
            <strong>Open inputs.</strong> Forecasts and the reference record both come from
            Open-Meteo — free, no API key. Reference data is CC BY 4.0.
          </li>
          <li>
            <strong>Committed fixtures.</strong> The demo scenario ships in the repository, so
            anyone cloning it reproduces identical results offline.
          </li>
          <li>
            <strong>Open code.</strong> Apache-2.0 throughout, with every dependency open source
            and free. The stack builds, tests and runs with zero credentials.
          </li>
          <li>
            <strong>Open contract.</strong> The API is defined in Protobuf; Go and TypeScript
            clients are generated from the same schema this page consumes.
          </li>
        </ul>
      </Card>
    </Page>
  );
}

/**
 * Runs a real request against this deployment and shows the response. It is
 * the "open data" claim made checkable: a reviewer picks an endpoint and a
 * format and sees exactly what a third party would receive.
 */
function ApiExplorer({ origin }: { origin: string }) {
  const [path, setPath] = useState("/v1/risk/current");
  const [format, setFormat] = useState("json");
  const [body, setBody] = useState("");
  const [status, setStatus] = useState("");
  const [busy, setBusy] = useState(false);

  const supportsFormat = path.startsWith("/v1/risk") || path === "/v1/stats";
  const url = supportsFormat && format !== "json" ? `${path}?format=${format}` : path;

  const run = () => {
    setBusy(true);
    setStatus("");
    fetch(url)
      .then(async (r) => {
        const t = await r.text();
        setStatus(`HTTP ${r.status}${r.headers.get("X-Data-Stale") === "true" ? " · X-Data-Stale: true" : ""}`);
        try {
          setBody(JSON.stringify(JSON.parse(t), null, 2).slice(0, 4000));
        } catch {
          setBody(t.slice(0, 4000));
        }
      })
      .catch((e: unknown) => {
        setStatus("request failed");
        setBody(String(e));
      })
      .finally(() => setBusy(false));
  };

  return (
    <Card title="Run a request">
      <form
        onSubmit={(e) => e.preventDefault()}
        style={{ display: "flex", gap: space(4), flexWrap: "wrap", alignItems: "flex-end", marginBottom: space(4) }}
      >
        <Select
          label="Endpoint"
          value={path}
          onChange={setPath}
          options={ENDPOINTS.map((e) => ({ value: e.path, label: e.path }))}
        />
        <Select
          label="Format"
          value={format}
          onChange={setFormat}
          options={[
            { value: "json", label: "JSON" },
            { value: "csv", label: "CSV" },
            { value: "geojson", label: "GeoJSON" },
          ]}
          hint={supportsFormat ? "" : "this endpoint serves JSON only"}
        />
        <Button kind="primary" onClick={run}>
          {busy ? "Running…" : "Send request"}
        </Button>
      </form>

      <Code>{`curl -s "${origin}${url}"`}</Code>

      {status !== "" && (
        <p style={{ ...text.small, color: brand.muted, marginTop: space(3), marginBottom: space(2) }}>
          {status}
        </p>
      )}
      {body !== "" && (
        <pre
          style={{
            ...text.mono,
            background: brand.canvas,
            border: `1px solid ${brand.line}`,
            borderRadius: 8,
            padding: space(3),
            maxHeight: 320,
            overflow: "auto",
            margin: 0,
          }}
        >
          {body}
        </pre>
      )}
    </Card>
  );
}
