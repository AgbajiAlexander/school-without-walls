import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { acceptInvitation, verifyInvitation } from "@/lib/auth-client";
import AcceptInvitationPage from "./page";

const { replace } = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace }) }));
vi.mock("@/lib/auth-client", () => ({
  acceptInvitation: vi.fn(),
  verifyInvitation: vi.fn(),
}));

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

it("shows role-neutral school information and replaces the redeemed link", async () => {
  const user = userEvent.setup();
  const token = "fictional-invitation-token";
  const password = "fictional onboarding passphrase";
  vi.mocked(verifyInvitation).mockResolvedValue({
    invited_email: "fictional@school.invalid",
    expires_at: "2099-01-01T00:00:00Z",
    tenant_slug: "test-school",
    tenant_name: "Fictional School",
  });
  vi.mocked(acceptInvitation).mockResolvedValue({
    user: { id: "fictional-id", email: "fictional@school.invalid", full_name: "Fictional Staff", status: "ACTIVE", created_at: "2026-01-01T00:00:00Z" },
    csrf_token: "fictional-csrf",
  });
  await act(async () => {
    render(<AcceptInvitationPage params={Promise.resolve({ token })} />);
  });

  await screen.findByText("Fictional School");
  expect(screen.queryByText(/as its school owner/i)).toBeNull();
  await user.type(screen.getByLabelText("Full Name"), "Fictional Staff");
  await user.type(screen.getByLabelText("Create or confirm password"), password);
  await user.type(screen.getByLabelText("Confirm Password", { exact: true }), password);
  await user.click(screen.getByRole("button", { name: "Accept Invitation & Sign In" }));

  expect(acceptInvitation).toHaveBeenCalledWith(token, "Fictional Staff", password);
  await waitFor(() => expect(replace).toHaveBeenCalledWith("/schools/test-school/settings"));
});

it("shows an expired or invalid link without rendering the acceptance form", async () => {
  vi.mocked(verifyInvitation).mockRejectedValue(new Error("Invitation token is invalid or expired"));
  await act(async () => {
    render(<AcceptInvitationPage params={Promise.resolve({ token: "expired-test-token" })} />);
  });

  expect(await screen.findByRole("heading", { name: "Invitation Expired or Invalid" })).toBeVisible();
  expect(screen.queryByRole("button", { name: "Accept Invitation & Sign In" })).toBeNull();
  expect(acceptInvitation).not.toHaveBeenCalled();
});
