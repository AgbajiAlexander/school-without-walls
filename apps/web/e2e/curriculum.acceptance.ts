import { expect, test } from "@playwright/test";

test("a school member browses published editions and filters objectives on mobile", async ({ page, browser, baseURL }) => {
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
  await page.getByRole("link", { name: "Verified enrichment for FIX.COUNT" }).click();
  await expect(page.getByRole("heading", { name: "Verified enrichment alignments" })).toBeVisible();
  await expect(page.getByText("Optional enrichment", { exact: false })).toContainText("Complementary");
  await expect(page.getByText("Group fictional objects in pairs.")).toBeVisible();
  await expect(page.getByText("Fictional draft alignment text.")).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Publisher source" })).toHaveAttribute("href", "https://example.test/enrichment");
  await page.getByRole("link", { name: "Back to objectives" }).click();
  await expect(page.getByLabel("Education stage")).toHaveValue("PRIMARY");
  await page.getByLabel("Education stage").selectOption("SENIOR_SECONDARY");
  await page.getByRole("button", { name: "Filter objectives" }).click();
  await expect(page.getByRole("status")).toHaveText("No published objectives match this selection.");

  await page.goto("/platform/curriculum");
  await expect(page.getByRole("status")).toHaveText("Global curriculum administrator access required.");
  await expect(page.getByText("Fictional draft alignment text.")).toHaveCount(0);

  const operator = await browser.newContext({ baseURL, ignoreHTTPSErrors: true });
  try {
    const review = await operator.newPage();
    await review.goto("/login");
    await review.getByLabel("Email Address").fill("academic-reviewer@example.test");
    await review.getByLabel("Password", { exact: true }).fill(password);
    await review.getByRole("button", { name: "Sign in" }).click();
    await expect(review).toHaveURL(/\/dashboard$/);
    await review.goto("/platform/tenants");
    await review.getByRole("link", { name: "Curriculum alignment review" }).click();
    await expect(review.getByText("Fictional draft alignment text.")).toBeVisible();
    await review.getByRole("button", { name: "Review mapping FIX.COUNT to FIX.DRAFT" }).click();
    await expect(review.getByText("Make this mapping visible as verified enrichment?")).toBeVisible();
    await review.getByRole("button", { name: "Confirm approval" }).click();
    await expect(review.getByText("Alignment approved.")).toBeVisible();
    await review.reload();
    await expect(review.getByText("No draft mappings on this page.")).toBeVisible();
  } finally {
    await operator.close();
  }

  await page.goto("/curriculum");
  await nigeria.getByRole("link", { name: /Browse objectives/ }).click();
  await page.getByRole("link", { name: "Verified enrichment for FIX.COUNT" }).click();
  await expect(page.getByText("Fictional draft alignment text.")).toBeVisible();
});
