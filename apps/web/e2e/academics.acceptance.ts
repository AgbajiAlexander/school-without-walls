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
  test.setTimeout(60_000);
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
    await verifySchoolMode(page, teacher);
    await page.goto(academicPath);
    await page.getByRole("button", { name: /Enrollment fixture session/ }).click();
    await page.getByRole("listitem").filter({ hasText: "Fixture First Term" }).getByRole("link", { name: "Assessment schemes" }).click();
    await expect(page.getByRole("heading", { name: "Assessment schemes", exact: true })).toBeVisible();
    await page.getByLabel("Component name").fill("Fictional coursework");
    await page.getByLabel("Maximum score").fill("20");
    await page.getByLabel("Weight (%)").fill("30.25");
    await page.getByRole("button", { name: "Save assessment draft" }).click();
    await expect(page.getByRole("button", { name: "Publish scheme" })).toBeDisabled();
    await page.getByRole("button", { name: "Edit assessment draft", exact: true }).click();
    await page.getByRole("button", { name: "Add component" }).click();
    const second = page.getByRole("group", { name: "Component 2" });
    await second.getByLabel("Component name").fill("Fictional examination");
    await second.getByLabel("Maximum score").fill("50");
    await second.getByLabel("Weight (%)").fill("69.75");
    await page.getByRole("button", { name: "Add grade band" }).click();
    const upper = page.getByRole("group", { name: "Grade band 1", exact: true });
    await upper.getByLabel("Grade label").fill("A");
    await upper.getByLabel("Minimum (%)").fill("70");
    await upper.getByLabel("Maximum (%)").fill("100");
    await page.getByRole("button", { name: "Add grade band" }).click();
    const lower = page.getByRole("group", { name: "Grade band 2", exact: true });
    await lower.getByLabel("Grade label").fill("B");
    await lower.getByLabel("Minimum (%)").fill("0");
    await lower.getByLabel("Maximum (%)").fill("70");
    await page.getByRole("button", { name: "Save assessment draft" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "Bands cannot overlap" })).toBeVisible();
    await lower.getByLabel("Maximum (%)").fill("69.99");
    await page.getByRole("button", { name: "Save assessment draft" }).click();
    await page.getByRole("button", { name: "Publish scheme" }).click();
    await page.getByRole("button", { name: "Confirm publication" }).click();
    await expect(page.getByText("Assessment scheme published.")).toBeVisible();
    await page.reload();
    await expect(page.getByText(/Published.*100\.00%/)).toBeVisible();
    await expect(page.getByText("B: 0.00% to 69.99%")).toBeVisible();
    await expect(page.getByRole("button", { name: "Edit assessment draft", exact: true })).toHaveCount(0);
    await teacher.goto(page.url());
    await expect(teacher.getByText("Assessment schemes are unavailable for this term or account.")).toBeVisible();
  } finally {
    await teacherContext.close();
  }
});


async function verifySchoolMode(page: Page, teacher: Page) {
  const settings = "/schools/" + school + "/settings";

  await page.goto(settings);
  await expect(page.getByRole("heading", { name: "School operating mode" })).toBeVisible();
  const start = new Date(Date.now() - 3600000).toISOString().slice(0, 16);
  const end = new Date(Date.now() + 3600000).toISOString().slice(0, 16);
  await page.getByRole("combobox", { name: "Operating mode", exact: true }).selectOption("REMOTE");
  await page.getByLabel("Starts (UTC)").fill(start);
  await page.getByLabel("Ends (UTC)").fill(end);
  await page.getByLabel("Reason", { exact: true }).fill("Fictional building maintenance");
  await page.getByLabel("Where to get help").fill("Contact the fictional office");
  await page.getByLabel("Schedule link").fill("https://schedule.example.test/fictional");
  await page.getByRole("button", { name: "Save operating mode" }).click();
  await expect(page.getByText("Current mode: Remote")).toBeVisible();

  await teacher.goto("/dashboard");
  const schoolCard = teacher.getByRole("article").filter({ hasText: "Fictional Academic School A" });
  await expect(schoolCard.getByRole("heading", { name: "Remote learning" })).toBeVisible();
  await expect(schoolCard.getByText("Fictional building maintenance")).toBeVisible();
  await expect(schoolCard.getByRole("link", { name: "View school schedule" })).toHaveAttribute("href", "https://schedule.example.test/fictional");
  await expect(teacher.getByRole("article").filter({ hasText: "Fictional Classroom School" })
    .getByRole("heading", { name: "Remote learning" })).toHaveCount(0);

  await page.getByRole("combobox", { name: "Operating mode", exact: true }).selectOption("HYBRID");
  await page.getByRole("button", { name: "Save operating mode" }).click();
  await expect(page.getByText("Current mode: Hybrid")).toBeVisible();
  await teacher.reload();
  await expect(schoolCard.getByRole("heading", { name: "Hybrid learning" })).toBeVisible();

  await page.getByRole("combobox", { name: "Operating mode", exact: true }).selectOption("ON_SITE");
  await page.getByRole("button", { name: "Save operating mode" }).click();
  await expect(page.getByText("Current mode: On site")).toBeVisible();
  await teacher.reload();
  await expect(schoolCard.getByRole("heading", { name: "Fictional Academic School A" })).toBeVisible();
  await expect(schoolCard.getByRole("region", { name: "School operating mode" })).toHaveCount(0);
}
