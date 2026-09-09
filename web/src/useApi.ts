// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState, useSyncExternalStore } from "react";

import { staleEndpointCount, subscribeToStaleData } from "./api";

export type Load<T> =
  | { kind: "loading" }
  | { kind: "ready"; data: T }
  | { kind: "error"; message: string };

/** Calls a generated Connect method once on mount. */
export function useApi<T>(fetcher: () => Promise<T>, deps: unknown[] = []): Load<T> {
  const [state, setState] = useState<Load<T>>({ kind: "loading" });
  useEffect(() => {
    let cancelled = false;
    setState({ kind: "loading" });
    fetcher()
      .then((data) => {
        if (!cancelled) setState({ kind: "ready", data });
      })
      .catch((err: unknown) => {
        if (!cancelled) setState({ kind: "error", message: String(err) });
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  return state;
}

/**
 * The response if it arrived, otherwise undefined.
 *
 * Views that combine several endpoints use this so one unreachable call cannot
 * blank the whole page: the sections that loaded render, and the ones that did
 * not say “—” rather than a zero that would read as a real measurement.
 */
export function dataOf<T>(load: Load<T>): T | undefined {
  return load.kind === "ready" ? load.data : undefined;
}

// Hoisted so the store is not resubscribed on every render.
const anyStale = () => staleEndpointCount() > 0;
const neverStaleOnServer = () => false;

/** True while any endpoint on this page is answering from the stale cache. */
export function useStaleData(): boolean {
  return useSyncExternalStore(subscribeToStaleData, anyStale, neverStaleOnServer);
}

/** Sets the browser tab title, so an open tab says which view it is. */
export function useTitle(title: string): void {
  useEffect(() => {
    document.title = title;
  }, [title]);
}

/** Formats a protobuf Timestamp for display, or a dash when absent. */
export function ts(t: { seconds: bigint } | undefined): string {
  if (t === undefined) return "—";
  return new Date(Number(t.seconds) * 1000).toLocaleString();
}

/** A short date-time for headline sentences, or a dash when absent. */
export function tsShort(t: { seconds: bigint } | undefined): string {
  if (t === undefined) return "—";
  return new Date(Number(t.seconds) * 1000).toLocaleString(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}
