import {
  cleanup,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import GradeLevelsPanel from "./grade-levels-panel";
import {
  createGradeLevel,
  listGradeLevels,
  updateGradeLevel,
  archiveGradeLevel,
  type GradeLevel,
} from "@/lib/structure-client";

vi.mock("@/lib/structure-client", () => ({
  createGradeLevel: vi.fn(),
  listGradeLevels: vi.fn(),
  updateGradeLevel: vi.fn(),
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

describe("GradeLevelsPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listGradeLevels).mockResolvedValue([]);
  });

  afterEach(() => {
    cleanup();
  });

  it("shows an empty state for a school without grade levels", async () => {
    render(
      <GradeLevelsPanel schoolSlug="demo-school" canManage={false} />,
    );

    expect(
      await screen.findByText(
        "No grade levels configured for this school.",
      ),
    ).toBeInTheDocument();

    expect(listGradeLevels).toHaveBeenCalledWith("demo-school");
  });

  it("allows read-only users to view levels but not create them", async () => {
    vi.mocked(listGradeLevels).mockResolvedValue([level]);

    render(
      <GradeLevelsPanel schoolSlug="demo-school" canManage={false} />,
    );

    expect(await screen.findByText("Primary 3"))
      .toBeInTheDocument();

    expect(
      screen.queryByRole("button", { name: "Create grade level" }),
    ).not.toBeInTheDocument();
  });

  it("creates a grade level with normalized code", async () => {
    const user = userEvent.setup();

    vi.mocked(createGradeLevel).mockResolvedValue(level);

    render(
      <GradeLevelsPanel schoolSlug="demo-school" canManage={true} />,
    );

    await screen.findByText(
      "No grade levels configured for this school.",
    );

    await user.type(
      screen.getByLabelText("Grade level code"),
      "pri_3",
    );
    await user.type(
      screen.getByLabelText("Grade level name"),
      "Primary 3",
    );
    await user.clear(screen.getByLabelText("Display order"));
    await user.type(screen.getByLabelText("Display order"), "30");

    await user.click(
      screen.getByRole("button", { name: "Create grade level" }),
    );

    await waitFor(() => {
      expect(createGradeLevel).toHaveBeenCalledWith("demo-school", {
        stage: "PRIMARY",
        code: "PRI_3",
        name: "Primary 3",
        sort_order: 30,
      });
    });

    expect(
      await screen.findByText("Grade level created successfully."),
    ).toBeInTheDocument();
  });

  it("shows the API error without claiming success", async () => {
    const user = userEvent.setup();

    vi.mocked(createGradeLevel).mockRejectedValue(
      new Error("Duplicate grade level code"),
    );

    render(
      <GradeLevelsPanel schoolSlug="demo-school" canManage={true} />,
    );

    await screen.findByText(
      "No grade levels configured for this school.",
    );

    await user.type(
      screen.getByLabelText("Grade level code"),
      "PRI_3",
    );
    await user.type(
      screen.getByLabelText("Grade level name"),
      "Primary 3",
    );

    await user.click(
      screen.getByRole("button", { name: "Create grade level" }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("Duplicate grade level code");

    expect(
      screen.queryByText("Grade level created successfully."),
    ).not.toBeInTheDocument();
  });

  it("retries a failed grade level listing", async () => {
    const user = userEvent.setup();

    vi.mocked(listGradeLevels)
      .mockRejectedValueOnce(new Error("Service unavailable"))
      .mockResolvedValueOnce([level]);

    render(
      <GradeLevelsPanel schoolSlug="demo-school" canManage={false} />,
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("Service unavailable");

    await user.click(
      screen.getByRole("button", { name: "Retry grade levels" }),
    );

    expect(await screen.findByText("Primary 3"))
      .toBeInTheDocument();

    expect(listGradeLevels).toHaveBeenCalledTimes(2);
  });

  it("updates the visible list after editing a grade level", async () => {
    const user = userEvent.setup();

    vi.mocked(listGradeLevels).mockResolvedValue([level]);
    vi.mocked(updateGradeLevel).mockResolvedValue({
      ...level,
      name: "Primary Three",
    });

    render(
      <GradeLevelsPanel schoolSlug="demo-school" canManage={true} />,
    );

    await screen.findByText("Primary 3");

    await user.click(
      screen.getByRole("button", { name: "Edit Primary 3" }),
    );

    const nameInput = screen.getByLabelText("Edit grade level name");

    await user.clear(nameInput);
    await user.type(nameInput, "Primary Three");

    await user.click(
      screen.getByRole("button", { name: "Save grade level" }),
    );

    expect(await screen.findByText("Primary Three"))
      .toBeInTheDocument();

    expect(updateGradeLevel).toHaveBeenCalledWith(
      "demo-school",
      "level-1",
      expect.objectContaining({ name: "Primary Three" }),
    );
  });

  it("updates the visible status after archiving a level", async () => {
    const user = userEvent.setup();

    vi.mocked(listGradeLevels).mockResolvedValue([level]);
    vi.mocked(archiveGradeLevel).mockResolvedValue({
      ...level,
      status: "ARCHIVED",
    });

    render(
      <GradeLevelsPanel schoolSlug="demo-school" canManage={true} />,
    );

    await screen.findByText("Primary 3");

    await user.click(
      screen.getByRole("button", { name: "Archive Primary 3" }),
    );

    await user.click(
      screen.getByRole("button", { name: "Confirm archive" }),
    );

    expect(await screen.findByText("ARCHIVED"))
      .toBeInTheDocument();

    expect(archiveGradeLevel).toHaveBeenCalledWith(
      "demo-school",
      "level-1",
    );

    expect(
      screen.queryByRole("button", { name: "Archive Primary 3" }),
    ).not.toBeInTheDocument();
  });
});