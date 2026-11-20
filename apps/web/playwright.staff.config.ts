import { randomBytes } from "node:crypto";
import { resolve } from "node:path";
import { defineConfig } from "@playwright/test";

const listing = process.argv.includes("--list");
const webDir = process.env.SWW_B12_WEB_DIR;
const database = process.env.SWW_B12_DATABASE_URL;
if (!listing && (!webDir || resolve(webDir) === process.cwd() ||
    database !== "postgres://sww_browser@127.0.0.1:15432/sww_e2e_test?sslmode=disable")) {
  throw new Error("Staff browser verification requires a separate web copy and the disposable loopback database");
}

const password = process.env.SWW_B12_ADMIN_PASSWORD || randomBytes(24).toString("base64url");
process.env.SWW_B12_ADMIN_PASSWORD = password;

export default defineConfig({
  testDir: "./e2e",
  testMatch: "staff-invitation.browser.ts",
  workers: 1,
  timeout: 120_000,
  reporter: "list",
  outputDir: webDir ? resolve(webDir, "browser-results") : undefined,
  preserveOutput: "never",
  use: {
    baseURL: "http://127.0.0.1:13000",
    channel: process.env.SWW_B12_BROWSER_CHANNEL || undefined,
    trace: "off",
    screenshot: "off",
    video: "off",
  },
  webServer: listing ? [] : [
    {
      command: "go test ./internal/platform/http -count=1 -timeout=15m -run ^TestStaffBrowserServer$",
      cwd: resolve(process.cwd(), "../../services/api"),
      url: "http://127.0.0.1:18080/ready",
      reuseExistingServer: false,
      timeout: 120_000,
      stdout: "ignore",
      stderr: "ignore",
      env: { SWW_B12_DATABASE_URL: database!, SWW_B12_ADMIN_PASSWORD: password },
    },
    {
      command: "node node_modules/next/dist/bin/next start --hostname 127.0.0.1 --port 13000",
      cwd: webDir,
      url: "http://127.0.0.1:13000/api/health",
      reuseExistingServer: false,
      timeout: 120_000,
      stdout: "ignore",
      stderr: "ignore",
      env: { API_INTERNAL_URL: "http://127.0.0.1:18080" },
    },
  ],
});
