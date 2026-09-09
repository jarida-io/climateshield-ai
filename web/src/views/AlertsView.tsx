// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";

import { publicClient } from "../api";
import { Columns, TableView } from "../charts";
import { Select, TextInput } from "../forms";
import { septets } from "../gsm7";
import { Caveat, Card, Code, Disclosure, Failed, Loading, Page, StatTile, Table, Td, TileRow, brand, levelColor, space, text } from "../ui";
import { useApi } from "../useApi";

const COUNTIES = ["Nairobi", "Kisumu", "Mombasa", "Nakuru", "Eldoret"];

// The whole KEPI schedule seeded by migration 0006, longest names included —
// the previewer is only useful for stress-testing the GSM-7 budget if it can
// offer the doses that actually exist. No endpoint publishes the schedule
// (the registry is an internal service), so this list is a copy: if a dose is
// added there, add it here too.
const VACCINES = [
  "BCG", "OPV birth dose", "OPV 1", "OPV 2", "OPV 3",
  "DPT-HepB-Hib 1", "DPT-HepB-Hib 2", "DPT-HepB-Hib 3",
  "PCV 1", "PCV 2", "PCV 3", "Rotavirus 1", "Rotavirus 2",
  "IPV", "Measles-Rubella 1", "Measles-Rubella 2",
];

/**
 * Proves: the messaging path is complete and bilingual — and states plainly
 * that nothing was sent, because the mock channel is active.
 */
export function AlertsView() {
  const alerts = useApi(() => publicClient.getAlertSummary({}), []);

  if (alerts.kind === "loading") return <Loading what="alert summary" />;
  if (alerts.kind === "error") return <Failed what="alert summary" error={alerts.message} />;
  const a = alerts.data;

  const total = a.statuses.reduce((n, s) => n + Number(s.count ?? 0n), 0);

  const budget = a.templates[0]?.maxSeptets;

  return (
    <Page
      title="Guardian messaging"
      lede="One short message, in the guardian’s language, naming their own child and the dose that is due. The alert path renders it, gates it on consent and quiet hours, and records what it did."
    >
      {a.channelSends && (
        <Caveat>
          <strong>Channel “{a.channel}” is active and delivers messages.</strong> {a.channelNote}{" "}
          Messages leaving this deployment reach real handsets.
        </Caveat>
      )}

      <TileRow>
        <StatTile
          label="Active channel"
          value={a.channel}
          hint={a.channelSends ? "delivers messages" : "sends nothing"}
        />
        <StatTile label="Alerts recorded" value={String(total)} />
        <StatTile
          label="One-segment budget"
          value={budget === undefined ? "—" : `${budget} septets`}
          hint="a longer message costs two and can fail on basic handsets"
        />
      </TileRow>

      <Disclosure>
        {a.channelSends ? (
          <>
            <strong>Every message is rendered, gated and recorded here</strong>, and the active
            channel “{a.channel}” then delivers it. {a.channelNote}
          </>
        ) : (
          <>
            <strong>Everything up to the carrier is real: rendering, the consent gate, quiet
            hours, deduplication and the recorded outcome.</strong> The last step is not.{" "}
            {a.channelNote} Messages are recorded with status <code>would_send</code> — never{" "}
            <code>sent</code>, which only a real carrier adapter may write, so no screen and no log
            can imply a delivery that did not happen.
          </>
        )}{" "}
        Message bodies as sent are not published here: they contain a child’s first name. The
        samples below are the same templates rendered with placeholder values.
      </Disclosure>

      <Card title="Outcomes">
        {a.statuses.length === 0 ? (
          <p style={{ ...text.body, color: brand.muted, margin: 0 }}>
            No alert has been recorded yet in this deployment. Outcomes appear here as soon as a
            county is scored at an elevated level and the dispatch job runs.
          </p>
        ) : (
          <Table head={["Status", "Count", "Meaning"]}>
            {a.statuses.map((s) => (
              <tr key={s.status}>
                <Td mono>{s.status}</Td>
                <Td mono>{s.suppressed ? "withheld (k<10)" : String(s.count ?? 0n)}</Td>
                <Td>{statusMeaning(s.status)}</Td>
              </tr>
            ))}
          </Table>
        )}
      </Card>

      <Card title="Message length by outcome">
        <p style={{ ...text.body, color: brand.muted, marginTop: 0 }}>
          Every alert must fit a single GSM-7 segment. Anything longer costs two
          messages and can fail on basic handsets.
        </p>
        <Columns
          data={a.templates.map((t) => ({
            label: t.lang === "sw" ? "Kiswahili" : "English",
            value: t.septets,
            color: t.septets <= t.maxSeptets ? levelColor["LOW"] : levelColor["HIGH"],
            detail: `${t.lang === "sw" ? "Kiswahili" : "English"}: ${t.septets} of ${t.maxSeptets} septets`,
          }))}
          unit=" septets"
          height={140}
        />
        <TableView
          head={["Language", "Septets", "Budget", "Fits one segment"]}
          rows={a.templates.map((t) => [
            t.lang === "sw" ? "Kiswahili" : "English",
            t.septets, t.maxSeptets, t.septets <= t.maxSeptets ? "yes" : "no",
          ])}
        />
      </Card>

      <MessagePreview />

      <Card title="Message templates">
        <p style={{ ...text.body, color: brand.muted, marginTop: 0 }}>
          Rendered with placeholders. Each must fit one GSM-7 segment so it costs one message and
          arrives on any handset — including basic phones on low-bandwidth networks. No disease
          name and no identity number ever appears in a message body; a test enforces both.
        </p>
        {a.templates.map((t) => (
          <div key={t.lang} style={{ marginBottom: space(4) }}>
            <div style={{ ...text.small, color: brand.muted, marginBottom: space(2) }}>
              <strong>{t.lang === "sw" ? "Kiswahili" : "English"}</strong> · {t.septets}/
              {t.maxSeptets} septets
              <span
                style={{
                  color: t.septets <= t.maxSeptets ? brand.low : brand.high,
                  fontWeight: 700,
                  marginLeft: space(2),
                }}
              >
                {t.septets <= t.maxSeptets ? "fits one segment" : "TOO LONG"}
              </span>
            </div>
            <Code>{t.body}</Code>
          </div>
        ))}
      </Card>

      <Card title="Sending rules">
        <ul style={{ ...text.body, color: brand.ink, margin: 0, paddingLeft: space(5), lineHeight: 1.7 }}>
          <li>
            <strong>Quiet hours:</strong> {a.quietHours}.
          </li>
          <li>
            <strong>Consent:</strong> the most recent entry in an append-only consent log decides.
            A guardian who replied STOP is skipped, and the skip is recorded rather than hidden.
          </li>
          <li>
            <strong>No duplicates:</strong> one alert per child per risk score, so a retried job
            cannot message a family twice.
          </li>
        </ul>
      </Card>
    </Page>
  );
}

/**
 * Live template previewer. Everything here runs in the browser: the values
 * you type are never sent to the server, never logged and never stored. It
 * exists so a reviewer can push the templates to their limits — a long name,
 * a long vaccine, the longest county — and watch the GSM-7 budget respond.
 */
function MessagePreview() {
  const [lang, setLang] = useState("en");
  const [level, setLevel] = useState("HIGH");
  const [county, setCounty] = useState("Kisumu");
  const [vaccine, setVaccine] = useState("Measles-Rubella 1");
  const [name, setName] = useState("Amina");

  const body =
    lang === "sw"
      ? `ClimateShield: Hatari ya mlipuko ni ${level} katika ${county}. ${name} anahitaji chanjo ya ${vaccine}. Tembelea kliniki. Jibu STOP kujiondoa.`
      : `ClimateShield: Outbreak risk is ${level} in ${county}. ${name} is due for ${vaccine}. Visit your nearest clinic. Reply STOP to opt out.`;

  const count = septets(body);
  const over = count.ok && count.septets > 160;

  return (
    <Card title="Preview a message">
      <p style={{ ...text.body, color: brand.muted, marginTop: 0 }}>
        Rendered in your browser only — nothing typed here is transmitted or stored.
      </p>
      <form
        onSubmit={(e) => e.preventDefault()}
        style={{ display: "flex", gap: space(4), flexWrap: "wrap", marginBottom: space(4) }}
      >
        <Select
          label="Language" value={lang} onChange={setLang}
          options={[{ value: "en", label: "English" }, { value: "sw", label: "Kiswahili" }]}
        />
        <Select
          label="Risk level" value={level} onChange={setLevel}
          options={["HIGH", "MEDIUM"].map((v) => ({ value: v, label: v }))}
        />
        <Select
          label="County" value={county} onChange={setCounty}
          options={COUNTIES.map((v) => ({ value: v, label: v }))}
        />
        <Select
          label="Vaccine due" value={vaccine} onChange={setVaccine}
          options={VACCINES.map((v) => ({ value: v, label: v }))}
        />
        <TextInput
          label="Child first name" value={name} onChange={setName} maxLength={24}
          hint="first name only — never a surname"
        />
      </form>

      <Code>{body}</Code>

      <div style={{ ...text.small, marginTop: space(3), display: "flex", gap: space(4), flexWrap: "wrap" }}>
        {count.ok ? (
          <>
            <span style={{ color: over ? brand.high : brand.low, fontWeight: 700 }}>
              {count.septets} / 160 septets — {over ? "TOO LONG, would split into 2 messages" : "fits one segment"}
            </span>
            <span style={{ color: brand.muted }}>
              {count.extended > 0 && `${count.extended} character(s) cost 2 septets each`}
            </span>
          </>
        ) : (
          <span style={{ color: brand.high, fontWeight: 700 }}>
            “{count.offending}” cannot be encoded in GSM-7 — this message would be
            rejected at render time rather than sent as a costly multi-part message.
          </span>
        )}
      </div>
    </Card>
  );
}

function statusMeaning(status: string): string {
  switch (status) {
    case "would_send":
      return "Rendered and recorded by the mock channel. Nothing was transmitted.";
    case "sent":
      return "Handed to a live carrier adapter.";
    case "skipped_consent":
      return "Guardian has opted out; no message was prepared.";
    case "skipped_quiet_hours":
      return "Fell inside quiet hours and was deferred.";
    case "failed":
      return "The channel returned an error.";
    default:
      return status;
  }
}
