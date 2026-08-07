// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from "react";

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

/** Formats a protobuf Timestamp for display, or a dash when absent. */
export function ts(t: { seconds: bigint } | undefined): string {
  if (t === undefined) return "—";
  return new Date(Number(t.seconds) * 1000).toLocaleString();
}
