import { readFile } from "node:fs/promises";
import { expect, test } from "@playwright/test";

const mediaEnabled = process.env.SWW_E2E_LESSON_MEDIA === "1";
const attachmentButton = "Download lesson attachment";
const fixturePDF = Buffer.from("%PDF-1.4\nFictional counting worksheet\n%%EOF\n");

async function checkAttachment(page: import("@playwright/test").Page, button: import("@playwright/test").Locator) {
  const [download] = await Promise.all([
    page.waitForEvent("download", { timeout: 15_000 }),
    button.click(),
  ]);
  expect(download.suggestedFilename()).toBe("counting.pdf");
  const file = await download.path();
  expect(file && (await readFile(file)).equals(fixturePDF)).toBe(true);
}

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
  const progress = page.getByRole("status").filter({ hasText: /^(Not started|In progress|Completed)$/ });
  await expect(progress).toHaveText("Not started");
  await page.getByRole("button", { name: "Start lesson" }).click();
  await expect(progress).toHaveText("In progress");
  await page.reload();
  await expect(progress).toHaveText("In progress");
  await page.getByRole("button", { name: "Mark complete" }).click();
  await expect(progress).toHaveText("Completed");
  await page.reload();
  await expect(progress).toHaveText("Completed");
  await expect(page.getByRole("button", { name: "Mark complete" })).toHaveCount(0);
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

    if (mediaEnabled) {
      await teacher.getByLabel("Lesson attachment").setInputFiles({
        name: "counting.pdf", mimeType: "application/pdf", buffer: fixturePDF,
      });
      const checkpoints: string[] = [];
      teacher.on("response", (response) => {
        const url = new URL(response.url());
        if (url.pathname.endsWith("/media")) checkpoints.push("intent " + response.status());
        if (url.pathname.endsWith("/complete")) checkpoints.push("completion " + response.status());
        if (url.hostname === "127.0.0.2") checkpoints.push("storage " + response.status());
      });
      teacher.on("requestfailed", (request) => {
        if (new URL(request.url()).hostname === "127.0.0.2") checkpoints.push("storage blocked");
      });
      await teacher.getByRole("button", { name: "Upload attachment" }).click();
      try {
        await expect(teacher.getByText("Attachment scanned and added to this draft.")).toBeVisible();
      } catch {
        throw new Error("Upload did not complete: " + (checkpoints.join(", ") || "no network requests"));
      }
    }
    await page.goto("/schools/classroom-fixture/learning");
    await expect(page.getByRole("link", { name: "Teacher-created counting exercise" })).toHaveCount(0);
    await expect(teacher.getByRole("button", { name: "Publish lesson" })).toHaveCount(0);
    await teacher.getByRole("button", { name: "Submit for review" }).click();
    await expect(teacher.getByText("Awaiting approval.")).toBeVisible();
    await reviewer.reload();
    const review = reviewer.getByRole("article").filter({ hasText: "Teacher-created counting exercise" });
    await expect(review.getByText("Count the five practice counters.")).toBeVisible();
    await expect(review.getByText(/Group fictional objects in pairs/)).toBeVisible();
    if (mediaEnabled) await checkAttachment(reviewer, review.getByRole("button", { name: attachmentButton }));
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
    if (mediaEnabled) await checkAttachment(page, page.getByRole("button", { name: attachmentButton }));

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
