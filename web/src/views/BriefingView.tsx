// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";

import { publicClient } from "../api";
import { Select } from "../forms";
import type {
  BriefingFacts,
  GetBriefingResponse,
  GroundingNote,
} from "../gen/climateshield/v1/briefing_pb";
import {
  Caveat,
  Card,
  Code,
  Failed,
  Loading,
  Page,
  StatTile,
  Table,
  Td,
  TileRow,
  brand,
  space,
  text,
} from "../ui";
import { ts, useApi } from "../useApi";

const LANGS = [
  { value: "en", label: "English" },
  { value: "sw", label: "Kiswahili" },
];

/** Human wording for each grounding violation kind the API can report. */
const KIND_LABELS: Record<string, string> = {
  unknown_number: "a number that is not in the fact sheet",
  foreign_county: "a county the briefing was not about",
  unknown_disease: "a disease this system does not score",
  level_mismatch: "a disease put at the wrong risk level",
  forbidden_claim: "a claim this system cannot support",
  possible_name: "something shaped like a person’s name",
  possible_phone: "something shaped like a phone number",
  mock_label: "text impersonating the “[mock]” label",
  too_short: "a draft too short to be a briefing",
  generator_unavailable: "the generator could not be reached",
};

function label(kind: string): string {
  return KIND_LABELS[kind] ?? kind;
}

/** Short form of the fact-sheet hash, which ties text to the facts behind it. */
function shortHash(hash: string): string {
  return hash.length > 12 ? `${hash.slice(0, 12)}…` : hash;
}

/**
 * Proves: the numbers this system publishes can be read as language a county
 * health officer can act on — without a model being able to invent anything.
 *
 * Every sentence of the briefing on the right must be supported by the fact
 * sheet on the left; a draft that is not, is refused before it reaches this
 * page, and the refusal is shown rather than hidden. Nothing on this page is
 * written by a model unless the provenance line says a model wrote it.
 */
export function BriefingView() {
  const [county, setCounty] = useState("Kisumu");
  const [lang, setLang] = useState("en");

  const briefing = useApi(
    () => publicClient.getBriefing({ area: county, lang }),
    [county, lang],
  );
  const risk = useApi(() => publicClient.getCurrentRisk({}), []);

  if (briefing.kind === "loading") return <Loading what="county briefing" />;
  if (briefing.kind === "error") return <Failed what="county briefing" error={briefing.message} />;
  const b = briefing.data;

  const counties =
    risk.kind === "ready"
      ? Array.from(new Set(risk.data.scores.map((s) => s.area))).sort()
      : [county];

  return (
    <Page
      title="County briefing"
      lede="The same aggregate facts this system publishes, written as plain language a county health officer can act on — in English and Kiswahili, with every number traceable to the fact sheet beside it."
    >
      <Caveat>
        <strong>This is not clinical guidance.</strong> The risk levels come from the published
        thresholds applied to a weather forecast; they describe weather, not an outbreak, and this
        system holds no outbreak surveillance data. No generated text is ever sent to a guardian —
        SMS comes only from the fixed, length-checked templates. The Kiswahili wording has{" "}
        <strong>not been reviewed by a Kiswahili speaker</strong> and stays labelled that way until
        one signs it off.
      </Caveat>

      <TileRow>
        <StatTile
          label="Written by"
          value={b.model === "" || b.model === "none" ? "no model" : b.model}
          hint={
            b.generator === "" ? "nothing generated yet" : `generator: ${b.generator}`
          }
        />
        <StatTile
          label="Grounding check"
          value={statusWord(b)}
          hint={groundingHint(b)}
        />
        <StatTile
          label="Fact sheet"
          value={b.factsHashHex === "" ? "—" : shortHash(b.factsHashHex)}
          hint="SHA-256 of the exact facts the text was written from"
        />
        <StatTile
          label="Written at"
          value={b.createdAt === undefined ? "—" : ts(b.createdAt)}
          hint={`prompt ${b.promptVersion === "" ? "—" : b.promptVersion}`}
        />
      </TileRow>

      <form
        onSubmit={(e) => e.preventDefault()}
        style={{
          display: "flex",
          gap: space(4),
          flexWrap: "wrap",
          alignItems: "flex-end",
          padding: space(4),
          background: brand.surface,
          border: `1px solid ${brand.line}`,
          borderRadius: 10,
          marginBottom: space(5),
        }}
      >
        <Select
          label="County"
          value={county}
          onChange={setCounty}
          options={counties.map((c) => ({ value: c, label: c }))}
        />
        <Select
          label="Language"
          value={lang}
          onChange={setLang}
          options={LANGS}
          hint="the same facts, written twice — not a machine translation of the English"
        />
      </form>

      <div
        style={{
          display: "flex",
          gap: space(5),
          flexWrap: "wrap",
          alignItems: "flex-start",
        }}
      >
        <div style={{ flex: "1 1 380px", minWidth: 0 }}>
          <FactSheetCard facts={b.facts} />
        </div>
        <div style={{ flex: "1 1 420px", minWidth: 0 }}>
          <BriefingCard briefing={b} />
        </div>
      </div>

      <Card title="Why a language model cannot invent anything here">
        <ol
          style={{
            ...text.body,
            color: brand.ink,
            margin: 0,
            paddingLeft: space(5),
            lineHeight: 1.7,
          }}
        >
          <li>
            <strong>The generator is only ever shown the fact sheet on the left.</strong> The
            structure it receives has no field for a child, a guardian, a phone number or any count
            below the k≥10 threshold, so none can reach a prompt.
          </li>
          <li>
            <strong>The default generator is not a model at all.</strong> A deterministic template
            writes the briefing, and says so in its first line. A model is opt-in per deployment.
          </li>
          <li>
            <strong>Every model draft is checked against its own fact sheet.</strong> A number,
            county, disease or risk level the facts do not support gets the draft refused — and the
            template is served instead, with the reasons shown above.
          </li>
          <li>
            <strong>Nothing generated reaches a guardian.</strong> Alert SMS is rendered from fixed
            templates that are length-checked and tested, and the mock channel sends nothing at all.
          </li>
        </ol>
      </Card>

      <Card title="Check it yourself">
        <p style={{ ...text.body, color: brand.muted, marginTop: 0 }}>
          The endpoint returns the text and the fact sheet together, so every number above can be
          traced without trusting this page.
        </p>
        <Code>{`curl -s "${origin()}/v1/briefings?area=${encodeURIComponent(county)}&lang=${lang}" | jq .
curl -s "${origin()}/v1/briefings?area=${encodeURIComponent(county)}&lang=${lang}" | jq -r '.body'
curl -s "${origin()}/v1/briefings?area=${encodeURIComponent(county)}&lang=${lang}" | jq '.facts, .groundingNotes'`}</Code>
      </Card>
    </Page>
  );
}

function origin(): string {
  return typeof window === "undefined" ? "" : window.location.origin;
}

function statusWord(b: GetBriefingResponse): string {
  switch (b.status) {
    case "served":
      return b.generator === "mock" ? "not needed" : "passed";
    case "rejected":
      return "refused";
    case "unavailable":
      return "no draft";
    default:
      return "—";
  }
}

function groundingHint(b: GetBriefingResponse): string {
  switch (b.status) {
    case "served":
      return b.generator === "mock"
        ? "the template is written from the facts themselves"
        : "every number, county, disease and level matched the fact sheet";
    case "rejected":
      return "a model draft was refused; the template is shown instead";
    case "unavailable":
      return "the configured generator could not be reached";
    default:
      return "nothing has been generated for this county yet";
  }
}

function BriefingCard({ briefing }: { briefing: GetBriefingResponse }) {
  const empty = briefing.status === "none" || briefing.body === "";
  return (
    <Card title={`Briefing — ${briefing.area === "" ? "—" : briefing.area}`}>
      {empty ? (
        <p style={{ ...text.body, color: brand.muted, margin: 0 }}>
          {briefing.note === ""
            ? "No briefing has been written for this county and language yet."
            : briefing.note}
        </p>
      ) : (
        <>
          <p
            style={{
              ...text.small,
              color: brand.muted,
              margin: `0 0 ${space(3)}`,
              lineHeight: 1.6,
            }}
          >
            {briefing.provenance}
          </p>
          <div
            style={{
              ...text.body,
              color: brand.ink,
              background: brand.canvas,
              border: `1px solid ${brand.line}`,
              borderRadius: 8,
              padding: space(4),
              lineHeight: 1.7,
              whiteSpace: "pre-wrap",
            }}
          >
            {briefing.body}
          </div>
          <GroundingNotes notes={briefing.groundingNotes} status={briefing.status} />
        </>
      )}
    </Card>
  );
}

function GroundingNotes({ notes, status }: { notes: GroundingNote[]; status: string }) {
  if (notes.length === 0) return null;
  return (
    <div style={{ marginTop: space(4) }}>
      <h3 style={{ ...text.h2, margin: `0 0 ${space(2)}`, color: brand.ink }}>
        {status === "rejected"
          ? "Why the model draft was refused"
          : "What the generator reported"}
      </h3>
      <ul
        style={{
          ...text.small,
          color: brand.ink,
          margin: 0,
          paddingLeft: space(5),
          lineHeight: 1.7,
        }}
      >
        {notes.map((n, i) => (
          <li key={`${n.kind}-${i}`}>
            <strong>{label(n.kind)}</strong>
            {n.excerpt === "" ? "" : `: “${n.excerpt}”`}
            <span style={{ color: brand.muted }}> — {n.detail}</span>
          </li>
        ))}
      </ul>
      <p style={{ ...text.small, color: brand.muted, margin: `${space(2)} 0 0` }}>
        The refused text itself is not stored or shown: it may contain an invented name, and
        repeating it would be repeating exactly what the check exists to stop.
      </p>
    </div>
  );
}

function FactSheetCard({ facts }: { facts: BriefingFacts | undefined }) {
  if (facts === undefined) {
    return (
      <Card title="Fact sheet">
        <p style={{ ...text.body, color: brand.muted, margin: 0 }}>
          No fact sheet yet — nothing has been generated for this county and language.
        </p>
      </Card>
    );
  }
  return (
    <Card title="Fact sheet the briefing was written from">
      <p style={{ ...text.small, color: brand.muted, marginTop: 0 }}>
        {facts.windowFrom === ""
          ? "No forecast window has been ingested for this county yet."
          : `Forecast window ${facts.windowFrom} to ${facts.windowTo} (${facts.windowDays} days, source: ${facts.windowSource}).`}
      </p>

      <Table head={["Disease", "Level", "Driver", "Value"]}>
        {facts.scores.map((s) => (
          <tr key={s.disease}>
            <Td>{s.disease}</Td>
            <Td>{s.level}</Td>
            <Td>{s.driver}</Td>
            <Td mono>{s.driverValue.toFixed(1)}</Td>
          </tr>
        ))}
      </Table>

      {facts.scores.length > 0 && (
        <ul
          style={{
            ...text.small,
            color: brand.muted,
            margin: `${space(3)} 0 0`,
            paddingLeft: space(5),
            lineHeight: 1.7,
          }}
        >
          {facts.scores
            .filter((s) => s.explanation !== "")
            .map((s) => (
              <li key={`why-${s.disease}`}>
                <strong>{s.disease}:</strong> {s.explanation}
              </li>
            ))}
        </ul>
      )}

      <h3 style={{ ...text.h2, margin: `${space(4)} 0 ${space(2)}`, color: brand.ink }}>
        Messaging, across all monitored counties
      </h3>
      <Table head={["Outcome", "Count"]}>
        {facts.alertsAllCounties.map((a) => (
          <tr key={a.status}>
            <Td>{a.status}</Td>
            <Td mono>{a.suppressed ? "withheld (k<10)" : String(a.count ?? 0n)}</Td>
          </tr>
        ))}
      </Table>
      <p style={{ ...text.small, color: brand.muted, margin: `${space(2)} 0 0` }}>
        {facts.channelNote} These counts are system-wide, not this county’s: a per-county alert
        count is people-derived and is published, suppressed county by county, on{" "}
        <code>/v1/stats</code>.
      </p>
    </Card>
  );
}
