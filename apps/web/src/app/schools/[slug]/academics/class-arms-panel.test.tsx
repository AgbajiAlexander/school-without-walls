import {
  cleanup,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ClassArmsPanel from "./class-arms-panel";
import {
  createClassArm,
  listClassArms,
  updateClassArm,
  archiveClassArm,
  type SchoolClass,
  type ClassArm,
} from "@/lib/structure-client";

vi.mock("@/lib/structure-client", () => ({
  createClassArm: vi.fn(),
  listClassArms: vi.fn(),
  updateClassArm: vi.fn(),
  archiveClassArm: vi.fn(),
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

const arm: ClassArm = {
  id: "arm-1",
  class_id: "class-1",
  code: "A",
  name: "Arm A",
  status: "ACTIVE",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("ClassArmsPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listClassArms).mockResolvedValue([]);
  });

  afterEach(() => cleanup());

  it("loads only the selected school's class arms", async () => {
    vi.mocked(listClassArms).mockResolvedValue([arm]);

    render(
      <ClassArmsPanel
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        canManage={false}
      />,
    );

    expect(await screen.findByText("Code: A · Status: ACTIVE"))
      .toBeInTheDocument();

    expect(listClassArms).toHaveBeenCalledWith(
      "demo-school",
      "class-1",
    );

    expect(
      screen.queryByRole("button", { name: "Create class arm" }),
    ).not.toBeInTheDocument();
  });

  it("creates a class arm under its parent class", async () => {
    const user = userEvent.setup();
    vi.mocked(createClassArm).mockResolvedValue(arm);

    render(
      <ClassArmsPanel
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        canManage={true}
      />,
    );

    await screen.findByText("No arms configured for this class.");

    await user.type(screen.getByLabelText("Arm code"), "a");
    await user.type(screen.getByLabelText("Arm name"), "Arm A");

    await user.click(
      screen.getByRole("button", { name: "Create class arm" }),
    );

    await waitFor(() => {
      expect(createClassArm).toHaveBeenCalledWith(
        "demo-school",
        {
          class_id: "class-1",
          code: "A",
          name: "Arm A",
        },
      );
    });

    expect(await screen.findByText("Class arm created successfully."))
      .toBeInTheDocument();

    expect(screen.getByText("Code: A · Status: ACTIVE"))
      .toBeInTheDocument();
  });

  it("shows an API error without reporting successful creation", async () => {
    const user = userEvent.setup();

    vi.mocked(createClassArm).mockRejectedValue(
      new Error("Duplicate arm code"),
    );

    render(
      <ClassArmsPanel
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        canManage={true}
      />,
    );

    await screen.findByText("No arms configured for this class.");

    await user.type(screen.getByLabelText("Arm code"), "A");
    await user.type(screen.getByLabelText("Arm name"), "Arm A");

    await user.click(
      screen.getByRole("button", { name: "Create class arm" }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("Duplicate arm code");

    expect(
      screen.queryByText("Class arm created successfully."),
    ).not.toBeInTheDocument();
  });

  it("does not allow creation under an archived class", async () => {
    render(
      <ClassArmsPanel
        schoolSlug="demo-school"
        schoolClass={{ ...schoolClass, status: "ARCHIVED" }}
        canManage={true}
      />,
    );

    await screen.findByText("No arms configured for this class.");

    expect(
      screen.queryByRole("button", { name: "Create class arm" }),
    ).not.toBeInTheDocument();
  });

  it("ignores a delayed list response from a previous class", async () => {
    let resolveOld!: (items: ClassArm[]) => void;

    vi.mocked(listClassArms)
      .mockImplementationOnce(
        () => new Promise<ClassArm[]>((resolve) => {
          resolveOld = resolve;
        }),
      )
      .mockResolvedValueOnce([
        {
          ...arm,
          id: "arm-2",
          class_id: "class-2",
          code: "B",
          name: "Arm B",
        },
      ]);

    const { rerender } = render(
      <ClassArmsPanel
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        canManage={false}
      />,
    );

    rerender(
      <ClassArmsPanel
        schoolSlug="demo-school"
        schoolClass={{
          ...schoolClass,
          id: "class-2",
          name: "Primary 4",
        }}
        canManage={false}
      />,
    );

    expect(await screen.findByText("Code: B · Status: ACTIVE"))
      .toBeInTheDocument();

    resolveOld([arm]);

    await waitFor(() => {
      expect(listClassArms).toHaveBeenCalledWith(
        "demo-school",
        "class-2",
      );
    });

    expect(
      screen.queryByText("Code: A · Status: ACTIVE"),
    ).not.toBeInTheDocument();
  });

  it("updates the displayed arm after editing", async () => {
    const user = userEvent.setup();

    vi.mocked(listClassArms).mockResolvedValue([arm]);
    vi.mocked(updateClassArm).mockResolvedValue({
      ...arm,
      code: "BLUE",
      name: "Blue",
    });

    render(
      <ClassArmsPanel
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        canManage={true}
      />,
    );

    await screen.findByText("Code: A · Status: ACTIVE");

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

    expect(
      await screen.findByText("Code: BLUE · Status: ACTIVE"),
    ).toBeInTheDocument();

    expect(updateClassArm).toHaveBeenCalledWith(
      "demo-school",
      "arm-1",
      { code: "BLUE", name: "Blue" },
    );
  });

  it("updates the displayed arm status after archiving", async () => {
    const user = userEvent.setup();

    vi.mocked(listClassArms).mockResolvedValue([arm]);
    vi.mocked(archiveClassArm).mockResolvedValue({
      ...arm,
      status: "ARCHIVED",
    });

    render(
      <ClassArmsPanel
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        canManage={true}
      />,
    );

    await screen.findByText("Code: A · Status: ACTIVE");

    await user.click(
      screen.getByRole("button", { name: "Archive arm Arm A" }),
    );

    await user.click(
      screen.getByRole("button", {
        name: "Confirm arm archive",
      }),
    );

    expect(
      await screen.findByText("Code: A · Status: ARCHIVED"),
    ).toBeInTheDocument();

    expect(archiveClassArm).toHaveBeenCalledWith(
      "demo-school",
      "arm-1",
    );

    expect(
      screen.queryByRole("button", { name: "Archive arm Arm A" }),
    ).not.toBeInTheDocument();
  });

  it("does not show delayed creation under a different parent class", async () => {
    const user = userEvent.setup();

    let resolveCreate!: (value: ClassArm) => void;

    vi.mocked(createClassArm).mockImplementation(
      () =>
        new Promise<ClassArm>((resolve) => {
          resolveCreate = resolve;
        }),
    );

    const { rerender } = render(
      <ClassArmsPanel
        schoolSlug="demo-school"
        schoolClass={schoolClass}
        canManage={true}
      />,
    );

    await screen.findByText("No arms configured for this class.");

    await user.type(screen.getByLabelText("Arm code"), "A");
    await user.type(screen.getByLabelText("Arm name"), "Arm A");

    await user.click(
      screen.getByRole("button", { name: "Create class arm" }),
    );

    expect(createClassArm).toHaveBeenCalledWith(
      "demo-school",
      {
        class_id: "class-1",
        code: "A",
        name: "Arm A",
      },
    );

    rerender(
      <ClassArmsPanel
        schoolSlug="demo-school"
        schoolClass={{
          ...schoolClass,
          id: "class-2",
          name: "Primary 4",
        }}
        canManage={true}
      />,
    );

    await waitFor(() => {
      expect(listClassArms).toHaveBeenCalledWith(
        "demo-school",
        "class-2",
      );
    });

    resolveCreate(arm);

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Create class arm" }),
      ).not.toBeDisabled();
    });

    expect(
      screen.queryByText("Class arm created successfully."),
    ).not.toBeInTheDocument();

    expect(
      screen.queryByText("Code: A · Status: ACTIVE"),
    ).not.toBeInTheDocument();
  });
});