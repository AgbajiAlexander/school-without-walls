import { resolve } from "node:path";
import { defineConfig } from "@playwright/test";

const webOrigin = "https://127.0.0.1:3443";

function hasDisposableE2EDatabase() {
  const databaseUrl = process.env.DATABASE_URL;
  if (!databaseUrl || databaseUrl !== process.env.SWW_TEST_DATABASE_URL) {
    return false;
  }

  try {
    const database = new URL(databaseUrl);
    return (database.protocol === "postgres:" || database.protocol === "postgresql:")
      && ["127.0.0.1", "localhost"].includes(database.hostname)
      && database.pathname === "/sww_e2e_test";
  } catch {
    return false;
  }
}

if (!process.argv.includes("--list") && !hasDisposableE2EDatabase()) {
  throw new Error("Browser E2E requires matching DATABASE_URL and SWW_TEST_DATABASE_URL for loopback sww_e2e_test");
}

export default defineConfig({
  testDir: "./e2e",
  testMatch: "**/*.spec.ts",
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: "list",
  use: {
    baseURL: webOrigin,
    ignoreHTTPSErrors: true,
    trace: "off",
    screenshot: "off",
    video: "off",
  },
  webServer: [
    {
      command: "go run ./cmd/server",
      cwd: resolve(process.cwd(), "../../services/api"),
      url: "http://127.0.0.1:8080/ready",
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        DATABASE_URL: process.env.DATABASE_URL ?? "",
        SERVER_ENV: "staging",
        CORS_ALLOWED_ORIGINS: webOrigin,
        PUBLIC_WEB_URL: webOrigin,
      },
    },
    {
      command: "npm run start -- --hostname 127.0.0.1 --port 3000",
      cwd: process.cwd(),
      url: "http://127.0.0.1:3000/api/health",
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        API_INTERNAL_URL: "http://127.0.0.1:8080",
      },
    },
    {
      command: "node e2e/https-proxy.mjs",
      cwd: process.cwd(),
      port: 3443,
      reuseExistingServer: false,
      timeout: 30_000,
      env: {
        E2E_TLS_CERT: process.env.E2E_TLS_CERT ?? "",
        E2E_TLS_KEY: process.env.E2E_TLS_KEY ?? "",
      },
    },
  ],
});
