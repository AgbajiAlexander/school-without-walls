import { expect, test, type Page } from "@playwright/test";

async function signIn(page: Page, email: string) {
  const password = process.env.E2E_ADMIN_PASSWORD;
  if (!password) throw new Error("Fictional fixture password is required");
  await page.goto("/login");
  await page.getByLabel("Email Address").fill(email);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/dashboard$/);
}

test("owner registers and edits a student while teachers and other schools are denied", async ({ page, browser, baseURL }) => {
  const path = "/schools/academic-fixture-a/students";
  await signIn(page, "academic-owner@example.test");
  await page.getByRole("link", { name: "Students", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(path + "$"));
  await page.getByLabel("Admission number").fill("FICTIONAL-001");
  await page.getByLabel("Display name").fill("Fictional Browser Learner");
  await page.getByRole("button", { name: "Save student" }).click();
  await expect(page.getByText("Student registered.")).toBeVisible();
  await page.getByRole("button", { name: "Edit Fictional Browser Learner" }).click();
  await page.getByLabel("Education stage").selectOption("JUNIOR_SECONDARY");
  await page.getByRole("button", { name: "Save student" }).click();
  await expect(page.getByText("Student profile updated.")).toBeVisible();
  await page.reload();
  await expect(page.getByText(/FICTIONAL-001 · Junior Secondary · Active/)).toBeVisible();
  await page.goto("/schools/academic-fixture-b/students");
  await expect(page.getByText("Student management is unavailable to your account for this school.")).toBeVisible();
  await expect(page.getByText("Fictional Browser Learner")).toHaveCount(0);

  const teacherContext = await browser.newContext({ baseURL, ignoreHTTPSErrors: true, viewport: { width: 390, height: 844 } });
  try {
    const teacher = await teacherContext.newPage();
    await signIn(teacher, "academic-teacher@example.test");
    await teacher.goto(path);
    await expect(teacher.getByText("Student management is unavailable to your account for this school.")).toBeVisible();
    await expect(teacher.getByLabel("Admission number")).toHaveCount(0);
    await expect(teacher.getByText("Fictional Browser Learner")).toHaveCount(0);
  } finally {
    await teacherContext.close();
  }
});