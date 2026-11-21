import { defineConfig } from "@playwright/test";
import base from "./playwright.config";

export default defineConfig({
  ...base,
  testMatch: ["academics.acceptance.ts", "staff-profiles.acceptance.ts", "students.acceptance.ts", "guardians.acceptance.ts", "enrollment.acceptance.ts"],
  workers: 1,
  use: { ...base.use, channel: "msedge" },
});