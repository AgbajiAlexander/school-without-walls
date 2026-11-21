import { expect, test } from "@playwright/test";

test("a school member browses published editions and filters objectives on mobile", async ({ page }) => {
  const password = process.env.E2E_ADMIN_PASSWORD;
  if (!password) throw new Error("Fictional fixture password is required");
  await page.setViewportSize({ width: 360, height: 800 });
  await page.goto("/curriculum");
  await expect(page).toHaveURL(/\/login$/);
  await page.getByLabel("Email Address").fill("academic-curriculum@example.test");
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.getByRole("link", { name: "Browse curriculum" }).click();

  const nigeria = page.getByRole("listitem").filter({ hasText: "Fictional Nigerian Framework" });
  await expect(nigeria).toContainText("Nigerian curriculum");
  await expect(nigeria).toContainText("Fictional source note");
  await expect(nigeria.getByRole("link", { name: "Publisher source" })).toHaveAttribute("href", "https://example.test/curriculum");
  await expect(page.getByRole("listitem").filter({ hasText: "Fictional British Framework" })).toContainText("Optional enrichment");
  await expect(page.getByText("Fictional Unpublished Framework")).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);

  await nigeria.getByRole("link", { name: /Browse objectives/ }).click();
  await expect(page.getByText("Count the objects in a fictional group.")).toBeVisible();
  await expect(page.getByText("Compare two fictional shapes.")).toBeVisible();
  await page.getByLabel("Education stage").selectOption("PRIMARY");
  await page.getByRole("button", { name: "Filter objectives" }).click();
  await expect(page.getByText("Count the objects in a fictional group.")).toBeVisible();
  await expect(page.getByText("Compare two fictional shapes.")).toHaveCount(0);
  await page.getByLabel("Education stage").selectOption("SENIOR_SECONDARY");
  await page.getByRole("button", { name: "Filter objectives" }).click();
  await expect(page.getByRole("status")).toHaveText("No published objectives match this selection.");
});
