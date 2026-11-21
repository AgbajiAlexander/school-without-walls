import { expect, test } from "@playwright/test";

test("school owner registers and edits an existing staff profile and teacher cannot open it", async ({ page, browser, baseURL }) => {
  const password = process.env.E2E_ADMIN_PASSWORD;
  if (!password) throw new Error("Fictional fixture password is required");
  const school = "academic-fixture-a";
  const path = `/schools/${school}/staff`;
  const endpoint = `/api/v1/tenants/${school}/staff`;
  await page.goto("/login");
  await page.getByLabel("Email Address").fill("academic-owner@example.test");
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/dashboard$/);
  await page.goto(path);
  await expect(page.getByRole("heading", { name: "Staff profiles" })).toBeVisible();
  await page.getByLabel("Staff membership").selectOption({ label: "Fictional Teacher (teacher) \u2014 academic-teacher@example.test" });
  await page.getByLabel("Staff code").fill("TEACHER-1");
  const created = page.waitForResponse((response) =>
    new URL(response.url()).pathname === endpoint && response.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Save staff profile" }).click();
  expect((await created).status()).toBe(201);
  await expect(page.getByText("TEACHER-1", { exact: true })).toBeVisible();
  await expect(page.getByRole("option", { name: /Fictional Teacher \(teacher\).*academic-teacher@example\.test/ })).toHaveCount(0);
  await page.getByRole("button", { name: "Edit Fictional Teacher" }).click();
  await page.getByLabel("Staff code").fill("TEACHER-2");
  await page.getByRole("button", { name: "Save staff profile" }).click();
  await expect(page.getByText("TEACHER-2", { exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByText("TEACHER-2", { exact: true })).toBeVisible();
  await page.goto("/schools/academic-fixture-b/staff");
  await expect(page.getByText("Staff profiles are unavailable to your account for this school.")).toBeVisible();

  const context = await browser.newContext({ baseURL, ignoreHTTPSErrors: true, viewport: { width: 390, height: 844 } });
  try {
    const teacher = await context.newPage();
    await teacher.goto("/login");
    await teacher.getByLabel("Email Address").fill("academic-teacher@example.test");
    await teacher.getByLabel("Password", { exact: true }).fill(password);
    await teacher.getByRole("button", { name: "Sign in" }).click();
    await expect(teacher).toHaveURL(/\/dashboard$/);
    await teacher.goto(path);
    await expect(teacher.getByText("Staff profiles are unavailable to your account for this school.")).toBeVisible();
    await expect(teacher.getByLabel("Staff membership")).toHaveCount(0);
    await expect(teacher.getByText("TEACHER-2", { exact: true })).toHaveCount(0);
  } finally {
    await context.close();
  }
});