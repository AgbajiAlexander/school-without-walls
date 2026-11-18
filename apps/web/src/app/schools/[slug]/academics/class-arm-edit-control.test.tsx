import {
  cleanup,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ClassArmEditControl from "./class-arm-edit-control";
import {
  updateClassArm,
  type ClassArm,
} from "@/lib/structure-client";

vi.mock("@/lib/structure-client", () => ({
  updateClassArm: vi.fn(),
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

describe("ClassArmEditControl", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => cleanup());

  it("cancels editing without calling the API", async () => {
    const user = userEvent.setup();

    render(
      <ClassArmEditControl
        schoolSlug="demo-school"
        arm={arm}
        onUpdated={vi.fn()}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Edit arm Arm A" }),
    );

    await user.click(
      screen.getByRole("button", { name: "Cancel arm editing" }),
    );

    expect(updateClassArm).not.toHaveBeenCalled();
  });

  it("updates an arm within the correct school", async () => {
    const user = userEvent.setup();
    const onUpdated = vi.fn();
    const updated = { ...arm, code: "BLUE", name: "Blue" };

    vi.mocked(updateClassArm).mockResolvedValue(updated);

    render(
      <ClassArmEditControl
        schoolSlug="demo-school"
        arm={arm}
        onUpdated={onUpdated}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Edit arm Arm A" }),
    );

    await user.clear(screen.getByLabelText("Edit arm code"));
    await user.type(screen.getByLabelText("Edit arm code"), "blue");

    await user.clear(screen.getByLabelText("Edit arm name"));
    await user.type(screen.getByLabelText("Edit arm name"), "Blue");

    await user.click(
      screen.getByRole("button", { name: "Save arm" }),
    );

    await waitFor(() => {
      expect(updateClassArm).toHaveBeenCalledWith(
        "demo-school",
        "arm-1",
        {
          code: "BLUE",
          name: "Blue",
        },
      );

      expect(onUpdated).toHaveBeenCalledWith(updated);
    });
  });

  it("shows backend errors without claiming success", async () => {
    const user = userEvent.setup();
    const onUpdated = vi.fn();

    vi.mocked(updateClassArm).mockRejectedValue(
      new Error("Class arm update forbidden"),
    );

    render(
      <ClassArmEditControl
        schoolSlug="demo-school"
        arm={arm}
        onUpdated={onUpdated}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Edit arm Arm A" }),
    );

    await user.click(
      screen.getByRole("button", { name: "Save arm" }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("Class arm update forbidden");

    expect(onUpdated).not.toHaveBeenCalled();
  });

  it("does not offer editing for archived arms", () => {
    render(
      <ClassArmEditControl
        schoolSlug="demo-school"
        arm={{ ...arm, status: "ARCHIVED" }}
        onUpdated={vi.fn()}
      />,
    );

    expect(
      screen.queryByRole("button", { name: "Edit arm Arm A" }),
    ).not.toBeInTheDocument();
  });

  it("rejects an update returned for a different parent class", async () => {
    const user = userEvent.setup();
    const onUpdated = vi.fn();

    vi.mocked(updateClassArm).mockResolvedValue({
      ...arm,
      class_id: "class-2",
    });

    render(
      <ClassArmEditControl
        schoolSlug="demo-school"
        arm={arm}
        onUpdated={onUpdated}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Edit arm Arm A" }),
    );

    await user.click(
      screen.getByRole("button", { name: "Save arm" }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent(
        "The server did not confirm the class arm update",
      );

    expect(onUpdated).not.toHaveBeenCalled();
  });
});