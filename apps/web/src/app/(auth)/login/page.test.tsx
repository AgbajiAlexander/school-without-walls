import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import LoginPage from "./page";
import { login } from "@/lib/auth-client";

const { push } = vi.hoisted(() => ({
  push: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, replace: vi.fn(), refresh: vi.fn(), back: vi.fn(), forward: vi.fn(), prefetch: vi.fn() }),
}));

vi.mock("@/lib/auth-client", () => ({
  login: vi.fn(),
}));

describe("LoginPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("requires an email and password before submitting", async () => {
    const user = userEvent.setup();
    render(<LoginPage />);

    const email = screen.getByLabelText("Email Address");
    const password = screen.getByLabelText("Password");
    expect(email).toBeRequired();
    expect(password).toBeRequired();

    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(login).not.toHaveBeenCalled();
  });

  it("shows the invalid-credentials error", async () => {
    const user = userEvent.setup();
    vi.mocked(login).mockRejectedValue(new Error("Invalid email or password"));
    render(<LoginPage />);

    await user.type(screen.getByLabelText("Email Address"), "fictional@example.test");
    await user.type(screen.getByLabelText("Password"), "incorrect-password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Invalid email or password");
    expect(push).not.toHaveBeenCalled();
  });

  it("disables the submit button while login is pending", async () => {
    const user = userEvent.setup();
    let resolveLogin!: () => void;
    vi.mocked(login).mockReturnValue(new Promise((resolve) => {
      resolveLogin = () => resolve({
        user: {
          id: "00000000-0000-4000-8000-000000000001",
          email: "fictional@example.test",
          full_name: "Fictional Test User",
          status: "ACTIVE",
          created_at: "2026-01-01T00:00:00Z",
        },
        csrf_token: "test-csrf",
      });
    }));
    render(<LoginPage />);

    await user.type(screen.getByLabelText("Email Address"), "fictional@example.test");
    await user.type(screen.getByLabelText("Password"), "fictional-password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(screen.getByRole("button", { name: "Signing in..." })).toBeDisabled();
    resolveLogin();
    await waitFor(() => expect(push).toHaveBeenCalledWith("/dashboard"));
  });

  it("navigates to the protected landing after successful login", async () => {
    const user = userEvent.setup();
    vi.mocked(login).mockResolvedValue({
      user: {
        id: "00000000-0000-4000-8000-000000000001",
        email: "fictional@example.test",
        full_name: "Fictional Test User",
        status: "ACTIVE",
        created_at: "2026-01-01T00:00:00Z",
      },
      csrf_token: "test-csrf",
    });
    render(<LoginPage />);

    await user.type(screen.getByLabelText("Email Address"), "fictional@example.test");
    await user.type(screen.getByLabelText("Password"), "fictional-password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    await waitFor(() => expect(push).toHaveBeenCalledWith("/dashboard"));
  });
});
