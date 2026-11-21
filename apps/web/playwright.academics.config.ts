import { defineConfig } from "@playwright/test";
import base from "./playwright.config";

const mediaEnabled = process.env.SWW_E2E_LESSON_MEDIA === "1";

export default defineConfig({
  ...base,
  testMatch: ["academics.acceptance.ts", "staff-profiles.acceptance.ts", "students.acceptance.ts", "guardians.acceptance.ts", "enrollment.acceptance.ts", "curriculum.acceptance.ts", "learning.acceptance.ts"],
  webServer: [
    ...(mediaEnabled ? [{
      command: "node e2e/lesson-media-fixture.mjs",
      url: "http://127.0.0.2:18086/health",
      reuseExistingServer: false,
      timeout: 15_000,
    }] : []),
    ...(Array.isArray(base.webServer) ? base.webServer : [base.webServer!]),
  ],
  workers: 1,
  use: { ...base.use, channel: "msedge" },
});