import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import SessionCreateForm from "./session-create-form";
import { createAcademicSession } from "@/lib/academics-client";

vi.mock("@/lib/academics-client", () => ({
  createAcademicSession: vi.fn(),
}));

const createdSession = {
  id: "session-123",
  name: "2026/2027",
  starts_on: "2026-09-01",
  ends_on: "2027-07-31",
  status: "DRAFT" as const,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

async function fillForm(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText("Session name"), "2026/2027");
  await user.type(screen.getByLabelText("Start date"), "2026-09-01");
  await user.type(screen.getByLabelText("End date"), "2027-07-31");
}

describe("SessionCreateForm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("requires all session fields", async () => {
    const user = userEvent.setup();
    render(
      <SessionCreateForm schoolSlug="demo-school" onCreated={vi.fn()} />,
    );

    expect(screen.getByLabelText("Session name")).toBeRequired();
    expect(screen.getByLabelText("Start date")).toBeRequired();
    expect(screen.getByLabelText("End date")).toBeRequired();

    await user.click(
      screen.getByRole("button", { name: "Create draft session" }),
    );

    expect(createAcademicSession).not.toHaveBeenCalled();
  });

  it("rejects an end date before the start date", async () => {
    const user = userEvent.setup();
    render(
      <SessionCreateForm schoolSlug="demo-school" onCreated={vi.fn()} />,
    );

    await fillForm(user);
    await user.clear(screen.getByLabelText("End date"));
    await user.type(screen.getByLabelText("End date"), "2026-08-01");

    await user.click(
      screen.getByRole("button", { name: "Create draft session" }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("end date must be after the start date");
    expect(createAcademicSession).not.toHaveBeenCalled();
  });

  it("creates a draft session in the selected school", async () => {
    const user = userEvent.setup();
    const onCreated = vi.fn();

    vi.mocked(createAcademicSession).mockResolvedValue(createdSession);

    render(
      <SessionCreateForm schoolSlug="demo-school" onCreated={onCreated} />,
    );

    await fillForm(user);
    await user.click(
      screen.getByRole("button", { name: "Create draft session" }),
    );

    expect(await screen.findByRole("button", {
      name: "Create draft session",
    })).toBeInTheDocument();

    expect(createAcademicSession).toHaveBeenCalledWith(
      "demo-school",
      {
        name: "2026/2027",
        starts_on: "2026-09-01",
        ends_on: "2027-07-31",
      },
    );

    expect(onCreated).toHaveBeenCalledWith(createdSession);
  });

  it("shows a server error without reporting success", async () => {
    const user = userEvent.setup();
    const onCreated = vi.fn();

    vi.mocked(createAcademicSession)
      .mockRejectedValue(new Error("Session already exists"));

    render(
      <SessionCreateForm schoolSlug="demo-school" onCreated={onCreated} />,
    );

    await fillForm(user);
    await user.click(
      screen.getByRole("button", { name: "Create draft session" }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("Session already exists");

    expect(onCreated).not.toHaveBeenCalled();
  });
});