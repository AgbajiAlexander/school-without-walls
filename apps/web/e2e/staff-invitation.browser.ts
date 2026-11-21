import { randomBytes } from "node:crypto";
import { expect, test, type BrowserContext, type Page } from "@playwright/test";

const origin = "http://127.0.0.1:13000";
const operator = "browser-platform@school.invalid";
const inviteButton = { name: "Send invitation" };

const inviteAlert = (page: Page) => page.getByRole("region", { name: "Invite Staff" }).getByRole("alert");

test.afterEach(async ({ browser }) => {
  for (const context of browser.contexts()) {
    await Promise.all(context.pages().map((page) => page.goto("about:blank").catch(() => {})));
    await context.close();
  }
});

async function withSafeFixtureDiagnostics<T>(operation: () => Promise<T>): Promise<T> {
  try {
    return await operation();
  } catch {
    throw new Error("Staff fixture request failed");
  }
}

async function signIn(page: Page, email: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("Email Address").fill(email);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/dashboard$/);
}

async function post(context: BrowserContext, path: string, data: object) {
  const csrf = (await context.cookies(origin)).find(({ name }) => name === "sww_csrf");
  if (!csrf) throw new Error("Missing browser CSRF cookie");
  return context.request.post(path, { headers: { Origin: origin, "X-CSRF-Token": csrf.value }, data });
}

async function captured(context: BrowserContext, email: string, schoolName?: string): Promise<string> {
  const response = await context.request.get("/api/v1/platform/mail-capture");
  expect(response.status()).toBe(200);
  expect(response.headers()["cache-control"]).toBe("no-store");
  const { data } = await response.json() as { data: { messages: { to: string; body: string }[] } };
  const messages = data.messages.filter(({ to, body }) => to === email && (!schoolName || body.includes(schoolName)));
  if (messages.length !== 1) throw new Error("Expected one locally captured message for this fictional address");
  const link = messages[0].body.match(/http:\/\/127\.0\.0\.1:13000\/invite\/[\w-]+/)?.[0];
  if (!link) throw new Error("No local invitation link in captured message");
  return link;
}

async function redeem(page: Page, link: string, password: string) {
  let responsePayloadSafe = false;
  try {
    await page.goto(link);
    await page.getByLabel("Full Name").fill("Fictional Staff Member");
    await page.getByLabel("Create or confirm password").fill(password);
    await page.getByLabel("Confirm Password", { exact: true }).fill(password);
    const completion = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/v1/invitations/accept");
    await page.getByRole("button", { name: "Accept Invitation & Sign In" }).click();
    const response = await completion;
    const payload = await response.text();
    const token = new URL(link).pathname.split("/").at(-1)!;
    const session = (await page.context().cookies(origin)).find(({ name }) => name === "sww_session");
    responsePayloadSafe = response.status() === 201 && !payload.includes(token) && !payload.includes(password)
      && response.headers()["cache-control"] === "no-store" && session?.httpOnly === true && !payload.includes(session.value);
    await page.waitForURL(/\/schools\/[\w-]+\/settings$/);
    if (!await page.evaluate((secrets) => {
      const storage = [...Object.values(localStorage), ...Object.values(sessionStorage)];
      return secrets.every((secret) => !location.href.includes(secret) && !document.cookie.includes(secret)
        && !storage.some((value) => value.includes(secret)));
    }, [token, password, ...(session ? [session.value] : [])])) {
      throw new Error("Invitation secret remained in the active URL or browser storage");
    }
  } catch {
    throw new Error("Browser invitation activation failed");
  }
  expect(responsePayloadSafe).toBe(true);
}

async function fillInvite(page: Page, email: string, role = "SCHOOL_ADMIN") {
  await page.getByLabel("Staff email address").fill(email);
  await page.getByLabel("Staff role").selectOption(role);
}

test("fictional owners invite both staff roles and the browser enforces invitation boundaries", async ({ browser, page: owner }) => {
  const suffix = randomBytes(4).toString("hex");
  const school = { slug: `browser-a-${suffix}`, name: "Fictional Browser School A" };
  const other = { slug: `browser-b-${suffix}`, name: "Fictional Browser School B" };
  const ownerEmail = `${school.slug}@school.invalid`;
  const ownerPassword = randomBytes(24).toString("base64url");
  const platformContext = await browser.newContext({ baseURL: origin });
  const platform = await platformContext.newPage();
  const platformPassword = process.env.SWW_B12_ADMIN_PASSWORD;
  if (!platformPassword) throw new Error("Missing ephemeral fixture credential");
  const fixtureKey = process.env.SWW_B12_FIXTURE_KEY;
  if (!fixtureKey || fixtureKey === platformPassword) throw new Error("Missing independent fixture key");
  await signIn(platform, operator, platformPassword);

  const secretInConsole: boolean[] = [];
  const protectedValues = new Set([platformPassword, ownerPassword, fixtureKey]);
  function watchConsole(page: Page) {
    page.on("console", (message) => {
      if ([...protectedValues].some((value) => message.text().includes(value))) secretInConsole.push(true);
    });
  }
  watchConsole(owner);
  for (const tenant of [school, other]) {
    const provision = await post(platformContext, "/api/v1/platform/tenants", {
      ...tenant, owner_email: ownerEmail, locale: "en-NG", timezone: "Africa/Lagos",
    });
    expect(provision.status()).toBe(201);
    const link = await captured(platformContext, ownerEmail, tenant.name);
    protectedValues.add(link.split("/").at(-1)!);
    await redeem(owner, link, ownerPassword);
    expect((await post(owner.context(), "/api/v1/auth/logout", {})).status()).toBe(200);
  }
  await signIn(owner, ownerEmail, ownerPassword);
  await owner.getByRole("article").filter({ hasText: school.name }).getByRole("link", { name: "Invite Staff" }).click();
  const endpoint = `/api/v1/tenants/${school.slug}/staff-invitations`;

  await fillInvite(owner, "invalid-email");
  await owner.getByRole("button", inviteButton).click();
  await expect(inviteAlert(owner)).toHaveText("Enter a valid email address.");
  await expect(owner.getByLabel("Staff role").locator("option")).toHaveCount(3);
  expect((await post(owner.context(), endpoint, { email: "invalid-role@school.invalid", role: "SCHOOL_OWNER" })).status()).toBe(422);

  await fillInvite(owner, "csrf-test@school.invalid");
  await owner.route(`**${endpoint}`, (route) => {
    const headers = { ...route.request().headers() };
    delete headers["x-csrf-token"];
    return route.continue({ headers });
  }, { times: 1 });
  await owner.getByRole("button", inviteButton).click();
  await expect(owner.getByText("Invitation access was refreshed. Submit again to send the invitation.")).toBeVisible();
  await expect(owner.getByRole("button", inviteButton)).toBeEnabled();

  await owner.route(`**${endpoint}`, (route) => route.abort(), { times: 1 });
  await owner.getByRole("button", inviteButton).click();
  await expect(inviteAlert(owner)).toContainText("delivery could not be confirmed");

  expect((await post(owner.context(), "/api/v1/auth/logout", {})).status()).toBe(200);
  await owner.getByRole("button", inviteButton).click();
  await expect(owner).toHaveURL(/\/login$/);
  await signIn(owner, ownerEmail, ownerPassword);
  await owner.getByRole("article").filter({ hasText: school.name }).getByRole("link", { name: "Invite Staff" }).click();

  let markIntercepted!: () => void;
  let release!: () => void;
  const intercepted = new Promise<void>((resolve) => { markIntercepted = resolve; });
  const held = new Promise<void>((resolve) => { release = resolve; });
  await owner.route(`**${endpoint}`, async (route) => {
    markIntercepted();
    await held;
    await route.abort().catch(() => {});
  }, { times: 1 });
  try {
    await fillInvite(owner, "switch-test@school.invalid");
    await owner.getByRole("button", inviteButton).click();
    await intercepted;
    await expect(owner.getByRole("button", { name: "Sending invitation…" })).toBeDisabled();
    await owner.getByRole("link", { name: "Account" }).click();
    await owner.getByRole("article").filter({ hasText: other.name }).getByRole("link", { name: "Invite Staff" }).click();
    await expect(owner.getByRole("heading", { name: other.name })).toBeVisible();
    await expect(owner.getByLabel("Staff email address")).toHaveValue("");
    await expect(owner.getByLabel("Staff role")).toHaveValue("");
  } finally {
    release();
  }
  await expect(owner.getByText("Invitation sent.", { exact: true })).toHaveCount(0);
  await owner.getByRole("link", { name: "Account" }).click();
  await owner.getByRole("article").filter({ hasText: school.name }).getByRole("link", { name: "Invite Staff" }).click();

  for (const [index, role] of ["SCHOOL_ADMIN", "PRINCIPAL"].entries()) {
    const email = `browser-${index}-${suffix}@school.invalid`;
    await fillInvite(owner, email, role);
    const posted = owner.waitForResponse((response) => new URL(response.url()).pathname === endpoint && response.request().method() === "POST");
    await owner.getByRole("button", inviteButton).click();
    const response = await posted;
    expect(response.status()).toBe(201);
    expect(Boolean(await response.request().headerValue("x-csrf-token"))).toBe(true);
    expect(Boolean(await response.request().headerValue("cookie"))).toBe(true);
    await expect(owner.getByRole("status").filter({ hasText: "Invitation sent." })).toBeVisible();
    const link = await captured(platformContext, email);
    protectedValues.add(link.split("/").at(-1)!);
    const createBody = await response.text();
    expect([...protectedValues].some((value) => createBody.includes(value))).toBe(false);

    await fillInvite(owner, email, role);
    await owner.getByRole("button", inviteButton).click();
    await expect(inviteAlert(owner)).toContainText("already has a pending invitation");
    await captured(platformContext, email);

    const staffContext = await browser.newContext({ baseURL: origin });
    const staff = await staffContext.newPage();
    watchConsole(staff);
    const password = randomBytes(24).toString("base64url");
    protectedValues.add(password);
    await redeem(staff, link, password);
    expect((await post(staffContext, "/api/v1/auth/logout", {})).status()).toBe(200);
    await signIn(staff, email, password);
    const schoolCard = staff.getByRole("article").filter({ hasText: school.name });
    await expect(schoolCard).toContainText(role.replaceAll("_", " "));
    await expect(staff.getByText(other.name, { exact: true })).toHaveCount(0);
    await expect(staff.getByRole("link", { name: "Invite Staff" })).toHaveCount(0);
    await schoolCard.getByRole("link", { name: "Academic Management" }).click();
    await expect(staff).toHaveURL(/\/academics$/);
    const sessions = `/api/v1/tenants/${school.slug}/academic-sessions`;
    expect((await staffContext.request.get(sessions)).status()).toBe(200);
    const year = 2040 + index;
    expect((await post(staffContext, sessions, {
      name: `Fictional ${year}/${year + 1}`, starts_on: `${year}-09-01`, ends_on: `${year + 1}-07-31`,
    })).status()).toBe(201);
    expect((await post(staffContext, endpoint, { email: "forbidden@school.invalid", role })).status()).toBe(403);
    expect((await staffContext.request.get(`/api/v1/tenants/${other.slug}/settings`)).status()).toBe(404);
    await staff.goto(`/schools/${other.slug}/settings`);
    await expect(staff.getByRole("heading", { name: "School access unavailable" })).toBeVisible();
    const replay = await staffContext.request.post("/api/v1/invitations/accept", {
      headers: { Origin: origin },
      data: { token: link.split("/").at(-1), full_name: "Fictional Staff Member", password },
    });
    expect(replay.status()).toBe(422);
    await staffContext.close();
  }
  await owner.bringToFront();
  await expect(owner.getByRole("button", inviteButton)).toBeVisible();
  const fixtureRequest = (operation: string, email: string) => withSafeFixtureDiagnostics(() => platformContext.request.post(
    "http://127.0.0.1:18080/__staff-fixture/owner",
    { headers: { "X-Staff-Fixture-Key": fixtureKey }, data: { slug: school.slug, email, operation } },
  ));
  const fixtureURL = "http://127.0.0.1:18080/__staff-fixture/owner";
  const unauthorized = await platformContext.request.post(fixtureURL, {
    data: { slug: school.slug, email: ownerEmail, operation: "suspend" },
  });
  expect(unauthorized.status()).toBe(404);

  // Model a raw Playwright transport error containing headers and verify that
  // only the fixed diagnostic escapes the same wrapper used by the fixture.
  let safeDiagnostic = false;
  try {
    await withSafeFixtureDiagnostics(async () => {
      throw new Error(`Transport diagnostic: ${fixtureKey} ${platformPassword}`);
    });
  } catch (error) {
    safeDiagnostic = error instanceof Error && error.message === "Staff fixture request failed" &&
      !String(error.stack).includes(fixtureKey) && !String(error.stack).includes(platformPassword) && error.cause === undefined;
  }
  expect(safeDiagnostic).toBe(true);
  const deniedEmail = `suspended-owner-${suffix}@school.invalid`;
  const before = await fixtureRequest("snapshot", deniedEmail);
  expect(before.status()).toBe(200);
  const beforeMail = await platformContext.request.get("/api/v1/platform/mail-capture");
  expect(beforeMail.status()).toBe(200);
  const mailCount = (await beforeMail.json()).data.messages.length;
  await fillInvite(owner, deniedEmail);
  expect((await fixtureRequest("suspend", ownerEmail)).status()).toBe(204);
  const denied = owner.waitForResponse((response) => new URL(response.url()).pathname === endpoint && response.request().method() === "POST");
  await owner.getByRole("button", inviteButton).click();
  expect((await denied).status()).toBe(403);
  await expect(owner.getByRole("heading", { name: "School access unavailable" })).toBeVisible();
  await expect(owner.getByRole("button", inviteButton)).toHaveCount(0);
  const after = await fixtureRequest("snapshot", deniedEmail);
  expect(after.status()).toBe(200);
  expect(await after.json()).toEqual(await before.json());
  const afterMail = await platformContext.request.get("/api/v1/platform/mail-capture");
  expect(afterMail.status()).toBe(200);
  expect((await afterMail.json()).data.messages.length).toBe(mailCount);
  await owner.getByRole("link", { name: "Return to account" }).click();
  await owner.getByRole("article").filter({ hasText: other.name }).getByRole("link", { name: "Invite Staff" }).click();
  await expect(owner.getByRole("button", inviteButton)).toBeVisible();
  await owner.getByRole("link", { name: "Account" }).click();
  await expect(owner.getByRole("article").filter({ hasText: other.name })).toBeVisible();
  await expect(owner.getByRole("article").filter({ hasText: school.name })).toHaveCount(0);
  await owner.goto(`/schools/${school.slug}/settings#invite-staff`);
  await expect(owner.getByRole("heading", { name: "School access unavailable" })).toBeVisible();
  await expect(owner.getByRole("button", inviteButton)).toHaveCount(0);
  expect(secretInConsole.length).toBe(0);
  await platformContext.close();
});

test("fictional student and guardian activate accounts from profile invitations", async ({ browser, page: owner }) => {
  const suffix = randomBytes(4).toString("hex");
  const school = { slug: `browser-people-${suffix}`, name: "Fictional People School" };
  const ownerEmail = `people-owner-${suffix}@school.invalid`;
  const studentEmail = `learner-${suffix}@school.invalid`;
  const guardianEmail = `guardian-${suffix}@school.invalid`;
  const platformPassword = process.env.SWW_B12_ADMIN_PASSWORD;
  if (!platformPassword) throw new Error("Missing ephemeral fixture credential");
  const platformContext = await browser.newContext({ baseURL: origin });
  await signIn(await platformContext.newPage(), operator, platformPassword);
  const provision = await post(platformContext, "/api/v1/platform/tenants", {
    ...school, owner_email: ownerEmail, locale: "en-NG", timezone: "Africa/Lagos",
  });
  expect(provision.status()).toBe(201);
  await redeem(owner, await captured(platformContext, ownerEmail, school.name), randomBytes(24).toString("base64url"));

  const base = `/schools/${school.slug}`;
  const api = `/api/v1/tenants/${school.slug}`;
  await owner.goto(`${base}/students`);
  await owner.getByLabel("Admission number").fill("INVITE-E2E");
  await owner.getByLabel("Display name").fill("Fictional Invited Learner");
  await owner.getByRole("button", { name: "Save student" }).click();
  const link = owner.getByRole("link", { name: "Guardians for Fictional Invited Learner" });
  const guardianPage = await link.getAttribute("href");
  const studentID = new URL(guardianPage!, origin).searchParams.get("student_id");
  if (!studentID) throw new Error("Student route missing");
  await owner.getByRole("button", { name: "Invite Fictional Invited Learner" }).click();
  await owner.getByLabel("Student account email").fill(studentEmail);
  await owner.getByRole("button", { name: "Send account invitation" }).click();
  await expect(owner.getByRole("status").filter({ hasText: "Invitation sent." })).toBeVisible();

  await link.click();
  await owner.getByLabel("Guardian display name").fill("Fictional Invited Guardian");
  await owner.getByLabel("Contact email").fill(guardianEmail);
  await owner.getByRole("button", { name: "Save guardian profile" }).click();
  await owner.getByLabel("Guardian to link").selectOption({ label: `Fictional Invited Guardian — ${guardianEmail}` });
  await owner.getByLabel("Relationship").fill("Parent");
  await owner.getByRole("button", { name: "Save relationship" }).click();
  await owner.getByRole("button", { name: "Invite Fictional Invited Guardian" }).click();
  await owner.getByRole("button", { name: "Send account invitation" }).click();
  await expect(owner.getByRole("status").filter({ hasText: "Invitation sent." })).toBeVisible();

  for (const [email, role] of [[studentEmail, "STUDENT"], [guardianEmail, "PARENT"]]) {
    const password = randomBytes(24).toString("base64url");
    const invitation = await captured(platformContext, email, school.name);
    const activation = await browser.newContext({ baseURL: origin });
    await redeem(await activation.newPage(), invitation, password);
    await activation.close();
    const fresh = await browser.newContext({ baseURL: origin });
    await signIn(await fresh.newPage(), email, password);
    const schools = await fresh.request.get("/api/v1/my/schools");
    expect(schools.status()).toBe(200);
    const schoolsPayload = await schools.json();
    expect(JSON.stringify(schoolsPayload)).toContain(role);
    const personalRecord = await fresh.request.get(`${api}/students/${studentID}`);
    expect(personalRecord.status()).toBe(200);
    expect(await personalRecord.text()).toContain("Fictional Invited Learner");
    expect((await fresh.request.get(`${api}/students`)).status()).toBe(404);
    const replay = await withSafeFixtureDiagnostics(() => fresh.request.post("/api/v1/invitations/accept", {
      data: { token: invitation.split("/").at(-1), full_name: "Fictional Replay", password },
    }));
    expect(replay.status()).toBe(422);
    if (role === "PARENT") {
      await owner.getByRole("button", { name: "Unlink Fictional Invited Guardian" }).click();
      await owner.getByRole("button", { name: "Confirm unlink" }).click();
      await expect(owner.getByText("Guardian unlinked.")).toBeVisible();
      expect((await fresh.request.get(`${api}/students/${studentID}`)).status()).toBe(404);
    }
    await fresh.close();
  }
});
