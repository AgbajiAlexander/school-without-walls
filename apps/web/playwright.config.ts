import { resolve } from "node:path";
import { defineConfig } from "@playwright/test";

function testPort(name: string, fallback: number) {
  const value = Number(process.env[name] ?? fallback);
  if (!Number.isInteger(value) || value < 1 || value > 65535) {
    throw new Error(`Invalid browser fixture port: ${name}`);
  }
  return value;
}
const proxyPort = testPort("E2E_HTTPS_PORT", 3443);
const webPort = testPort("E2E_WEB_PORT", 3000);
const apiPort = testPort("E2E_API_PORT", 8080);
const webOrigin = `https://127.0.0.1:${proxyPort}`;

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
      url: `http://127.0.0.1:${apiPort}/ready`,
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        DATABASE_URL: process.env.DATABASE_URL ?? "",
        SERVER_ENV: "staging",
        SERVER_PORT: String(apiPort),
        CORS_ALLOWED_ORIGINS: webOrigin,
        PUBLIC_WEB_URL: webOrigin,
      },
    },
    {
      command: `npm run start -- --hostname 127.0.0.1 --port ${webPort}`,
      cwd: process.cwd(),
      url: `http://127.0.0.1:${webPort}/api/health`,
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        API_INTERNAL_URL: `http://127.0.0.1:${apiPort}`,
      },
    },
    {
      command: "node e2e/https-proxy.mjs",
      cwd: process.cwd(),
      port: proxyPort,
      reuseExistingServer: false,
      timeout: 30_000,
      env: {
        E2E_TLS_CERT: process.env.E2E_TLS_CERT ?? "",
        E2E_TLS_KEY: process.env.E2E_TLS_KEY ?? "",
      },
    },
  ],
});
