import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import StaffClient from "./staff-client";

const refresh = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));

const props = {
  slug: "fictional-school",
  csrfToken: "fixture-csrf",
  staff: [{ id: "profile-a", display_name: "Fictional Editor", staff_code: "STAFF-1" }],
  candidates: [{ membership_id: "membership-b", display_name: "Fictional Candidate", role: "TEACHER", account_email: "candidate@example.test" }],
};

afterEach(() => {
  cleanup();
  refresh.mockClear();
  vi.unstubAllGlobals();
});

function fillNewProfile() {
  fireEvent.change(screen.getByLabelText("Staff membership"), { target: { value: "membership-b" } });
  fireEvent.change(screen.getByLabelText("Staff code"), { target: { value: " NEW-1 " } });
}

it("creates a profile for the selected membership with CSRF and refreshes", async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 201 }));
  vi.stubGlobal("fetch", fetcher);
  render(<StaffClient {...props} />);
  expect(screen.getByRole("button", { name: "Save staff profile" })).toBeDisabled();
  fillNewProfile();
  fireEvent.click(screen.getByRole("button", { name: "Save staff profile" }));
  expect(await screen.findByText("Staff profile saved.")).toBeVisible();
  expect(fetcher).toHaveBeenCalledExactlyOnceWith("/api/v1/tenants/fictional-school/staff", expect.objectContaining({
    method: "POST", credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": "fixture-csrf" },
    body: JSON.stringify({ display_name: "Fictional Candidate", staff_code: "NEW-1", membership_id: "membership-b" }),
  }));
  expect(refresh).toHaveBeenCalledOnce();
});

it("edits only presentation fields and reports conflicts without exposing diagnostics", async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response("internal fixture diagnostic", { status: 409 }));
  vi.stubGlobal("fetch", fetcher);
  render(<StaffClient {...props} />);
  fireEvent.click(screen.getByRole("button", { name: "Edit Fictional Editor" }));
  fireEvent.change(screen.getByLabelText("Staff code"), { target: { value: "REVISED" } });
  fireEvent.click(screen.getByRole("button", { name: "Save staff profile" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("already has a profile");
  expect(screen.queryByText("internal fixture diagnostic")).not.toBeInTheDocument();
  expect(fetcher).toHaveBeenCalledWith("/api/v1/tenants/fictional-school/staff/profile-a", expect.objectContaining({
    method: "PUT", body: JSON.stringify({ display_name: "Fictional Editor", staff_code: "REVISED" }),
  }));
  expect(refresh).not.toHaveBeenCalled();
});

it("blocks subsequent submissions after authorization is rejected", async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 403 }));
  vi.stubGlobal("fetch", fetcher);
  render(<StaffClient {...props} />);
  fillNewProfile();
  const save = screen.getByRole("button", { name: "Save staff profile" });
  fireEvent.click(save);
  expect(await screen.findByRole("alert")).toHaveTextContent("session or permissions changed");
  await waitFor(() => expect(save).toBeDisabled());
  fireEvent.click(save);
  expect(fetcher).toHaveBeenCalledOnce();
});

it("distinguishes same-name staff by account email", () => {
  render(<StaffClient {...props} candidates={[
    ...props.candidates,
    { ...props.candidates[0], membership_id: "membership-c", account_email: "other@example.test" },
  ]} />);
  expect(screen.getByRole("option", { name: /candidate@example.test/ })).toHaveValue("membership-b");
  expect(screen.getByRole("option", { name: /other@example.test/ })).toHaveValue("membership-c");
});
