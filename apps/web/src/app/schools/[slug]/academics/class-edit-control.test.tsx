import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ClassEditControl from "./class-edit-control";
import {
  updateClass,
  type SchoolClass,
} from "@/lib/structure-client";

vi.mock("@/lib/structure-client", () => ({
  updateClass: vi.fn(),
}));

const schoolClass: SchoolClass = {
  id: "class-1",
  session_id: "session-1",
  grade_level_id: "level-1",
  code: "PRI_3_2026",
  name: "Primary 3",
  status: "ACTIVE",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("ClassEditControl", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => cleanup());

  it("cancels editing without calling the API", async () => {
    const user = userEvent.setup();

    render(
      <ClassEditControl
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        onUpdated={vi.fn()}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Edit class Primary 3" }),
    );

    await user.click(
      screen.getByRole("button", { name: "Cancel class editing" }),
    );

    expect(updateClass).not.toHaveBeenCalled();
  });

  it("updates only the code and name within the school", async () => {
    const user = userEvent.setup();
    const onUpdated = vi.fn();
    const updated = { ...schoolClass, name: "Primary Three" };

    vi.mocked(updateClass).mockResolvedValue(updated);

    render(
      <ClassEditControl
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        onUpdated={onUpdated}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Edit class Primary 3" }),
    );

    await user.clear(screen.getByLabelText("Edit class name"));
    await user.type(
      screen.getByLabelText("Edit class name"),
      "Primary Three",
    );

    await user.click(
      screen.getByRole("button", { name: "Save class" }),
    );

    await waitFor(() => {
      expect(updateClass).toHaveBeenCalledWith(
        "demo-school",
        "class-1",
        {
          code: "PRI_3_2026",
          name: "Primary Three",
        },
      );

      expect(onUpdated).toHaveBeenCalledWith(updated);
    });
  });

  it("shows backend failures without reporting a successful update", async () => {
    const user = userEvent.setup();
    const onUpdated = vi.fn();

    vi.mocked(updateClass).mockRejectedValue(
      new Error("Class update not permitted"),
    );

    render(
      <ClassEditControl
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        onUpdated={onUpdated}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Edit class Primary 3" }),
    );

    await user.click(
      screen.getByRole("button", { name: "Save class" }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("Class update not permitted");

    expect(onUpdated).not.toHaveBeenCalled();
  });

  it("does not offer editing for archived classes", () => {
    render(
      <ClassEditControl
        schoolSlug="demo-school"
        schoolClass={{ ...schoolClass, status: "ARCHIVED" }}
        onUpdated={vi.fn()}
      />,
    );

    expect(
      screen.queryByRole("button", { name: "Edit class Primary 3" }),
    ).not.toBeInTheDocument();
  });

  it("rejects a response with a different academic session", async () => {
    const user = userEvent.setup();
    const onUpdated = vi.fn();

    vi.mocked(updateClass).mockResolvedValue({
      ...schoolClass,
      session_id: "another-session",
    });

    render(
      <ClassEditControl
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        onUpdated={onUpdated}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Edit class Primary 3" }),
    );

    await user.click(
      screen.getByRole("button", { name: "Save class" }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("The server did not confirm the class update");

    expect(onUpdated).not.toHaveBeenCalled();
  });
});