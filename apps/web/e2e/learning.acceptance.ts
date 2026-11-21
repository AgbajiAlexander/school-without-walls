import { expect, test } from "@playwright/test";

test("an enrolled student reads a published text lesson on mobile", async ({ page, browser, baseURL }) => {
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
  const teacherContext = await browser.newContext({ baseURL, ignoreHTTPSErrors: true });
  const reviewerContext = await browser.newContext({ baseURL, ignoreHTTPSErrors: true });
  try {
    const reviewer = await reviewerContext.newPage();
    await reviewer.goto("/login");
    await reviewer.getByLabel("Email Address").fill("academic-reviewer@example.test");
    await reviewer.getByLabel("Password", { exact: true }).fill(password);
    await reviewer.getByRole("button", { name: "Sign in" }).click();
    await reviewer.getByRole("article").filter({ hasText: "Fictional Classroom School" })
      .getByRole("link", { name: "Lesson reviews" }).click();
    await reviewer.getByRole("button", { name: "Require lesson approval" }).click();
    await expect(reviewer.getByText("Approval is required before publication.")).toBeVisible();
    const teacher = await teacherContext.newPage();
    await teacher.goto("/login");
    await teacher.getByLabel("Email Address").fill("academic-teacher@example.test");
    await teacher.getByLabel("Password", { exact: true }).fill(password);
    await teacher.getByRole("button", { name: "Sign in" }).click();
    await teacher.getByRole("article").filter({ hasText: "Fictional Classroom School" })
      .getByRole("link", { name: "Teaching lessons" }).click();
    await teacher.getByRole("link", { name: /Fictional number work/ }).click();
    const selector = teacher.getByLabel("Primary objective");
    const choice = selector.locator("option").filter({ hasText: "FIX.COUNT" });
    await expect(choice).toHaveCount(1);
    const objective = await choice.getAttribute("value");
    if (!objective) throw new Error("Expected a curriculum objective identifier");
    await selector.selectOption(objective);
    await teacher.getByRole("checkbox", { name: /FIX.EXTEND/ }).check();
    await teacher.getByLabel("Lesson title").fill("Teacher-created counting exercise");
    await teacher.getByLabel("Lesson text").fill("Count the five practice counters.");
    await teacher.getByRole("button", { name: "Save lesson draft" }).click();
    await expect(teacher.getByRole("heading", { name: "Teacher-created counting exercise" })).toBeVisible();

    await page.goto("/schools/classroom-fixture/learning");
    await expect(page.getByRole("link", { name: "Teacher-created counting exercise" })).toHaveCount(0);
    await expect(teacher.getByRole("button", { name: "Publish lesson" })).toHaveCount(0);
    await teacher.getByRole("button", { name: "Submit for review" }).click();
    await expect(teacher.getByText("Awaiting approval.")).toBeVisible();
    await reviewer.reload();
    const review = reviewer.getByRole("article").filter({ hasText: "Teacher-created counting exercise" });
    await expect(review.getByText("Count the five practice counters.")).toBeVisible();
    await expect(review.getByText(/Group fictional objects in pairs/)).toBeVisible();
    await review.getByRole("button", { name: "Review lesson" }).click();
    await review.getByRole("button", { name: "Confirm approval" }).click();
    await expect(reviewer.getByText("Lesson revision approved.")).toBeVisible();
    await teacher.reload();
    await expect(teacher.getByText("Current revision approved.")).toBeVisible();
    await teacher.getByRole("button", { name: "Publish lesson" }).click();
    await teacher.getByRole("button", { name: "Confirm publication" }).click();
    await expect(teacher.getByText("Lesson published.", { exact: true })).toBeVisible();
    await page.reload();
    await page.getByRole("link", { name: "Teacher-created counting exercise" }).click();
    await expect(page.getByText("Count the five practice counters.")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Optional enrichment" })).toBeVisible();
    await expect(page.getByText(/Group fictional objects in pairs/)).toBeVisible();

    await page.goto(new URL(teacher.url()).pathname + new URL(teacher.url()).search);
    await expect(page.getByText("Teaching lesson unavailable.")).toBeVisible();
    await expect(page.getByText("Count the five practice counters.")).toHaveCount(0);
  } finally {
    await teacherContext.close();
    await reviewerContext.close();
  }
  await page.goto(publishedPath.replace("classroom-fixture", "academic-fixture-b"));
  await expect(page.getByRole("heading", { name: "Lesson unavailable" })).toBeVisible();
  await page.goto("/schools/classroom-fixture/learning/13d0ffdf-aae1-4b72-b64c-6828436e94f2");
  await expect(page.getByRole("heading", { name: "Lesson unavailable" })).toBeVisible();
});
