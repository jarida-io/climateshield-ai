// SPDX-License-Identifier: Apache-2.0
// Comfortaa, self-hosted via @fontsource (SIL Open Font License 1.1). Weights
// are imported explicitly so the bundle carries only what the page uses.
import "@fontsource/comfortaa/400.css";
import "@fontsource/comfortaa/700.css";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import App from "./App";

const rootEl = document.getElementById("root");
if (rootEl === null) {
  throw new Error("missing #root element");
}
createRoot(rootEl).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
