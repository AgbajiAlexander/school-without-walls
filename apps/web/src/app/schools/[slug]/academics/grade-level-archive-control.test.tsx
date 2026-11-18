import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import GradeLevelArchiveControl from "./grade-level-archive-control";
import {
  archiveGradeLevel,
  type GradeLevel,
} from "@/lib/structure-client";

vi.mock("@/lib/structure-client", () => ({
  archiveGradeLevel: vi.fn(),
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

describe("GradeLevelArchiveControl", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("requires confirmation before archiving", async () => {
    const user = userEvent.setup();

    render(
      <GradeLevelArchiveControl
        schoolSlug="demo-school"
        level={level}
        onArchived={vi.fn()}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Archive Primary 3" }),
    );

    expect(archiveGradeLevel).not.toHaveBeenCalled();

    await user.click(
      screen.getByRole("button", { name: "Cancel" }),
    );

    expect(archiveGradeLevel).not.toHaveBeenCalled();
  });

  it("archives within the correct school after confirmation", async () => {
    const user = userEvent.setup();
    const onArchived = vi.fn();
    const archived = { ...level, status: "ARCHIVED" as const };

    vi.mocked(archiveGradeLevel).mockResolvedValue(archived);

    render(
      <GradeLevelArchiveControl
        schoolSlug="demo-school"
        level={level}
        onArchived={onArchived}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Archive Primary 3" }),
    );

    await user.click(
      screen.getByRole("button", { name: "Confirm archive" }),
    );

    await waitFor(() => {
      expect(archiveGradeLevel).toHaveBeenCalledWith(
        "demo-school",
        "level-1",
      );
      expect(onArchived).toHaveBeenCalledWith(archived);
    });
  });

  it("does not report success when the API rejects archiving", async () => {
    const user = userEvent.setup();
    const onArchived = vi.fn();

    vi.mocked(archiveGradeLevel).mockRejectedValue(
      new Error("Grade level is still in use"),
    );

    render(
      <GradeLevelArchiveControl
        schoolSlug="demo-school"
        level={level}
        onArchived={onArchived}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Archive Primary 3" }),
    );

    await user.click(
      screen.getByRole("button", { name: "Confirm archive" }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("Grade level is still in use");

    expect(onArchived).not.toHaveBeenCalled();
  });
});