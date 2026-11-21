import { expect, test } from "@playwright/test";

test("a school owner registers, edits, links and unlinks a guardian", async ({ page }) => {
  const password = process.env.E2E_ADMIN_PASSWORD;
  if (!password) throw new Error("Fictional fixture password is required");
  await page.goto("/login");
  await page.getByLabel("Email Address").fill("academic-owner@example.test");
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/dashboard$/);

  await page.goto("/schools/academic-fixture-a/students");
  await page.getByLabel("Admission number").fill("GUARDIAN-FIXTURE-01");
  await page.getByLabel("Display name").fill("Fictional Linked Learner");
  await page.getByRole("button", { name: "Save student" }).click();
  await page.getByRole("link", { name: "Guardians for Fictional Linked Learner" }).click();
  await expect(page.getByRole("heading", { name: "Guardians for Fictional Linked Learner" })).toBeVisible();

  await page.getByLabel("Guardian display name").fill("Fictional Browser Guardian");
  await page.getByLabel("Contact email").fill("fictional-guardian@example.test");
  await page.getByRole("button", { name: "Save guardian profile" }).click();
  await expect(page.getByText("Guardian profile saved.")).toBeVisible();
  await page.getByRole("button", { name: "Edit Fictional Browser Guardian" }).click();
  await page.getByLabel("Guardian display name").fill("Fictional Updated Guardian");
  await page.getByRole("button", { name: "Save guardian profile" }).click();

  await page.getByLabel("Guardian to link").selectOption({ label: "Fictional Updated Guardian \u2014 fictional-guardian@example.test" });
  await page.getByLabel("Relationship").fill("Parent");
  await page.getByRole("button", { name: "Save relationship" }).click();
  await expect(page.getByText(/Fictional Updated Guardian.*Parent/)).toBeVisible();
  await page.reload();
  await expect(page.getByText(/Fictional Updated Guardian.*Parent/)).toBeVisible();
  await page.getByRole("button", { name: "Unlink Fictional Updated Guardian" }).click();
  await page.getByRole("button", { name: "Confirm unlink" }).click();
  await expect(page.getByText("Guardian unlinked.")).toBeVisible();
  await page.reload();
  await expect(page.getByText("No linked guardians on this page.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Edit Fictional Updated Guardian", exact: true })).toBeVisible();

  await page.goto("/schools/academic-fixture-b/guardians");
  await expect(page.getByText("Guardian management is unavailable to your account for this school.")).toBeVisible();
  await expect(page.getByText("fictional-guardian@example.test", { exact: true })).toHaveCount(0);
});