import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import MySchoolsPanel from "@/app/dashboard/my-schools-panel";
import { setCSRFToken } from "@/lib/auth-client";
import type { Tenant } from "@/lib/tenant-client";
import SettingsClient from "./settings-client";

vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn() }) }));

const ownerSchool: Tenant = {
  id: "school-a", slug: "school-a", name: "Fictional School A", status: "ACTIVE",
  locale: "en-NG", timezone: "Africa/Lagos", branding: {}, education_stages: ["PRIMARY"],
  current_role: "SCHOOL_OWNER", created_at: "", updated_at: "",
};
const email = "staff@school.invalid";
const receipt = { data: { id: "invitation-id", email, role: "PRINCIPAL", expires_at: "2099-01-01T00:00:00Z" } };

afterEach(() => {
  cleanup();
  setCSRFToken(null);
  vi.unstubAllGlobals();
});

function mockFetch() {
  const fetchMock = vi.fn<typeof fetch>();
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

async function fillInvitation() {
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("Staff email address"), email);
  await user.selectOptions(screen.getByLabelText("Staff role"), "PRINCIPAL");
  return user;
}

it("sends the selected tenant and role with the verified CSRF token and waits for a delivery receipt", async () => {
  const fetchMock = mockFetch();
  let complete!: (value: Response) => void;
  fetchMock.mockImplementation(() => new Promise((resolve) => { complete = resolve; }));
  render(<SettingsClient tenant={ownerSchool} csrfToken="fictional-csrf" />);
  const user = await fillInvitation();
  await user.click(screen.getByRole("button", { name: "Send invitation" }));

  expect(screen.getByRole("button", { name: "Sending invitation…" })).toBeDisabled();
  expect(screen.queryByText("Invitation sent.")).toBeNull();
  expect(fetchMock).toHaveBeenCalledTimes(1);
  const [url, options] = fetchMock.mock.calls[0];
  expect(url).toBe("/api/v1/tenants/school-a/staff-invitations");
  expect(options).toMatchObject({ method: "POST", credentials: "same-origin" });
  expect(new Headers(options?.headers).get("X-CSRF-Token")).toBe("fictional-csrf");
  expect(JSON.parse(String(options?.body))).toEqual({ email, role: "PRINCIPAL" });

  await act(async () => complete(Response.json(receipt, { status: 201 })));
  expect(screen.getByRole("status")).toHaveTextContent("Invitation sent.");
  expect(screen.getByLabelText("Staff email address")).toHaveValue("");
});

it("focuses an invalid email and does not send a request", async () => {
  const fetchMock = mockFetch();
  render(<SettingsClient tenant={ownerSchool} csrfToken="fictional-csrf" />);
  const user = userEvent.setup();
  const input = screen.getByLabelText("Staff email address");
  await user.type(input, "not-an-email");
  await user.click(screen.getByRole("button", { name: "Send invitation" }));
  expect(screen.getByRole("alert")).toHaveTextContent("Enter a valid email address.");
  expect(input).toHaveFocus();
  expect(fetchMock).not.toHaveBeenCalled();
});

it("offers only the two staff roles and rejects a tampered role", async () => {
  const fetchMock = mockFetch();
  render(<SettingsClient tenant={ownerSchool} csrfToken="fictional-csrf" />);
  const user = await fillInvitation();
  const role = screen.getByRole<HTMLSelectElement>("combobox", { name: "Staff role" });
  expect(within(role).getAllByRole("option").map((option) => option.getAttribute("value")))
    .toEqual(["", "SCHOOL_ADMIN", "PRINCIPAL"]);
  role.add(new Option("Injected owner", "SCHOOL_OWNER"));
  fireEvent.change(role, { target: { value: "SCHOOL_OWNER" } });
  await user.click(screen.getByRole("button", { name: "Send invitation" }));
  expect(screen.getByRole("alert")).toHaveTextContent("Choose School Administrator or Principal.");
  expect(fetchMock).not.toHaveBeenCalled();
});

it.each([
  [409, "already has a pending invitation or membership"],
  [401, "Your session has expired"],
  [403, "You do not have permission"],
  [422, "Check the email address and role"],
  [500, "delivery could not be confirmed"],
  [200, "delivery could not be confirmed"],
])("handles status %i without exposing response details", async (status, message) => {
  mockFetch().mockResolvedValue(Response.json({
    ...receipt, error: { message: "private server detail", details: { token: "private-token" } },
  }, { status }));
  render(<SettingsClient tenant={ownerSchool} csrfToken="fictional-csrf" />);
  const user = await fillInvitation();
  await user.click(screen.getByRole("button", { name: "Send invitation" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(message);
  expect(screen.queryByText(/private server detail|private-token|Invitation sent/)).toBeNull();
});

it("shows unconfirmed delivery after a network error and keeps the form available", async () => {
  mockFetch().mockRejectedValue(new Error("private network detail"));
  render(<SettingsClient tenant={ownerSchool} csrfToken="fictional-csrf" />);
  const user = await fillInvitation();
  await user.click(screen.getByRole("button", { name: "Send invitation" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("delivery could not be confirmed");
  expect(screen.queryByText(/private network detail/)).toBeNull();
  expect(screen.getByRole("button", { name: "Send invitation" })).toBeEnabled();
});

it("shows the dashboard action only for an active owner and hides the form for non-owners", async () => {
  const staffSchool: Tenant = { ...ownerSchool, id: "school-b", slug: "school-b", name: "Fictional School B", current_role: "PRINCIPAL" };
  const suspendedSchool: Tenant = { ...ownerSchool, id: "school-c", slug: "school-c", name: "Fictional School C", status: "SUSPENDED" };
  mockFetch().mockResolvedValue(Response.json({ data: { schools: [ownerSchool, staffSchool, suspendedSchool] } }));
  const { rerender } = render(<MySchoolsPanel />);
  const link = await screen.findByRole("link", { name: "Invite Staff" });
  expect(link).toHaveAttribute("href", "/schools/school-a/settings#invite-staff");
  expect(screen.getAllByRole("link", { name: "Invite Staff" })).toHaveLength(1);
  rerender(<SettingsClient tenant={staffSchool} csrfToken="fictional-csrf" />);
  expect(screen.queryByRole("heading", { name: "Invite Staff" })).toBeNull();
  rerender(<SettingsClient tenant={suspendedSchool} csrfToken="fictional-csrf" />);
  expect(screen.queryByRole("heading", { name: "Invite Staff" })).toBeNull();
});

it("clears invitation state when changing schools and ignores a late receipt from the previous school", async () => {
  const fetchMock = mockFetch();
  let complete!: (value: Response) => void;
  fetchMock.mockImplementation(() => new Promise((resolve) => { complete = resolve; }));
  const { rerender } = render(<SettingsClient tenant={ownerSchool} csrfToken="fictional-csrf" />);
  const user = await fillInvitation();
  await user.click(screen.getByRole("button", { name: "Send invitation" }));
  const otherSchool = { ...ownerSchool, id: "school-b", slug: "school-b", name: "Fictional School B" };
  rerender(<SettingsClient tenant={otherSchool} csrfToken="fictional-csrf" />);
  await act(async () => complete(Response.json(receipt, { status: 201 })));
  expect(screen.getByLabelText("Staff email address")).toHaveValue("");
  expect(screen.getByLabelText("Staff role")).toHaveValue("");
  expect(screen.queryByText("Invitation sent.")).toBeNull();
  expect(fetchMock).toHaveBeenCalledTimes(1);
});
