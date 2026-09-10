// SPDX-License-Identifier: Apache-2.0

import type { ReactNode } from "react";

import { publicClient } from "../api";
import { ForecastHero } from "./ForecastHero";
import type { DailyRoot } from "../gen/climateshield/v1/public_pb";
import { diseaseName, levelName } from "../map";
import {
  Card,
  Chip,
  Disclosure,
  Grid,
  Page,
  StatTile,
  Table,
  Td,
  TileRow,
  brand,
  space,
  text,
} from "../ui";
import type { Tone } from "../ui";
import { dataOf, tsShort, useApi } from "../useApi";

/**
 * The front door.
 *
 * Every figure on this page is read from the public API through the generated
 * client. Nothing here is illustrative: when a call does not answer, the tile
 * says “—” and the sentence around it says what is missing. That is the whole
 * design constraint — a reader must never be able to mistake a placeholder for
 * a measurement, and must never meet a number this system cannot produce.
 */
export function OverviewView() {
  const risk = useApi(() => publicClient.getCurrentRisk({}), []);
  const model = useApi(() => publicClient.getModelInfo({}), []);
  const ledger = useApi(() => publicClient.getLedgerSummary({}), []);
  const alerts = useApi(() => publicClient.getAlertSummary({}), []);
  const pipeline = useApi(() => publicClient.getPipelineStatus({}), []);
  const stats = useApi(() => publicClient.getStats({}), []);
  const climate = useApi(() => publicClient.getClimateSeries({}), []);

  const r = dataOf(risk);
  const m = dataOf(model);
  const l = dataOf(ledger);
  const a = dataOf(alerts);
  const p = dataOf(pipeline);
  const s = dataOf(stats);
  const c = dataOf(climate);

  const scores = r?.scores ?? [];
  const elevated = scores.filter((x) => levelName(x.level) === "HIGH" || levelName(x.level) === "MEDIUM");
  const counties = new Set(scores.map((x) => x.area));
  const elevatedCounties = new Set(elevated.map((x) => x.area));
  const latestScoredAt = scores.reduce<{ seconds: bigint } | undefined>(
    (best, x) =>
      x.scoredAt !== undefined && (best === undefined || x.scoredAt.seconds > best.seconds)
        ? x.scoredAt
        : best,
    undefined,
  );
  const scorer = scores[0];

  // Whether messages leave this deployment decides how several sentences on
  // this page have to be written, so it is read once, from the API.
  const sends = a?.channelSends === true;
  const channel = a?.channel ?? "";

  const unreachable = (m?.rules ?? []).filter((rule) => !rule.reachableInReferencePeriod);
  const newest = newestAnchor(l?.roots ?? []);
  const latestRoot = l?.roots[0];
  const alertsRecorded = a?.statuses.reduce((n, x) => n + Number(x.count ?? 0n), 0);
  const jobsCompleted = p?.jobs
    .filter((j) => j.state === "completed")
    .reduce((n, j) => n + Number(j.count), 0);
  const sources = [...new Set((c?.series ?? []).map((x) => x.source))];

  // The generated client gains this method the day the briefing service ships
  // its RPC. Asking the client, rather than hardcoding a status, means this
  // card cannot go on claiming a service that is not there — or keep denying
  // one that is.
  const briefingWired = "getBriefing" in publicClient;

  return (
    <Page
      title="From the forecast to the clinic record"
      lede="Five Kenyan counties, the weather that drives outbreak risk in them, and a record of the doses given that can be checked rather than taken on trust."
    >
      <ForecastHero />

      <Card title="How this works for a family in Kisumu" plain>
        <div className="prose" style={{ ...text.body, color: brand.ink, maxWidth: "68ch" }}>
          <p style={{ marginTop: 0 }}>
            Kisumu’s own 14-day forecast is ingested on a schedule from Open-Meteo, the same free
            and open source anybody else can query
            {sources.includes("fixture")
              ? " — though this deployment is reading committed fixtures rather than live forecasts, so the demonstration is identical on every machine"
              : ""}
            . When the peak rainfall in that window reaches the cutoff published in the funding
            proposal, the county is scored for cholera risk and the alert path runs.
          </p>
          <p>
            For each child in Kisumu who is due or overdue for a dose, one short message is
            rendered — English or Kiswahili, one GSM-7 segment, the child’s first name and the
            vaccine, never a disease name — then checked against the guardian’s consent and against
            quiet hours before it is recorded.
          </p>
          <p style={{ marginBottom: 0 }}>
            When that dose is given, the clinic’s record becomes a leaf in the day’s Merkle tree, so
            a guardian, a clinic and an auditor can each prove the dose was recorded and that
            nobody has quietly changed it since.{" "}
            {alerts.kind === "loading" ? (
              <em style={{ color: brand.muted }}>Checking what the messaging channel does…</em>
            ) : a === undefined ? (
              <em style={{ color: brand.muted }}>
                The API is not answering, so this page cannot say what the messaging channel is
                doing right now.
              </em>
            ) : sends ? (
              <strong>
                Channel “{channel}” is active in this deployment and delivers messages: {a.channelNote}
              </strong>
            ) : (
              <strong>
                In this deployment the last step of the message stops at the door: the “{channel}”
                channel writes down what it would have sent and sends nothing.
              </strong>
            )}
          </p>
        </div>
      </Card>

      <Grid min={280}>
        <Audience who="A guardian in Kisumu">
          One short SMS naming their own child and the dose that is due, in the language they
          registered in, with STOP honoured from an append-only consent log.{" "}
          {a !== undefined && !sends ? "Here it is written and recorded, not delivered." : ""}
        </Audience>
        <Audience who="A community health worker">
          A county-by-county view of which risk is elevated and the weather figure that raised it,
          so a day’s visits can follow the forecast instead of the calendar.
        </Audience>
        <Audience who="A county health officer">
          The same aggregates as everyone else, unauthenticated and machine-readable, plus doses
          that can be proved recorded — with any count small enough to identify a family withheld.
        </Audience>
      </Grid>

      <Card title="Right now">
        <p style={{ ...text.body, color: brand.ink, margin: 0, lineHeight: 1.7 }}>
          {risk.kind === "loading" ? (
            <span style={{ color: brand.muted }}>Loading current risk…</span>
          ) : r === undefined ? (
            <span style={{ color: brand.muted }}>
              Current risk is not loading from the API, so no level is shown here. An empty map
              would read as “no risk”, which is not what is known.
            </span>
          ) : scores.length === 0 ? (
            <>No county has been scored yet in this deployment.</>
          ) : (
            <>
              <strong>
                {elevated.length} of {scores.length} county–disease pair
                {scores.length === 1 ? "" : "s"}{" "}
                {elevated.length === 1 ? "is" : "are"} elevated
              </strong>{" "}
              {elevated.length === 0
                ? `across the ${counties.size} monitored ${counties.size === 1 ? "county" : "counties"}`
                : `across ${elevatedCounties.size} of ${counties.size} monitored ${counties.size === 1 ? "county" : "counties"}`}
              , scored {tsShort(latestScoredAt)}
              {scorer === undefined ? "" : ` by ${scorer.predictor} v${scorer.predictorVersion}`}.
              {elevated.length > 0 && (
                <>
                  {" "}
                  Elevated now:{" "}
                  {elevated
                    .map((x) => `${x.area} — ${diseaseName(x.disease)} ${levelName(x.level)}`)
                    .join("; ")}
                  .
                </>
              )}
            </>
          )}
        </p>
      </Card>

      <h2 style={{ ...text.h2, color: brand.ink, margin: `${space(6)} 0 ${space(3)}` }}>
        The three things the proposal promised
      </h2>

      <Grid min={260}>
        <PillarCard
          title="A scoring model"
          value={m === undefined ? "—" : `${m.activePredictor} v${m.activeVersion}`}
          hint={
            m === undefined
              ? modelStatus(model.kind)
              : m.referencePeriod === ""
                ? "active predictor, as reported by the API"
                : `reference record ${m.referencePeriod}`
          }
          chip={
            m === undefined
              ? undefined
              : unreachable.length === 0
                ? { tone: "good", label: "every published cutoff is reachable" }
                : {
                    tone: "warn",
                    label: `${unreachable.length} of ${m.rules.length} published cutoffs unreachable`,
                  }
          }
          note={
            unreachable.length === 0
              ? "Two predictors, one active. Which one scored a row is stamped on the row."
              : `${unreachable.map((u) => u.disease).join(" and ")} can never reach HIGH in the reference record. That finding is reported here rather than fixed by moving a contractual number.`
          }
          href="#/model"
          cta="See both predictors and the cutoff check"
        />

        <PillarCard
          title="A tamper-evident record"
          value={l === undefined ? "—" : `${String(l.totalDays)} day${l.totalDays === 1n ? "" : "s"}`}
          hint={
            l === undefined
              ? modelStatus(ledger.kind)
              : latestRoot === undefined
                ? "no day has been committed yet"
                : `latest root ${latestRoot.rootHex.slice(0, 16)}… (${latestRoot.day})`
          }
          chip={
            l === undefined
              ? undefined
              : newest === undefined
                ? { tone: "neutral", label: `anchor mode: ${l.anchorMode}` }
                : {
                    tone: newest.anchorType === "evm" && !newest.readbackMatches ? "bad" : "good",
                    label:
                      newest.anchorType === "evm"
                        ? `chain ${String(newest.chainId)} · read-back ${newest.readbackMatches ? "matches" : "does not match"}`
                        : `anchored: ${newest.anchorType}`,
                    title: newest.anchorType === "evm" ? newest.chainLabel : "",
                  }
          }
          note={l?.anchorNote ?? ""}
          href="#/ledger"
          cta="Check a day’s root yourself"
        />

        <PillarCard
          title="A written briefing"
          value={briefingWired ? "available" : "—"}
          hint={
            briefingWired
              ? "served by the briefing service"
              : "no briefing endpoint in this build"
          }
          chip={
            briefingWired
              ? { tone: "info", label: "open the view for its provenance line" }
              : { tone: "neutral", label: "not wired up yet" }
          }
          note="A county summary in plain language, written from the same public aggregates shown here — with the generator, model and prompt version stated on the text itself."
          href="#/briefing"
          cta="Open the briefing view"
        />

        <PillarCard
          title="Guardian alerts"
          value={alertsRecorded === undefined ? "—" : String(alertsRecorded)}
          hint={alertsRecorded === undefined ? modelStatus(alerts.kind) : "alerts recorded"}
          chip={
            a === undefined
              ? undefined
              : sends
                ? { tone: "warn", label: `${channel} — delivers messages` }
                : { tone: "info", label: `${channel} — sends nothing` }
          }
          note={
            a === undefined
              ? ""
              : `${a.templates.length === 0 ? "No template" : a.templates.map(langName).join(" and ")} rendered, each inside one GSM-7 segment. ${a.channelNote}`
          }
          href="#/alerts"
          cta="Read the templates and the sending rules"
        />
      </Grid>

      <Card title="Running on its own">
        <TileRow>
          <StatTile
            label="Latest forecast ingested"
            value={p === undefined ? "—" : tsShort(p.latestObservationAt)}
          />
          <StatTile label="Jobs completed" value={jobsCompleted === undefined ? "—" : String(jobsCompleted)} />
          <StatTile label="Ingest interval" value={p?.ingestInterval ?? "—"} />
          <StatTile
            label="Risk scores stored"
            value={p === undefined ? "—" : String(p.riskScores)}
          />
        </TileRow>
        <div style={{ display: "flex", gap: space(2), flexWrap: "wrap", alignItems: "center" }}>
          <Chip
            tone={sources.length === 0 ? "neutral" : sources.includes("fixture") ? "info" : "good"}
            title="Read back from the stored observations rather than from configuration."
          >
            {sources.length === 0 ? "weather source: unavailable" : `weather source: ${sources.join(" + ")}`}
          </Chip>
          <span style={{ ...text.small, color: brand.muted }}>
            Postgres-backed jobs with real retry history: ingest → score → alert, plus a ledger
            sweep. <a href="#/pipeline" style={linkStyle}>See the job history</a>
          </span>
        </div>
      </Card>

      <Card title="Privacy, in the open">
        <p style={{ ...text.body, color: brand.muted, marginTop: 0 }}>
          These are the only people-derived numbers this system publishes, and they are county
          totals. Where a county has fewer than ten, the number is withheld rather than rounded:
          on a small population a count is close to a name.
        </p>
        {s === undefined ? (
          <p style={{ ...text.body, color: brand.muted, margin: 0 }}>
            {stats.kind === "loading"
              ? "Loading county counts…"
              : "County counts are not loading from the API, so none are shown."}
          </p>
        ) : (
          <Table head={["County", "Children registered", "Due", "Overdue", "Alerts generated"]}>
            {s.stats.map((row) => (
              <tr key={row.area}>
                <Td>{row.area}</Td>
                <Td mono>{count(row.childrenRegistered, row.childrenRegisteredSuppressed)}</Td>
                <Td mono>{count(row.childrenDue, row.childrenDueSuppressed)}</Td>
                <Td mono>{count(row.childrenOverdue, row.childrenOverdueSuppressed)}</Td>
                <Td mono>{count(row.alertsGenerated, row.alertsGeneratedSuppressed)}</Td>
              </tr>
            ))}
          </Table>
        )}
        <p style={{ ...text.small, color: brand.muted, marginBottom: 0, marginTop: space(3) }}>
          The rule is k≥10 and it is enforced in CI, not by review: a contract test named{" "}
          <code>TestContract_KAnonymity</code> seeds counties above, below and at zero and fails the
          build if a small count ever reaches a public response — in JSON or in the CSV export.
          Alongside it, <code>TestContract_PIILeak</code> walks the public surface looking for
          anything child-shaped.
        </p>
      </Card>

      <Disclosure caption="What this dashboard shows — and does not">
        <ul style={{ margin: 0, paddingLeft: space(5), lineHeight: 1.7 }}>
          <li>
            <strong>Where the weather comes from.</strong>{" "}
            {sources.length === 0
              ? "The climate source is not loading from the API, so this page does not claim one."
              : sources.includes("fixture")
                ? "This deployment is running committed demo fixtures, so the figures are identical on every machine and are not today’s weather."
                : `Live forecasts (${sources.join(", ")}), so these figures will not match the documented demo scenario.`}
          </li>
          <li>
            <strong>What happens to a message.</strong>{" "}
            {a === undefined
              ? "The messaging channel is not loading from the API, so this page does not claim one."
              : sends
                ? `Channel “${channel}” is active and delivers messages.`
                : `Nothing is delivered. The “${channel}” channel records status would_send, never sent, which only a real carrier adapter may write.`}
          </li>
          <li>
            <strong>No accuracy claim is made anywhere.</strong> The scores come from published
            threshold rules and from how unusual the weather is against a reference record. Neither
            has been validated against outbreak or vaccination outcomes, because this system holds
            no such data. There is no accuracy, sensitivity, uptime or “families protected” figure
            on this dashboard, and there should not be one until an evaluation exists.
          </li>
          <li>
            <strong>How the record is anchored.</strong>{" "}
            {l === undefined
              ? "The ledger summary is not loading from the API, so this page does not describe the anchor."
              : l.anchorNote}
          </li>
          <li>
            <strong>Who is in the data.</strong> The children, guardians and phone numbers in this
            deployment are invented for the demonstration.
          </li>
        </ul>
      </Disclosure>
    </Page>
  );
}

const linkStyle = { color: brand.purple, fontWeight: 700, textDecoration: "none" } as const;

/** A people-derived count: the value, “withheld” when suppressed, or “—”. */
function count(value: bigint | undefined, suppressed: boolean): string {
  if (suppressed) return "withheld (k<10)";
  return value === undefined ? "—" : String(value);
}

function langName(t: { lang: string }): string {
  return t.lang === "sw" ? "Kiswahili" : t.lang === "en" ? "English" : t.lang;
}

/** What to say in place of a figure that has not arrived. */
function modelStatus(kind: "loading" | "ready" | "error"): string {
  return kind === "loading" ? "loading…" : "not available from the API right now";
}

/** The newest anchor across all days decides how this page describes anchoring. */
function newestAnchor(roots: DailyRoot[]): DailyRoot | undefined {
  let best: DailyRoot | undefined;
  for (const root of roots) {
    if (root.anchoredAt === undefined) continue;
    if (best?.anchoredAt === undefined || root.anchoredAt.seconds > best.anchoredAt.seconds) {
      best = root;
    }
  }
  return best;
}

function Audience({ who, children }: { who: string; children: ReactNode }) {
  return (
    <div
      style={{
        // Three people, not three products. A rule above each is enough to
        // separate them; boxing prose gives it the weight of data and makes
        // the page read as a component kit rather than an argument.
        borderTop: `1px solid ${brand.lineStrong}`,
        paddingTop: space(3),
      }}
    >
      <div style={{ ...text.h3, color: brand.ink, marginBottom: space(2) }}>{who}</div>
      <div className="prose" style={{ ...text.small, color: brand.muted }}>
        {children}
      </div>
    </div>
  );
}

/**
 * One pillar: a live figure, the standing fact around it, and the view where
 * the claim can be checked. The figure is always the API's, or “—”.
 */
function PillarCard({
  title,
  value,
  hint,
  chip,
  note,
  href,
  cta,
}: {
  title: string;
  value: string;
  hint: string;
  chip?: { tone: Tone; label: string; title?: string } | undefined;
  note: string;
  href: string;
  cta: string;
}) {
  return (
    <div
      style={{
        background: brand.surface,
        border: `1px solid ${brand.line}`,
        borderRadius: 10,
        padding: space(4),
        display: "flex",
        flexDirection: "column",
        gap: space(2),
      }}
    >
      <div style={{ ...text.small, color: brand.muted, fontWeight: 700 }}>{title}</div>
      <div style={{ fontSize: 22, fontWeight: 700, color: brand.ink, lineHeight: 1.25, overflowWrap: "anywhere" }}>
        {value}
      </div>
      <div style={{ ...text.small, color: brand.muted }}>{hint}</div>
      {chip !== undefined && (
        <div>
          <Chip tone={chip.tone} {...(chip.title === undefined ? {} : { title: chip.title })}>
            {chip.label}
          </Chip>
        </div>
      )}
      {note !== "" && (
        // Clamped, not shortened: the API's own wording is what makes this
        // note trustworthy, so no sentence is rewritten here. Four cards whose
        // notes run from one line to twelve make a ragged row, so the overflow
        // is hidden and the full text stays on hover and on the linked view.
        <div
          title={note}
          style={{
            ...text.small,
            color: brand.ink,
            display: "-webkit-box",
            WebkitLineClamp: 5,
            WebkitBoxOrient: "vertical",
            overflow: "hidden",
          }}
        >
          {note}
        </div>
      )}
      <a href={href} style={{ ...text.small, ...linkStyle, marginTop: "auto", paddingTop: space(2) }}>
        {cta}
      </a>
    </div>
  );
}
