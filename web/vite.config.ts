// SPDX-License-Identifier: Apache-2.0
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Dev-server proxy: the dashboard always talks same-origin; in production
// nginx proxies these paths to the publicapi service.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/v1": "http://localhost:8080",
      "/climateshield.v1.PublicService": "http://localhost:8080",
    },
  },
});
