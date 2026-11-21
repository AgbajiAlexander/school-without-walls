import { expect, test, type Page } from "@playwright/test";

const school = "academic-fixture-a";
const otherSchool = "academic-fixture-b";
const academicPath = `/schools/${school}/academics`;
const endpoint = (slug: string) => `/api/v1/tenants/${slug}/academic-sessions`;

async function signIn(page: Page, email: string) {
  const password = process.env.E2E_ADMIN_PASSWORD;
  if (!password) throw new Error("Fictional fixture password is required");
  await page.goto("/login");
  await page.getByLabel("Email Address").fill(email);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/dashboard$/);
}

async function attemptCreate(page: Page, slug: string, includeCSRF: boolean) {
  return page.evaluate(async ({ url, includeCSRF }) => {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (includeCSRF) {
      const identity = await fetch("/api/v1/auth/me");
      if (!identity.ok) throw new Error("Fixture session is unavailable");
      const { data } = await identity.json();
      if (!data?.csrf_token) throw new Error("Fixture CSRF token is unavailable");
      headers["X-CSRF-Token"] = data.csrf_token;
    }
    const response = await fetch(url, {
      method: "POST",
      headers,
      body: JSON.stringify({ name: "Denied fixture", starts_on: "2031-09-01", ends_on: "2032-07-31" }),
    });
    const payload = await response.json();
    return { status: response.status, message: payload.error?.message };
  }, { url: endpoint(slug), includeCSRF });
}

test("academic session persists and Go rejects missing CSRF, foreign schools and teacher writes", async ({ page, browser, baseURL }) => {
  await signIn(page, "academic-owner@example.test");
  await page.goto(academicPath);
  await expect(page.getByRole("heading", { name: "Academic Management" })).toBeVisible();
  expect(await attemptCreate(page, school, false)).toEqual({ status: 403, message: "Invalid or missing CSRF token" });

  await page.getByLabel("Session name").fill("2030/2031 fixture");
  await page.getByLabel("Start date").fill("2030-09-01");
  await page.getByLabel("End date").fill("2031-07-31");
  const created = page.waitForResponse((response) =>
    new URL(response.url()).pathname === endpoint(school) && response.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Create draft session" }).click();
  expect((await created).status()).toBe(201);
  await page.reload();
  await expect(page.getByText("2030/2031 fixture", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Denied fixture", { exact: true })).toHaveCount(0);

  await page.getByRole("button", { name: /2030\/2031 fixture/ }).click();
  const editionLink = page.getByRole("link", { name: "Curriculum editions" });
  const editionPath = await editionLink.getAttribute("href");
  if (!editionPath) throw new Error("Missing curriculum link for the selected session");
  await editionLink.click();
  await expect(page.getByRole("heading", { name: "Session curriculum editions" })).toBeVisible();
  await expect(page.getByLabel("Framework edition").locator("option")).toHaveCount(2);
  await page.getByLabel("Framework edition").selectOption({ label: "Fictional Nigerian Framework \u00b7 Example 1 \u00b7 Fictional Publisher" });
  await page.getByRole("button", { name: "Save curriculum edition" }).click();
  await expect(page.getByText("Curriculum edition saved.")).toBeVisible();
  await page.reload();
  await expect(page.getByText(/Primary curriculum.*Fictional Nigerian Framework/)).toBeVisible();
  await page.getByLabel("Curriculum role").selectOption("ENRICHMENT");
  await page.getByRole("button", { name: "Show eligible editions" }).click();
  await page.getByLabel("Framework edition").selectOption({ label: "Fictional British Framework \u00b7 Example 1 \u00b7 Fictional Publisher" });
  await page.getByRole("button", { name: "Save curriculum edition" }).click();
  await expect(page.getByText(/Optional enrichment.*Fictional British Framework/)).toBeVisible();
  await page.goto(editionPath.replace(school, otherSchool));
  await expect(page.getByRole("status")).toHaveText("Curriculum selection is unavailable for this school session.");

  expect(await attemptCreate(page, otherSchool, true)).toEqual({ status: 404, message: "Resource not found" });
  await page.goto(`/schools/${otherSchool}/academics`);
  await expect(page.getByRole("heading", { name: "School access unavailable" })).toBeVisible();

  const teacherContext = await browser.newContext({ baseURL, ignoreHTTPSErrors: true });
  try {
    const teacher = await teacherContext.newPage();
    await signIn(teacher, "academic-teacher@example.test");
    await teacher.goto(academicPath);
    await expect(teacher.getByText("2030/2031 fixture", { exact: true }).first()).toBeVisible();
    await expect(teacher.getByRole("button", { name: "Create draft session" })).toHaveCount(0);
    await expect(teacher.getByRole("link", { name: "Curriculum editions" })).toHaveCount(0);
    await teacher.goto(editionPath);
    await expect(teacher.getByRole("status")).toHaveText("Curriculum selection is unavailable for this school session.");
    await teacher.goto(academicPath);
    expect(await attemptCreate(teacher, school, true)).toEqual({ status: 403, message: "Access denied" });
    await teacher.reload();
    await expect(teacher.getByText("2030/2031 fixture", { exact: true }).first()).toBeVisible();
    await expect(teacher.getByText("Denied fixture", { exact: true })).toHaveCount(0);
  } finally {
    await teacherContext.close();
  }
});
