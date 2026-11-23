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
  test.setTimeout(60_000);
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
    await teacher.getByRole("link", { name: "Attendance roster" }).click();
    const schoolDate = new Intl.DateTimeFormat("en-CA", { timeZone: "Africa/Lagos", year: "numeric", month: "2-digit", day: "2-digit" }).format(new Date());
    await teacher.getByLabel("Attendance date").fill(schoolDate);
    await teacher.getByRole("button", { name: "Load roster" }).click();
    const learnerRow = teacher.getByRole("listitem").filter({ hasText: "Fictional Learner" });
    await expect(learnerRow.getByText("Not recorded")).toBeVisible();
    await expect(teacher.getByText("Eligible learners: 1. No mark recorded: 1.")).toBeVisible();
    const totals = teacher.getByRole("table", { name: "Participation by attendance mode" });
    const remoteCounts = totals.getByRole("row").filter({ hasText: "Remote" }).getByRole("cell");
    await learnerRow.getByRole("combobox", { name: "Status" }).selectOption("PRESENT");
    await learnerRow.getByRole("combobox", { name: "Mode" }).selectOption("REMOTE");
    await teacher.getByRole("button", { name: "Save attendance" }).click();
    await expect(learnerRow.getByText("present (Remote)", { exact: true })).toBeVisible();
    await expect(teacher.getByText("Eligible learners: 1. No mark recorded: 0.")).toBeVisible();
    await expect(remoteCounts).toHaveText(["1", "0", "0", "0"]);
    await teacher.reload();
    await expect(learnerRow.getByText("present (Remote)", { exact: true })).toBeVisible();
    await learnerRow.getByRole("combobox", { name: "Status" }).selectOption("LATE");
    await teacher.getByRole("button", { name: "Save attendance" }).click();
    await expect(teacher.getByRole("alert").filter({ hasText: "enter a reason" })).toBeVisible();
    await learnerRow.getByLabel("Correction reason").fill("Fictional roll call correction");
    await teacher.getByRole("button", { name: "Save attendance" }).click();
    await expect(learnerRow.getByText("late (Remote)", { exact: true })).toBeVisible();
    await teacher.reload();
    await expect(learnerRow.getByText("late (Remote)", { exact: true })).toBeVisible();
    await expect(remoteCounts).toHaveText(["0", "1", "0", "0"]);
    await expect(totals.getByRole("row").filter({ hasText: "On site" }).getByRole("cell"))
      .toHaveText(["0", "0", "0", "0"]);
    const reviewURL = reviewer.url();
    await reviewer.goto("/schools/classroom-fixture/notices");
    await reviewer.getByLabel("Notice title").fill("Fictional class notice");
    await reviewer.getByLabel("Notice text").fill("Bring a fictional worksheet.");
    await reviewer.getByRole("combobox", { name: "Audience", exact: true }).selectOption("CLASS");
    await reviewer.getByRole("combobox", { name: "Target class", exact: true }).selectOption({ label: "Fictional classroom" });
    await reviewer.getByRole("button", { name: "Publish notice" }).click();
    await expect(reviewer.getByText("Notice published to the selected audience.")).toBeVisible();
    await reviewer.goto(reviewURL);
    const parentContext = await browser.newContext({ baseURL, ignoreHTTPSErrors: true, viewport: { width: 360, height: 800 } });
    try {
      const parent = await parentContext.newPage();
      await parent.goto("/login");
      await parent.getByLabel("Email Address").fill("academic-family@example.test");
      await parent.getByLabel("Password", { exact: true }).fill(password);
      await parent.getByRole("button", { name: "Sign in" }).click();
      await parent.getByRole("link", { name: "School notices" }).click();
      await expect(parent.getByRole("heading", { name: "Fictional class notice" })).toBeVisible();
      await expect(parent.getByRole("button", { name: "Publish notice" })).toHaveCount(0);
      await parent.getByRole("link", { name: "Back to My Schools" }).click();
      await parent.getByRole("link", { name: "Family attendance" }).click();
      await parent.getByRole("link", { name: "Fictional Learner" }).click();
      await expect(parent.getByText(/Late.*Remote/)).toBeVisible();
      await expect(parent.getByText("Fictional roll call correction")).toHaveCount(0);
      const child = new URL(parent.url()).searchParams.get("child");
      if (!child) throw new Error("Fictional child is missing");
      const revoked = await reviewer.evaluate(async (student) => {
        const session = await fetch("/api/v1/auth/me");
        const { data } = await session.json();
        const result = await fetch("/api/v1/tenants/classroom-fixture/students/" + encodeURIComponent(student) +
          "/guardians/124acf1f-3f16-4054-8c27-2a2dd29f5795", {
          method: "DELETE", headers: { "X-CSRF-Token": data.csrf_token },
        });
        return result.ok;
      }, child);
      expect(revoked).toBe(true);
      await parent.reload();
      await expect(parent.getByText("Attendance is unavailable for this child.")).toBeVisible();
      await expect(parent.getByText(/Late.*Remote/)).toHaveCount(0);
    } finally {
      await parentContext.close();
    }
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
    const title = "Counting homework fixture";
    const due = new Date(Date.now() + 86_400_000).toISOString().slice(0, 16);
    await teacher.getByLabel("Assignment title").fill(title);
    await teacher.getByLabel("Instructions", { exact: true }).fill("Count five fictional objects.");
    await teacher.getByLabel("Deadline (UTC)").fill(due);
    await teacher.getByRole("button", { name: "Save assignment draft" }).click();
    await expect(teacher.getByRole("heading", { name: title })).toBeVisible();
    await page.reload();
    await page.getByRole("link", { name: "Teacher-created counting exercise" }).click();
    await expect(page.getByText("Count the five practice counters.")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Optional enrichment" })).toBeVisible();
    await expect(page.getByText(/Group fictional objects in pairs/)).toBeVisible();
    if (mediaEnabled) await checkAttachment(page, page.getByRole("button", { name: attachmentButton }));
    await expect(page.getByRole("heading", { name: title })).toHaveCount(0);
    await teacher.getByRole("button", { name: "Edit draft", exact: true }).click();
    await teacher.getByLabel("Instructions", { exact: true }).fill("Count six fictional objects.");
    await teacher.getByRole("button", { name: "Save assignment draft" }).click();
    await expect(teacher.getByText("Assignment draft saved.")).toBeVisible();
    await teacher.getByRole("button", { name: "Open assignment", exact: true }).click();
    await expect(teacher.getByText("Assignment opened.")).toBeVisible();
    await page.reload();
    await expect(page.getByRole("heading", { name: title })).toBeVisible();
    await expect(page.getByText("Count six fictional objects.")).toBeVisible();
    await page.getByLabel("Your answer").fill("There are six fictional objects.");
    await page.getByRole("button", { name: "Submit answer" }).click();
    await expect(page.getByText(/Submission 1 received on/)).toBeVisible();
    await page.reload();
    await expect(page.getByText(/Submission 1 received on/)).toBeVisible();
    await expect(page.getByRole("button", { name: "Resubmit answer" })).toHaveCount(0);
    await teacher.getByRole("link", { name: "Review submissions" }).click();
    await expect(teacher.getByText("There are six fictional objects.")).toBeVisible();
    await teacher.getByLabel("Score out of 10").fill("8");
    await teacher.getByLabel("Feedback comment").fill("Recheck the final group.");
    await teacher.getByRole("button", { name: "Return feedback" }).click();
    await expect(teacher.getByRole("button", { name: "Update feedback" })).toBeVisible();
    await page.reload();
    await expect(page.getByText("Score 8 out of 10")).toBeVisible();
    await expect(page.getByText("Recheck the final group.")).toBeVisible();
    await teacher.getByLabel("Score out of 10").fill("9");
    await teacher.getByLabel("Feedback comment").fill("Final group verified.");
    await teacher.getByRole("button", { name: "Update feedback" }).click();
    await expect(teacher.getByText("Feedback returned.")).toBeVisible();
    await page.reload();
    await expect(page.getByText("Score 9 out of 10")).toBeVisible();
    await expect(page.getByText("Final group verified.")).toBeVisible();
    await teacher.getByRole("button", { name: "Close assignment", exact: true }).click();
    await teacher.getByRole("button", { name: "Confirm closure" }).click();
    await expect(teacher.getByText("Assignment closed.")).toBeVisible();
    await page.reload();
    await expect(page.getByText("Closed", { exact: false })).toBeVisible();
    if (mediaEnabled) {
      await teacher.getByLabel("Assignment title").fill("Fictional file assignment");
      await teacher.getByLabel("Instructions", { exact: true }).fill("Upload the fictional worksheet.");
      await teacher.getByLabel("Deadline (UTC)").fill(due);
      await teacher.getByLabel("Submission type").selectOption("FILE");
      await teacher.getByRole("button", { name: "Save assignment draft" }).click();
      const fileAssignment = teacher.getByRole("listitem").filter({ hasText: "Fictional file assignment" });
      await fileAssignment.getByRole("button", { name: "Open assignment", exact: true }).click();
      await expect(fileAssignment.getByRole("link", { name: "Review submissions" })).toBeVisible();
      await page.reload();
      const fileCard = page.getByRole("listitem").filter({ hasText: "Fictional file assignment" });
      await fileCard.getByLabel(/Your file/).setInputFiles({
        name: "counting.pdf", mimeType: "application/pdf", buffer: fixturePDF,
      });
      await fileCard.getByRole("button", { name: "Submit file" }).click();
      await expect(fileCard.getByText(/Submission 1 received on/)).toBeVisible();
      await page.reload();
      await expect(fileCard.getByText(/Submission 1 received on/)).toBeVisible();
      await fileAssignment.getByRole("link", { name: "Review submissions" }).click();
      await expect(teacher.getByRole("heading", { name: "Submissions for Fictional file assignment" })).toBeVisible();
      await checkAttachment(teacher, teacher.getByRole("button", { name: attachmentButton }));
    }

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
