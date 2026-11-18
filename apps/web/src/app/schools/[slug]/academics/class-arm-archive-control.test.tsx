import {
  cleanup,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ClassArmArchiveControl from "./class-arm-archive-control";
import {
  archiveClassArm,
  type ClassArm,
} from "@/lib/structure-client";

vi.mock("@/lib/structure-client", () => ({
  archiveClassArm: vi.fn(),
}));

const arm: ClassArm = {
  id: "arm-1",
  class_id: "class-1",
  code: "A",
  name: "Arm A",
  status: "ACTIVE",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("ClassArmArchiveControl", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => cleanup());

  it("requires confirmation and supports cancellation", async () => {
    const user = userEvent.setup();

    render(
      <ClassArmArchiveControl
        schoolSlug="demo-school"
        arm={arm}
        onArchived={vi.fn()}
      />,
    );

    await user.click(
      screen.getByRole("button", {
        name: "Archive arm Arm A",
      }),
    );

    expect(archiveClassArm).not.toHaveBeenCalled();

    await user.click(
      screen.getByRole("button", {
        name: "Cancel arm archive",
      }),
    );

    expect(archiveClassArm).not.toHaveBeenCalled();

    expect(
      screen.queryByRole("button", {
        name: "Confirm arm archive",
      }),
    ).not.toBeInTheDocument();
  });

  it("archives the selected arm within its school", async () => {
    const user = userEvent.setup();
    const onArchived = vi.fn();

    const archived: ClassArm = {
      ...arm,
      status: "ARCHIVED",
    };

    vi.mocked(archiveClassArm).mockResolvedValue(archived);

    render(
      <ClassArmArchiveControl
        schoolSlug="demo-school"
        arm={arm}
        onArchived={onArchived}
      />,
    );

    await user.click(
      screen.getByRole("button", {
        name: "Archive arm Arm A",
      }),
    );

    await user.click(
      screen.getByRole("button", {
        name: "Confirm arm archive",
      }),
    );

    await waitFor(() => {
      expect(archiveClassArm).toHaveBeenCalledWith(
        "demo-school",
        "arm-1",
      );

      expect(onArchived).toHaveBeenCalledWith(archived);
    });
  });

  it("does not report success when the API fails", async () => {
    const user = userEvent.setup();
    const onArchived = vi.fn();

    vi.mocked(archiveClassArm).mockRejectedValue(
      new Error("Class arm archive forbidden"),
    );

    render(
      <ClassArmArchiveControl
        schoolSlug="demo-school"
        arm={arm}
        onArchived={onArchived}
      />,
    );

    await user.click(
      screen.getByRole("button", {
        name: "Archive arm Arm A",
      }),
    );

    await user.click(
      screen.getByRole("button", {
        name: "Confirm arm archive",
      }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("Class arm archive forbidden");

    expect(onArchived).not.toHaveBeenCalled();
  });

  it("rejects a response belonging to another class", async () => {
    const user = userEvent.setup();
    const onArchived = vi.fn();

    vi.mocked(archiveClassArm).mockResolvedValue({
      ...arm,
      status: "ARCHIVED",
      class_id: "class-2",
    });

    render(
      <ClassArmArchiveControl
        schoolSlug="demo-school"
        arm={arm}
        onArchived={onArchived}
      />,
    );

    await user.click(
      screen.getByRole("button", {
        name: "Archive arm Arm A",
      }),
    );

    await user.click(
      screen.getByRole("button", {
        name: "Confirm arm archive",
      }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent(
        "The server did not confirm class arm archiving",
      );

    expect(onArchived).not.toHaveBeenCalled();
  });

  it("does not offer archiving for archived arms", () => {
    render(
      <ClassArmArchiveControl
        schoolSlug="demo-school"
        arm={{ ...arm, status: "ARCHIVED" }}
        onArchived={vi.fn()}
      />,
    );

    expect(
      screen.queryByRole("button", {
        name: "Archive arm Arm A",
      }),
    ).not.toBeInTheDocument();
  });
});