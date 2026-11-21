import { expect, test } from "@playwright/test";

test("owner enrolls and transfers a student and sees both placements after reload", async ({ page }) => {
  const password = process.env.E2E_ADMIN_PASSWORD;
  if (!password) throw new Error("Fictional fixture password is required");
  await page.goto("/login");
  await page.getByLabel("Email Address").fill("academic-owner@example.test");
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/dashboard$/);

  await page.goto("/schools/academic-fixture-a/students");
  await page.getByLabel("Admission number").fill("ENROLLMENT-FIXTURE-01");
  await page.getByLabel("Display name").fill("Fictional Enrolled Learner");
  await page.getByRole("button", { name: "Save student" }).click();
  await page.getByRole("link", { name: "Enrollment for Fictional Enrolled Learner" }).click();
  await expect(page.getByRole("heading", { name: "Enrollment for Fictional Enrolled Learner" })).toBeVisible();
  await page.getByRole("combobox", { name: "Term", exact: true }).selectOption({ label: "Fixture First Term" });
  await page.getByRole("combobox", { name: "Class", exact: true }).selectOption({ label: "Fixture Class A" });
  await page.getByRole("combobox", { name: "Class arm", exact: true }).selectOption({ label: "Fixture Arm A" });
  await page.getByRole("button", { name: "Save placement" }).click();
  await expect(page.getByText("Enrollment saved.")).toBeVisible();
  await page.getByRole("button", { name: "Transfer Fixture Class A, Fixture First Term" }).click();
  await page.getByRole("combobox", { name: "Class", exact: true }).selectOption({ label: "Fixture Class B" });
  await page.getByRole("button", { name: "Save placement" }).click();
  await expect(page.getByText("Student transferred.")).toBeVisible();
  await page.reload();

  const history = page.getByRole("region", { name: "Placement history" });
  await expect(history.getByRole("listitem").filter({ hasText: "Fixture Class A" })).toContainText("Ended");
  await expect(history.getByRole("listitem").filter({ hasText: "Fixture Class B" })).toContainText("Active");
  const foreign = new URL(page.url());
  foreign.pathname = foreign.pathname.replace("academic-fixture-a", "academic-fixture-b");
  await page.goto(foreign.href);
  await expect(page.getByText("Enrollment is unavailable to your account for this school.")).toBeVisible();
  await expect(page.getByText("Fixture Class B")).toHaveCount(0);
});