// SPDX-License-Identifier: Apache-2.0

// Typed client for the public API. Every response type comes from the
// protobuf codegen in src/gen — no hand-written response interfaces.
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

import { PublicService } from "./gen/climateshield/v1/public_pb";

const transport = createConnectTransport({ baseUrl: "/" });

export const publicClient = createClient(PublicService, transport);
