import { expect, test } from "@playwright/test";

test("an enrolled student reads a published text lesson on mobile", async ({ page }) => {
  const password = process.env.E2E_ADMIN_PASSWORD;
  if (!password) throw new Error("Fictional fixture password is required");
  await page.setViewportSize({ width: 360, height: 800 });
  await page.goto("/schools/classroom-fixture/learning");
  await expect(page).toHaveURL(/\/login$/);
  await page.getByLabel("Email Address").fill("academic-learner@example.test");
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.getByRole("article").filter({ hasText: "Fictional Classroom School" }).getByRole("link", { name: "Lessons" }).click();
  await expect(page.getByRole("heading", { name: "Lessons" })).toBeVisible();
  await expect(page.getByText("Unpublished classroom draft")).toHaveCount(0);
  await page.getByRole("link", { name: "Count the classroom objects" }).click();
  await expect(page.getByRole("heading", { name: "Count the classroom objects" })).toBeVisible();
  await expect(page.getByText("Read the example, then count <strong>three</strong> objects.", { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  const publishedPath = new URL(page.url()).pathname;
  await page.goto(publishedPath.replace("classroom-fixture", "academic-fixture-b"));
  await expect(page.getByRole("heading", { name: "Lesson unavailable" })).toBeVisible();
  await page.goto("/schools/classroom-fixture/learning/13d0ffdf-aae1-4b72-b64c-6828436e94f2");
  await expect(page.getByRole("heading", { name: "Lesson unavailable" })).toBeVisible();
});
