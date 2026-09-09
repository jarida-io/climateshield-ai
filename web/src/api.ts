// SPDX-License-Identifier: Apache-2.0

// Typed client for the public API. Every response type comes from the
// protobuf codegen in src/gen — no hand-written response interfaces.
import { createClient, type Interceptor } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

import { PublicService } from "./gen/climateshield/v1/public_pb";

/**
 * The public API never fails a read: when the database is unreachable it
 * serves the last good response and marks it `X-Data-Stale: true`. That is a
 * good property, and it is also exactly the kind of thing a dashboard can
 * quietly hide — a page full of confident numbers that are hours old looks
 * identical to a page of live ones.
 *
 * So the flag is tracked per method here and surfaced once, under the nav, on
 * whatever view the reader happens to be on. A method drops out of the set as
 * soon as it answers fresh again, so the notice disappears by itself rather
 * than by being dismissed.
 */
const staleMethods = new Set<string>();
const listeners = new Set<() => void>();

function publish(): void {
  for (const l of listeners) l();
}

export function subscribeToStaleData(onChange: () => void): () => void {
  listeners.add(onChange);
  return () => {
    listeners.delete(onChange);
  };
}

/** How many endpoints on this page are currently answering from cache. */
export function staleEndpointCount(): number {
  return staleMethods.size;
}

const staleFlag: Interceptor = (next) => async (req) => {
  const res = await next(req);
  const name = `${req.service.typeName}.${req.method.name}`;
  const before = staleMethods.size;
  if (res.header.get("X-Data-Stale") === "true") {
    staleMethods.add(name);
  } else {
    staleMethods.delete(name);
  }
  if (staleMethods.size !== before) publish();
  return res;
};

const transport = createConnectTransport({ baseUrl: "/", interceptors: [staleFlag] });

export const publicClient = createClient(PublicService, transport);
