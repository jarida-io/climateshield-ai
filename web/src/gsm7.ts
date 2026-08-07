// SPDX-License-Identifier: Apache-2.0

// GSM 03.38 septet counting, for live feedback in the message previewer.
//
// This mirrors internal/notify/gsm7.go. The authority is the Go
// implementation — it is what actually refuses to render an over-long or
// non-encodable message before anything reaches a channel. This copy exists
// only so the previewer can respond as you type without sending your input to
// the server. The character tables are the published GSM 03.38 standard, not
// business logic, so the duplication is a transcription of a spec rather than
// a second source of truth. gsm7.test.ts pins the two against each other.

const BASIC =
  "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?" +
  "¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà";

// Characters that cost two septets: an escape, then the character.
const EXTENDED = "\f^{}\\[]~|€";

const basic = new Set(Array.from(BASIC));
const extended = new Set(Array.from(EXTENDED));

export interface SeptetCount {
  /** False when the text contains a character GSM-7 cannot represent. */
  ok: boolean;
  septets: number;
  /** How many characters cost two septets rather than one. */
  extended: number;
  /** The first character that cannot be encoded, when ok is false. */
  offending: string;
}

/** Counts GSM-7 septets, or reports the first character that cannot be encoded. */
export function septets(s: string): SeptetCount {
  let total = 0;
  let ext = 0;
  for (const ch of s) {
    if (basic.has(ch)) {
      total += 1;
    } else if (extended.has(ch)) {
      total += 2;
      ext += 1;
    } else {
      return { ok: false, septets: 0, extended: 0, offending: ch };
    }
  }
  return { ok: true, septets: total, extended: ext, offending: "" };
}
