import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import GradeLevelEditControl from "./grade-level-edit-control";
import {
  updateGradeLevel,
  type GradeLevel,
} from "@/lib/structure-client";

vi.mock("@/lib/structure-client", () => ({
  updateGradeLevel: vi.fn(),
}));

const level: GradeLevel = {
  id: "level-1",
  stage: "PRIMARY",
  code: "PRI_3",
  name: "Primary 3",
  sort_order: 30,
  status: "ACTIVE",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("GradeLevelEditControl", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => cleanup());

  it("does not update a grade level when editing is cancelled", async () => {
    const user = userEvent.setup();

    render(
      <GradeLevelEditControl
        schoolSlug="demo-school"
        level={level}
        onUpdated={vi.fn()}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Edit Primary 3" }),
    );

    await user.click(
      screen.getByRole("button", { name: "Cancel editing" }),
    );

    expect(updateGradeLevel).not.toHaveBeenCalled();
  });

  it("saves a school-scoped grade level update", async () => {
    const user = userEvent.setup();
    const onUpdated = vi.fn();
    const updated = { ...level, name: "Primary Three" };

    vi.mocked(updateGradeLevel).mockResolvedValue(updated);

    render(
      <GradeLevelEditControl
        schoolSlug="demo-school"
        level={level}
        onUpdated={onUpdated}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Edit Primary 3" }),
    );

    await user.clear(
      screen.getByLabelText("Edit grade level name"),
    );

    await user.type(
      screen.getByLabelText("Edit grade level name"),
      "Primary Three",
    );

    await user.click(
      screen.getByRole("button", { name: "Save grade level" }),
    );

    await waitFor(() => {
      expect(updateGradeLevel).toHaveBeenCalledWith(
        "demo-school",
        "level-1",
        {
          stage: "PRIMARY",
          code: "PRI_3",
          name: "Primary Three",
          sort_order: 30,
        },
      );
    });

    expect(onUpdated).toHaveBeenCalledWith(updated);
  });

  it("displays API errors without updating the list", async () => {
    const user = userEvent.setup();
    const onUpdated = vi.fn();

    vi.mocked(updateGradeLevel).mockRejectedValue(
      new Error("Grade level update not permitted"),
    );

    render(
      <GradeLevelEditControl
        schoolSlug="demo-school"
        level={level}
        onUpdated={onUpdated}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Edit Primary 3" }),
    );

    await user.click(
      screen.getByRole("button", { name: "Save grade level" }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("Grade level update not permitted");

    expect(onUpdated).not.toHaveBeenCalled();
  });

  it("does not show editing for archived levels", () => {
    render(
      <GradeLevelEditControl
        schoolSlug="demo-school"
        level={{ ...level, status: "ARCHIVED" }}
        onUpdated={vi.fn()}
      />,
    );

    expect(
      screen.queryByRole("button", { name: "Edit Primary 3" }),
    ).not.toBeInTheDocument();
  });
});