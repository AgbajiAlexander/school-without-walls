import { expect, test, type Page, type Response } from "@playwright/test";

const adminEmail = process.env.E2E_ADMIN_EMAIL;
const adminPassword = process.env.E2E_ADMIN_PASSWORD;
function getAdminCredentials() {
  if (!adminEmail || !adminPassword) {
    throw new Error("E2E_ADMIN_EMAIL and E2E_ADMIN_PASSWORD must identify the fictional CI test account");
  }
  return { adminEmail, adminPassword };
}

declare global {
  interface Window {
    __swwCspViolations?: string[];
  }
}

async function installCspViolationListener(page: Page) {
  const violations: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error" && /content security policy/i.test(message.text())) {
      violations.push(message.text());
    }
  });
  page.on("pageerror", (error) => violations.push(error.message));
  await page.addInitScript(() => {
    window.addEventListener("securitypolicyviolation", (event) => {
      window.__swwCspViolations ??= [];
      window.__swwCspViolations.push(event.violatedDirective);
    });
  });
  return violations;
}

async function assertDocumentNonce(response: Response | null): Promise<string> {
  if (!response) {
    throw new Error("Expected a document response");
  }
  const policy = response.headers()["content-security-policy"];
  expect(policy).toContain("script-src");
  const match = policy.match(/script-src[^;]*'nonce-([^']+)'/);
  expect(match).not.toBeNull();
  if (!match) {
    throw new Error("The document CSP does not contain a script nonce");
  }
  return match[1];
}

async function assertRenderedScriptsUseNonce(page: Page, nonce: string) {
  const scripts = await page.locator("script").evaluateAll((elements) =>
    elements.map((element) => {
      const script = element as HTMLScriptElement;
      return { nonce: script.nonce, type: script.type };
    }),
  );
  const executableScripts = scripts.filter(({ type }) =>
    !type || type === "module" || type === "text/javascript" || type === "application/javascript",
  );
  expect(executableScripts.length).toBeGreaterThan(0);
  expect(executableScripts.every(({ nonce: scriptNonce }) => scriptNonce === nonce)).toBe(true);
}

async function collectCspViolations(page: Page, violations: string[]) {
  violations.push(...await page.evaluate(() => {
    const current = window.__swwCspViolations ?? [];
    window.__swwCspViolations = [];
    return current;
  }));
}

test("unauthenticated dashboard access is redirected to login", async ({ page }) => {
  const response = await page.goto("/dashboard");

  expect(response?.status()).toBe(200);
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByRole("heading", { name: "Sign In" })).toBeVisible();
});

test("real backend rejects invalid credentials", async ({ page }) => {
  const credentials = getAdminCredentials();
  await page.goto("/login");
  await page.getByLabel("Email Address").fill(credentials.adminEmail);
  await page.getByLabel("Password", { exact: true }).fill(`${credentials.adminPassword}-incorrect`);
  await page.getByRole("button", { name: "Sign in" }).click();

  await expect(
    page.getByRole("alert").filter({ hasText: "Invalid email or password" }),
  ).toContainText("Invalid email or password");
  expect(await page.context().cookies("https://127.0.0.1:3443"))
    .not.toEqual(expect.arrayContaining([
      expect.objectContaining({ name: "__Host-sww_session" }),
    ]));
});

test("real login, CSP, secure cookies, CSRF, client navigation, and logout work over HTTPS", async ({ page, context }) => {
  const credentials = getAdminCredentials();
  const securityErrors = await installCspViolationListener(page);
  const cspViolations: string[] = [];
  const clientNavigationRequests: string[] = [];
  page.on("request", (request) => {
    if (request.resourceType() === "fetch" && new URL(request.url()).pathname === "/dashboard") {
      clientNavigationRequests.push(request.url());
    }
  });

  const landingResponse = await page.goto("/");
  const landingNonce = await assertDocumentNonce(landingResponse);
  await assertRenderedScriptsUseNonce(page, landingNonce);
  await collectCspViolations(page, cspViolations);
  await expect(page.getByRole("status")).toHaveText("API ready");

  const loginResponse = await page.goto("/login");
  const loginNonce = await assertDocumentNonce(loginResponse);
  expect(loginNonce).not.toBe(landingNonce);
  await assertRenderedScriptsUseNonce(page, loginNonce);
  await collectCspViolations(page, cspViolations);

  await page.getByLabel("Email Address").fill(credentials.adminEmail);
  await page.getByLabel("Password", { exact: true }).fill(credentials.adminPassword);
  await page.getByRole("button", { name: "Sign in" }).click();

  await expect(page).toHaveURL(/\/dashboard$/);
  await expect(page.getByRole("heading", { name: "Welcome back, Fictional!" })).toBeVisible();
  await expect(page.getByText(credentials.adminEmail, { exact: true })).toBeVisible();
  expect(clientNavigationRequests.length).toBeGreaterThan(0);
  await collectCspViolations(page, cspViolations);

  const cookies = await context.cookies("https://127.0.0.1:3443");
  const sessionCookie = cookies.find(({ name }) => name === "__Host-sww_session");
  expect(sessionCookie).toBeDefined();
  expect(sessionCookie?.httpOnly).toBe(true);
  expect(sessionCookie?.secure).toBe(true);
  expect(sessionCookie?.sameSite).toBe("Lax");
  expect(sessionCookie?.path).toBe("/");
  const sessionCookieIsJavaScriptVisible = await page.evaluate(() =>
    document.cookie.split(";").some((cookie) => cookie.trim().startsWith("__Host-sww_session=")),
  );
  expect(sessionCookieIsJavaScriptVisible).toBe(false);

  const rejectedWithoutCSRF = await page.evaluate(async () => {
    const response = await fetch("/api/v1/auth/logout", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
    return response.status;
  });
  expect(rejectedWithoutCSRF).toBe(403);

  await page.getByRole("button", { name: "Sign Out" }).click();
  await expect(page).toHaveURL(/\/login$/);

  if (!sessionCookie) {
    throw new Error("Expected the real login flow to issue a session cookie");
  }
  // Replay only the cookie issued by this login to verify the server revoked that session.
  const replayedSession = await context.request.get("https://127.0.0.1:3443/api/v1/auth/me", {
    headers: { Cookie: `${sessionCookie.name}=${sessionCookie.value}` },
  });
  expect(replayedSession.status()).toBe(401);

  const remainingSessionCookie = (await context.cookies("https://127.0.0.1:3443"))
    .find(({ name }) => name === "__Host-sww_session");
  expect(remainingSessionCookie).toBeUndefined();
  await page.goto("/dashboard");
  await expect(page).toHaveURL(/\/login$/);
  await collectCspViolations(page, cspViolations);

  expect(cspViolations).toEqual([]);
  expect(securityErrors).toEqual([]);
});
