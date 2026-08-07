// SPDX-License-Identifier: Apache-2.0

import { publicClient } from "../api";
import { Caveat, Card, Code, Failed, Loading, Page, StatTile, Table, Td, TileRow, brand, space, text } from "../ui";
import { useApi } from "../useApi";

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

  return (
    <Page
      title="Guardian messaging"
      lede="What the alert path produced, in both languages, and exactly what happened to it."
    >
      <Caveat>
        {a.channelSends ? (
          <>
            <strong>Channel “{a.channel}” is active and delivers messages.</strong> {a.channelNote}
          </>
        ) : (
          <>
            <strong>No SMS was sent.</strong> {a.channelNote} Messages are rendered, checked and
            recorded with status <code>would_send</code> — never <code>sent</code>, which only a
            real carrier adapter may write. Message bodies are not published here: they contain a
            child’s first name. The samples below are the same templates rendered with placeholder
            values.
          </>
        )}
      </Caveat>

      <TileRow>
        <StatTile label="Active channel" value={a.channel} hint={a.channelSends ? "delivers" : "sends nothing"} />
        <StatTile label="Alerts recorded" value={String(total)} />
        <StatTile label="Languages" value={String(a.templates.length)} hint="English and Kiswahili" />
      </TileRow>

      <Card title="Outcomes">
        <Table head={["Status", "Count", "Meaning"]}>
          {a.statuses.map((s) => (
            <tr key={s.status}>
              <Td mono>{s.status}</Td>
              <Td mono>{s.suppressed ? "withheld (k<10)" : String(s.count ?? 0n)}</Td>
              <Td>{statusMeaning(s.status)}</Td>
            </tr>
          ))}
        </Table>
      </Card>

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
