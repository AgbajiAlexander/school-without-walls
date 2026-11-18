import {
  cleanup,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ClassArchiveControl from "./class-archive-control";
import {
  archiveClass,
  type SchoolClass,
} from "@/lib/structure-client";

vi.mock("@/lib/structure-client", () => ({
  archiveClass: vi.fn(),
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

describe("ClassArchiveControl", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => cleanup());

  it("requires confirmation and allows cancellation", async () => {
    const user = userEvent.setup();

    render(
      <ClassArchiveControl
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        onArchived={vi.fn()}
      />,
    );

    await user.click(
      screen.getByRole("button", {
        name: "Archive class Primary 3",
      }),
    );

    expect(archiveClass).not.toHaveBeenCalled();

    await user.click(
      screen.getByRole("button", {
        name: "Cancel class archive",
      }),
    );

    expect(archiveClass).not.toHaveBeenCalled();
  });

  it("archives the correct school class", async () => {
    const user = userEvent.setup();
    const onArchived = vi.fn();
    const archived: SchoolClass = {
      ...schoolClass,
      status: "ARCHIVED",
    };

    vi.mocked(archiveClass).mockResolvedValue(archived);

    render(
      <ClassArchiveControl
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        onArchived={onArchived}
      />,
    );

    await user.click(
      screen.getByRole("button", {
        name: "Archive class Primary 3",
      }),
    );

    await user.click(
      screen.getByRole("button", {
        name: "Confirm class archive",
      }),
    );

    await waitFor(() => {
      expect(archiveClass).toHaveBeenCalledWith(
        "demo-school",
        "class-1",
      );

      expect(onArchived).toHaveBeenCalledWith(archived);
    });
  });

  it("shows an API error without reporting success", async () => {
    const user = userEvent.setup();
    const onArchived = vi.fn();

    vi.mocked(archiveClass).mockRejectedValue(
      new Error("Class still has active dependencies"),
    );

    render(
      <ClassArchiveControl
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        onArchived={onArchived}
      />,
    );

    await user.click(
      screen.getByRole("button", {
        name: "Archive class Primary 3",
      }),
    );

    await user.click(
      screen.getByRole("button", {
        name: "Confirm class archive",
      }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("Class still has active dependencies");

    expect(onArchived).not.toHaveBeenCalled();
  });

  it("rejects a response with a different session", async () => {
    const user = userEvent.setup();
    const onArchived = vi.fn();

    vi.mocked(archiveClass).mockResolvedValue({
      ...schoolClass,
      status: "ARCHIVED",
      session_id: "another-session",
    });

    render(
      <ClassArchiveControl
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        onArchived={onArchived}
      />,
    );

    await user.click(
      screen.getByRole("button", {
        name: "Archive class Primary 3",
      }),
    );

    await user.click(
      screen.getByRole("button", {
        name: "Confirm class archive",
      }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("The server did not confirm class archiving");

    expect(onArchived).not.toHaveBeenCalled();
  });
});