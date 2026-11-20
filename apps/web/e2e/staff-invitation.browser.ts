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
  await signIn(platform, operator, platformPassword);

  const secretInConsole: boolean[] = [];
  const protectedValues = new Set([platformPassword, ownerPassword]);
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
  await expect(inviteAlert(owner)).toContainText("do not have permission");

  await owner.route(`**${endpoint}`, (route) => route.abort(), { times: 1 });
  await owner.getByRole("button", inviteButton).click();
  await expect(inviteAlert(owner)).toContainText("delivery could not be confirmed");

  expect((await post(owner.context(), "/api/v1/auth/logout", {})).status()).toBe(200);
  await owner.getByRole("button", inviteButton).click();
  await expect(inviteAlert(owner)).toContainText("session has expired");
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
  expect(secretInConsole.length).toBe(0);
  await platformContext.close();
});
