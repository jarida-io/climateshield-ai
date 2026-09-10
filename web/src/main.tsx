// SPDX-License-Identifier: Apache-2.0
// Typefaces, self-hosted via @fontsource (all SIL Open Font License 1.1), so
// the dashboard makes no third-party request and a public visitor is never
// disclosed to a font CDN. Weights are imported one by one: the bundle carries
// only what the page actually sets.
//
// The sans is the VARIABLE cut: one file covers every weight the page sets,
// where the static package shipped 288 separate faces. That is a smaller
// download for a dashboard someone may open on a county office connection,
// and it stops `npm ci` falling over on the file count during the image build.
//
// IBM Plex Sans is the interface and display face. It was drawn for technical
// and institutional work, and it has true tabular figures — which is what a
// page comparing five counties' numbers needs. IBM Plex Mono carries the
// things that are literally machine output: Merkle roots, endpoint paths,
// driver values.
//
// Comfortaa is kept for the ClimateShield wordmark and nothing else. It is a
// rounded display face: right for a logotype, wrong for a 13px table cell,
// where its wide letterforms and low x-height cost real legibility.
import "@fontsource/comfortaa/700.css";
import "@fontsource-variable/ibm-plex-sans";
import "@fontsource/ibm-plex-mono/400.css";
import "@fontsource/ibm-plex-mono/500.css";
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
