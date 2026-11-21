import { defineConfig } from "@playwright/test";
import base from "./playwright.config";

export default defineConfig({
  ...base,
  testMatch: "academics.acceptance.ts",
  workers: 1,
  use: { ...base.use, channel: "msedge" },
});