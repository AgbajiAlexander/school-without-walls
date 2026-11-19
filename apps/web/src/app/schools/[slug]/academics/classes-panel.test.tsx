import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ClassesPanel from "./classes-panel";
import { listAcademicSessions } from "@/lib/academics-client";
import {
  createClass,
  listClasses,
  listGradeLevels,
  updateClass,
  archiveClass,
  type SchoolClass,
} from "@/lib/structure-client";

vi.mock("./class-arms-panel", () => ({
  default: ({
    schoolSlug,
    schoolClass,
    canManage,
  }: {
    schoolSlug: string;
    schoolClass: { id: string };
    canManage: boolean;
  }) => (
    <div
      data-testid="class-arms-panel"
      data-school-slug={schoolSlug}
      data-class-id={schoolClass.id}
      data-can-manage={String(canManage)}
    />
  ),
}));
vi.mock("@/lib/academics-client", () => ({
  listAcademicSessions: vi.fn(),
}));

vi.mock("@/lib/structure-client", () => ({
  createClass: vi.fn(),
  listClasses: vi.fn(),
  listGradeLevels: vi.fn(),
  updateClass: vi.fn(),
  archiveClass: vi.fn(),
}));

const session = {
  id: "session-1",
  name: "2026/2027",
  starts_on: "2026-09-01",
  ends_on: "2027-07-31",
  status: "ACTIVE" as const,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const level = {
  id: "level-1",
  stage: "PRIMARY" as const,
  code: "PRI_3",
  name: "Primary 3",
  sort_order: 30,
  status: "ACTIVE" as const,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const schoolClass = {
  id: "class-1",
  session_id: "session-1",
  grade_level_id: "level-1",
  code: "PRI_3_2026",
  name: "Primary 3",
  status: "ACTIVE" as const,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("ClassesPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listAcademicSessions).mockResolvedValue([session]);
    vi.mocked(listGradeLevels).mockResolvedValue([level]);
    vi.mocked(listClasses).mockResolvedValue([]);
  });

  afterEach(() => cleanup());

  it("reloads grade levels when notified of a change", async () => {
    const { rerender } = render(
      <ClassesPanel
        schoolSlug="demo-school"
        canManage={true}
        gradeLevelsVersion={0}
      />,
    );

    await waitFor(() => {
      expect(listGradeLevels).toHaveBeenCalledWith("demo-school");
    });

    const initialCalls = vi.mocked(listGradeLevels).mock.calls.length;

    rerender(
      <ClassesPanel
        schoolSlug="demo-school"
        canManage={true}
        gradeLevelsVersion={1}
      />,
    );

    await waitFor(() => {
      expect(vi.mocked(listGradeLevels).mock.calls.length)
        .toBeGreaterThan(initialCalls);
    });
  });
  it("lists classes only for the selected school and session", async () => {
    vi.mocked(listClasses).mockResolvedValue([schoolClass]);

    render(<ClassesPanel schoolSlug="demo-school" canManage={false} />);

    expect(
      await screen.findByText("Code: PRI_3_2026 · Status: ACTIVE"),
    ).toBeInTheDocument();

    expect(listAcademicSessions).toHaveBeenCalledWith("demo-school");
    expect(listGradeLevels).toHaveBeenCalledWith("demo-school");
    expect(listClasses).toHaveBeenCalledWith(
      "demo-school",
      "session-1",
    );
  });

  it("hides class creation from read-only users", async () => {
    render(<ClassesPanel schoolSlug="demo-school" canManage={false} />);

    await screen.findByText(
      "No classes configured for this academic session.",
    );

    expect(
      screen.queryByRole("button", { name: "Create class" }),
    ).not.toBeInTheDocument();
  });

  it("creates a class with its session and grade level", async () => {
    const user = userEvent.setup();

    vi.mocked(createClass).mockResolvedValue(schoolClass);

    render(<ClassesPanel schoolSlug="demo-school" canManage={true} />);

    await screen.findByText(
      "No classes configured for this academic session.",
    );

    await user.selectOptions(
      screen.getByLabelText("Grade level"),
      "level-1",
    );

    await user.type(
      screen.getByLabelText("Class code"),
      "pri_3_2026",
    );

    await user.type(
      screen.getByLabelText("Class name"),
      "Primary 3",
    );

    await user.click(
      screen.getByRole("button", { name: "Create class" }),
    );

    await waitFor(() => {
      expect(createClass).toHaveBeenCalledWith("demo-school", {
        session_id: "session-1",
        grade_level_id: "level-1",
        code: "PRI_3_2026",
        name: "Primary 3",
      });
    });

    expect(
      await screen.findByText("Class created successfully."),
    ).toBeInTheDocument();
  });

  it("does not claim success when class creation fails", async () => {
    const user = userEvent.setup();

    vi.mocked(createClass).mockRejectedValue(
      new Error("Duplicate class code"),
    );

    render(<ClassesPanel schoolSlug="demo-school" canManage={true} />);

    await screen.findByText(
      "No classes configured for this academic session.",
    );

    await user.selectOptions(
      screen.getByLabelText("Grade level"),
      "level-1",
    );

    await user.type(screen.getByLabelText("Class code"), "PRI_3_2026");
    await user.type(screen.getByLabelText("Class name"), "Primary 3");

    await user.click(
      screen.getByRole("button", { name: "Create class" }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("Duplicate class code");

    expect(
      screen.queryByText("Class created successfully."),
    ).not.toBeInTheDocument();
  });

  it("requires an academic session before creating classes", async () => {
    vi.mocked(listAcademicSessions).mockResolvedValue([]);

    render(<ClassesPanel schoolSlug="demo-school" canManage={true} />);

    expect(
      await screen.findByText(
        "Create an academic session before configuring classes.",
      ),
    ).toBeInTheDocument();

    expect(listClasses).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("button", { name: "Create class" }),
    ).not.toBeInTheDocument();
  });

  it("updates the visible class after editing", async () => {
    const user = userEvent.setup();

    vi.mocked(listClasses).mockResolvedValue([schoolClass]);
    vi.mocked(updateClass).mockResolvedValue({
      ...schoolClass,
      name: "Primary Three",
    });

    render(<ClassesPanel schoolSlug="demo-school" canManage={true} />);

    await screen.findByText("Code: PRI_3_2026 · Status: ACTIVE");

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
    });

    expect(await screen.findByText("Primary Three"))
      .toBeInTheDocument();
  });

  it("updates the displayed class status after archiving", async () => {
    const user = userEvent.setup();

    vi.mocked(listClasses).mockResolvedValue([schoolClass]);
    vi.mocked(archiveClass).mockResolvedValue({
      ...schoolClass,
      status: "ARCHIVED",
    });

    render(<ClassesPanel schoolSlug="demo-school" canManage={true} />);

    await screen.findByText("Code: PRI_3_2026 · Status: ACTIVE");

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

    expect(
      await screen.findByText("Code: PRI_3_2026 · Status: ARCHIVED"),
    ).toBeInTheDocument();

    expect(archiveClass).toHaveBeenCalledWith(
      "demo-school",
      "class-1",
    );

    expect(
      screen.queryByRole("button", {
        name: "Archive class Primary 3",
      }),
    ).not.toBeInTheDocument();
  });

  it("does not add a delayed creation to another selected session", async () => {
    const user = userEvent.setup();

    const nextSession = {
      ...session,
      id: "session-2",
      name: "2027/2028",
      status: "DRAFT" as const,
    };

    vi.mocked(listAcademicSessions).mockResolvedValue([
      session,
      nextSession,
    ]);

    let resolveCreate!: (value: SchoolClass) => void;

    vi.mocked(createClass).mockImplementation(
      () =>
        new Promise<SchoolClass>((resolve) => {
          resolveCreate = resolve;
        }),
    );

    render(<ClassesPanel schoolSlug="demo-school" canManage={true} />);

    await screen.findByText(
      "No classes configured for this academic session.",
    );

    await user.selectOptions(
      screen.getByLabelText("Grade level"),
      "level-1",
    );

    await user.type(screen.getByLabelText("Class code"), "PRI_3_2026");
    await user.type(screen.getByLabelText("Class name"), "Primary 3");

    await user.click(
      screen.getByRole("button", { name: "Create class" }),
    );

    expect(createClass).toHaveBeenCalledTimes(1);

    await user.selectOptions(
      screen.getByLabelText("Academic session"),
      "session-2",
    );

    await waitFor(() => {
      expect(listClasses).toHaveBeenCalledWith(
        "demo-school",
        "session-2",
      );
    });

    resolveCreate(schoolClass);

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Create class" }),
      ).not.toBeDisabled();
    });

    expect(
      screen.queryByText("Class created successfully."),
    ).not.toBeInTheDocument();

    expect(
      screen.queryByText("Code: PRI_3_2026 · Status: ACTIVE"),
    ).not.toBeInTheDocument();
  });

  it("opens arms for the correct class with management permissions", async () => {
    const user = userEvent.setup();

    vi.mocked(listClasses).mockResolvedValue([schoolClass]);

    render(<ClassesPanel schoolSlug="demo-school" canManage={true} />);

    await screen.findByText("Code: PRI_3_2026 · Status: ACTIVE");

    expect(
      screen.queryByTestId("class-arms-panel"),
    ).not.toBeInTheDocument();

    await user.click(
      screen.getByRole("button", {
        name: "Manage arms for Primary 3",
      }),
    );

    const panel = screen.getByTestId("class-arms-panel");

    expect(panel).toHaveAttribute("data-school-slug", "demo-school");
    expect(panel).toHaveAttribute("data-class-id", "class-1");
    expect(panel).toHaveAttribute("data-can-manage", "true");

    await user.click(
      screen.getByRole("button", {
        name: "Hide arms for Primary 3",
      }),
    );

    expect(
      screen.queryByTestId("class-arms-panel"),
    ).not.toBeInTheDocument();
  });

  it("passes read-only permissions to class arms", async () => {
    const user = userEvent.setup();

    vi.mocked(listClasses).mockResolvedValue([schoolClass]);

    render(<ClassesPanel schoolSlug="demo-school" canManage={false} />);

    await screen.findByText("Code: PRI_3_2026 · Status: ACTIVE");

    await user.click(
      screen.getByRole("button", {
        name: "Manage arms for Primary 3",
      }),
    );

    expect(screen.getByTestId("class-arms-panel"))
      .toHaveAttribute("data-can-manage", "false");
  });

  it("closes class arms when switching academic sessions", async () => {
    const user = userEvent.setup();

    vi.mocked(listAcademicSessions).mockResolvedValue([
      session,
      {
        ...session,
        id: "session-2",
        name: "2027/2028",
        status: "DRAFT",
      },
    ]);

    vi.mocked(listClasses).mockImplementation(
      async (_slug, sessionID) =>
        sessionID === "session-1" ? [schoolClass] : [],
    );

    render(<ClassesPanel schoolSlug="demo-school" canManage={true} />);

    await screen.findByText("Code: PRI_3_2026 · Status: ACTIVE");

    await user.click(
      screen.getByRole("button", {
        name: "Manage arms for Primary 3",
      }),
    );

    expect(
      screen.getByTestId("class-arms-panel"),
    ).toBeInTheDocument();

    await user.selectOptions(
      screen.getByLabelText("Academic session"),
      "session-2",
    );

    expect(
      screen.queryByTestId("class-arms-panel"),
    ).not.toBeInTheDocument();

    await waitFor(() => {
      expect(listClasses).toHaveBeenCalledWith(
        "demo-school",
        "session-2",
      );
    });
  });

  it("does not display a delayed class creation after switching schools", async () => {
    const user = userEvent.setup();

    let resolveCreate!: (value: SchoolClass) => void;

    vi.mocked(createClass).mockImplementation(
      () =>
        new Promise<SchoolClass>((resolve) => {
          resolveCreate = resolve;
        }),
    );

    const { rerender } = render(
      <ClassesPanel schoolSlug="school-a" canManage={true} />,
    );

    await screen.findByText(
      "No classes configured for this academic session.",
    );

    await user.selectOptions(
      screen.getByLabelText("Grade level"),
      "level-1",
    );

    await user.type(
      screen.getByLabelText("Class code"),
      "PRI_3_2026",
    );

    await user.type(
      screen.getByLabelText("Class name"),
      "Primary 3",
    );

    await user.click(
      screen.getByRole("button", { name: "Create class" }),
    );

    expect(createClass).toHaveBeenCalledWith("school-a", {
      session_id: "session-1",
      grade_level_id: "level-1",
      code: "PRI_3_2026",
      name: "Primary 3",
    });

    // Preserve the same academic session ID in both schools.
    // This specifically tests the school guard, not just the session guard.
    rerender(
      <ClassesPanel schoolSlug="school-b" canManage={true} />,
    );

    await waitFor(() => {
      expect(listAcademicSessions).toHaveBeenCalledWith("school-b");
      expect(listGradeLevels).toHaveBeenCalledWith("school-b");
      expect(listClasses).toHaveBeenCalledWith("school-b", "session-1");
    });

    await act(async () => {
      resolveCreate(schoolClass);
    });

    expect(
      screen.queryByText("Code: PRI_3_2026 · Status: ACTIVE"),
    ).not.toBeInTheDocument();

    expect(
      screen.queryByText("Class created successfully."),
    ).not.toBeInTheDocument();
  });
});