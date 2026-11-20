import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import AcademicsClient from "./academics-client";
import {
  listAcademicSessions,
  listAcademicTerms,
  createAcademicSession,
  type AcademicSession,
  type AcademicTerm,
} from "@/lib/academics-client";
import { establishCSRFToken, type Tenant } from "@/lib/tenant-client";

vi.mock("./classes-panel", () => ({
  default: ({
    schoolSlug,
    canManage,
  }: {
    schoolSlug: string;
    canManage: boolean;
  }) => (
    <div
      data-testid="classes-panel"
      data-school-slug={schoolSlug}
      data-can-manage={String(canManage)}
    />
  ),
}));
vi.mock("./grade-levels-panel", () => ({
  default: ({
    schoolSlug,
    canManage,
  }: {
    schoolSlug: string;
    canManage: boolean;
  }) => (
    <div
      data-testid="grade-levels-panel"
      data-school-slug={schoolSlug}
      data-can-manage={String(canManage)}
    />
  ),
}));
vi.mock("@/lib/academics-client", () => ({
  listAcademicSessions: vi.fn(),
  listAcademicTerms: vi.fn(),
  createAcademicSession: vi.fn(),
  configureAcademicTerms: vi.fn(),
  activateAcademicSession: vi.fn(),
  archiveAcademicSession: vi.fn(),
}));

vi.mock("@/lib/tenant-client", () => ({
  establishCSRFToken: vi.fn(),
}));

const school: Tenant = {
  id: "school-1",
  slug: "demo-school",
  name: "Demo School",
  status: "ACTIVE",
  locale: "en-NG",
  timezone: "Africa/Lagos",
  branding: {},
  education_stages: ["PRIMARY"],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  current_role: "SCHOOL_ADMIN",
  can_manage_academics: true,
};

const firstSession: AcademicSession = {
  id: "session-1",
  name: "2026/2027",
  starts_on: "2026-09-01",
  ends_on: "2027-07-31",
  status: "ACTIVE",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const secondSession: AcademicSession = {
  ...firstSession,
  id: "session-2",
  name: "2027/2028",
  starts_on: "2027-09-01",
  ends_on: "2028-07-31",
  status: "DRAFT",
};

const firstTerm: AcademicTerm = {
  id: "term-1",
  session_id: "session-1",
  sequence: 1,
  name: "First Term",
  starts_on: "2026-09-01",
  ends_on: "2026-12-15",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("Academic Management dashboard", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.mocked(listAcademicSessions).mockResolvedValue([]);
    vi.mocked(listAcademicTerms).mockResolvedValue([]);
  });

  afterEach(() => {
    cleanup();
  });

  it("displays the loading state and verifies the CSRF context", () => {
    vi.mocked(listAcademicSessions).mockReturnValue(
      new Promise<AcademicSession[]>(() => {}),
    );

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    expect(screen.getByText("Loading academic sessions...")).toBeInTheDocument();
    expect(establishCSRFToken).toHaveBeenCalledWith("fictional-csrf");
    expect(listAcademicSessions).toHaveBeenCalledWith("demo-school");
  });

  it("displays an empty state without inventing academic records", async () => {
    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    expect(
      await screen.findByText("No academic sessions configured"),
    ).toBeInTheDocument();

    expect(listAcademicTerms).not.toHaveBeenCalled();
  });

  it("loads the school's active session and its terms", async () => {
    vi.mocked(listAcademicSessions).mockResolvedValue([firstSession]);
    vi.mocked(listAcademicTerms).mockResolvedValue([firstTerm]);

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    expect(await screen.findByText("1. First Term")).toBeInTheDocument();

    expect(listAcademicTerms).toHaveBeenCalledWith(
      "demo-school",
      "session-1",
    );

    expect(
      screen.getByRole("button", { name: /2026\/2027/ }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("switches sessions without retaining previous term records", async () => {
    const user = userEvent.setup();

    vi.mocked(listAcademicSessions).mockResolvedValue([
      firstSession,
      secondSession,
    ]);

    vi.mocked(listAcademicTerms).mockImplementation(
      async (_slug, sessionID) =>
        sessionID === "session-1"
          ? [firstTerm]
          : [{ ...firstTerm, id: "term-2", session_id: "session-2", name: "New Term" }],
    );

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    expect(await screen.findByText("1. First Term")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /2027\/2028/ }));

    expect(await screen.findByText("1. New Term")).toBeInTheDocument();
    expect(screen.queryByText("1. First Term")).not.toBeInTheDocument();
    expect(listAcademicTerms).toHaveBeenCalledWith("demo-school", "session-2");
  });

  it("shows API errors and retries successfully", async () => {
    const user = userEvent.setup();

    vi.mocked(listAcademicSessions)
      .mockRejectedValueOnce(new Error("Academic service unavailable"))
      .mockResolvedValueOnce([firstSession]);

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    expect(
      await screen.findByRole("alert"),
    ).toHaveTextContent("Academic service unavailable");

    await user.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /2026\/2027/ }),
      ).toBeInTheDocument();
    });

    expect(listAcademicSessions).toHaveBeenCalledTimes(2);
  });

  it("shows the creation form to school administrators", async () => {
    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    expect(
      await screen.findByRole("button", { name: "Create draft session" }),
    ).toBeInTheDocument();
  });

  it("shows management controls for a Principal whose display role is Bursar", async () => {
    const multiRoleSchool = { ...school, current_role: "BURSAR", can_manage_academics: true, can_invite_staff: false };
    render(<AcademicsClient school={multiRoleSchool} csrfToken="fictional-csrf" />);
    expect(await screen.findByRole("button", { name: "Create draft session" })).toBeVisible();
    expect(screen.queryByRole("link", { name: "Invite Staff" })).toBeNull();
  });

  it("adds a newly created session to the dashboard", async () => {
    const user = userEvent.setup();
    vi.mocked(createAcademicSession).mockResolvedValue({
      ...firstSession,
      status: "DRAFT",
    });

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    await screen.findByRole("button", { name: "Create draft session" });

    await user.type(screen.getByLabelText("Session name"), "2026/2027");
    await user.type(screen.getByLabelText("Start date"), "2026-09-01");
    await user.type(screen.getByLabelText("End date"), "2027-07-31");

    await user.click(
      screen.getByRole("button", { name: "Create draft session" }),
    );

    expect(
      await screen.findByRole("button", { name: /2026\/2027/ }),
    ).toHaveAttribute("aria-pressed", "true");

    expect(createAcademicSession).toHaveBeenCalledWith(
      "demo-school",
      {
        name: "2026/2027",
        starts_on: "2026-09-01",
        ends_on: "2027-07-31",
      },
    );
  });
  it("shows term configuration only for an authorized draft session", async () => {
    vi.mocked(listAcademicSessions).mockResolvedValue([
      { ...firstSession, status: "DRAFT" },
      secondSession,
    ]);
    vi.mocked(listAcademicTerms).mockResolvedValue([]);

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    expect(
      await screen.findByRole("button", { name: "Save three terms" }),
    ).toBeInTheDocument();
  });

  it("does not permit editing terms on an active session", async () => {
    vi.mocked(listAcademicSessions).mockResolvedValue([firstSession]);
    vi.mocked(listAcademicTerms).mockResolvedValue([]);

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    await screen.findByRole("button", { name: /2026\/2027/ });

    expect(
      screen.queryByRole("button", { name: "Save three terms" }),
    ).not.toBeInTheDocument();
  });
  it("resets term inputs when switching between draft sessions", async () => {
    const user = userEvent.setup();

    vi.mocked(listAcademicSessions).mockResolvedValue([
      { ...firstSession, status: "DRAFT" },
      secondSession,
    ]);

    vi.mocked(listAcademicTerms).mockImplementation(
      async (_slug, sessionID) =>
        sessionID === "session-1"
          ? [firstTerm]
          : [],
    );

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    expect(
      await screen.findByRole("button", { name: "Save three terms" }),
    ).toBeInTheDocument();

    expect(screen.getByLabelText("First Term start date"))
      .toHaveValue("2026-09-01");

    await user.click(screen.getByRole("button", { name: /2027\/2028/ }));

    await waitFor(() => {
      expect(screen.getByLabelText("First Term start date"))
        .toHaveValue("");
    });

    expect(listAcademicTerms).toHaveBeenCalledWith(
      "demo-school",
      "session-2",
    );
  });

  it("shows a success notice after saving three terms", async () => {
    const user = userEvent.setup();

    vi.mocked(listAcademicSessions).mockResolvedValue([
      { ...firstSession, status: "DRAFT" },
    ]);
    vi.mocked(listAcademicTerms).mockResolvedValue([]);

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    await screen.findByRole("button", { name: "Save three terms" });

    const validDates = [
      ["First Term", "2026-09-07", "2026-12-18"],
      ["Second Term", "2027-01-11", "2027-04-09"],
      ["Third Term", "2027-04-26", "2027-07-23"],
    ];

    for (const [name, start, end] of validDates) {
      await user.type(screen.getByLabelText(`${name} start date`), start);
      await user.type(screen.getByLabelText(`${name} end date`), end);
    }

    const savedTerms: AcademicTerm[] = validDates.map(
      ([name, start, end], index) => ({
        ...firstTerm,
        id: `saved-term-${index + 1}`,
        session_id: "session-1",
        sequence: index + 1,
        name,
        starts_on: start,
        ends_on: end,
      }),
    );

    const { configureAcademicTerms } = await import(
      "@/lib/academics-client"
    );

    vi.mocked(configureAcademicTerms).mockResolvedValue(savedTerms);

    await user.click(screen.getByRole("button", { name: "Save three terms" }));

    expect(
      await screen.findByText("Academic terms saved."),
    ).toBeInTheDocument();

    expect(screen.getByText("1. First Term")).toBeInTheDocument();
    expect(screen.getByText("2. Second Term")).toBeInTheDocument();
    expect(screen.getByText("3. Third Term")).toBeInTheDocument();
  });

  it("hides the term configuration form from teachers", async () => {
    vi.mocked(listAcademicSessions).mockResolvedValue([
      { ...firstSession, status: "DRAFT" },
    ]);
    vi.mocked(listAcademicTerms).mockResolvedValue([]);

    render(
      <AcademicsClient
        school={{ ...school, current_role: "TEACHER", can_manage_academics: false }}
        csrfToken="fictional-csrf"
      />,
    );

    expect(
      await screen.findByRole("button", { name: /2026\/2027/ }),
    ).toBeInTheDocument();

    await waitFor(() => {
      expect(listAcademicTerms).toHaveBeenCalled();
    });

    expect(
      screen.queryByRole("button", { name: "Save three terms" }),
    ).not.toBeInTheDocument();
  });
  it("activates a draft session and updates its displayed status", async () => {
    const user = userEvent.setup();

    const draft = { ...firstSession, status: "DRAFT" as const };
    const threeTerms: AcademicTerm[] = [1, 2, 3].map((sequence) => ({
      ...firstTerm,
      id: `term-${sequence}`,
      sequence,
    }));

    vi.mocked(listAcademicSessions).mockResolvedValue([draft]);
    vi.mocked(listAcademicTerms).mockResolvedValue(threeTerms);

    const { activateAcademicSession } = await import(
      "@/lib/academics-client"
    );

    vi.mocked(activateAcademicSession).mockResolvedValue({
      ...draft,
      status: "ACTIVE",
    });

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    const activate = await screen.findByRole("button", {
      name: "Activate academic session",
    });

    expect(activate).toBeDisabled();

    await user.click(
      screen.getByRole("checkbox", {
        name: /I confirm that I want to activate the academic session/,
      }),
    );

    await user.click(activate);

    expect(
      await screen.findByText("Academic session activated."),
    ).toBeInTheDocument();

    expect(activateAcademicSession).toHaveBeenCalledWith(
      "demo-school",
      "session-1",
    );

    expect(
      screen.queryByRole("button", { name: "Activate academic session" }),
    ).not.toBeInTheDocument();

    expect(
      screen.queryByRole("button", { name: "Save three terms" }),
    ).not.toBeInTheDocument();

    expect(
      screen.getByRole("button", { name: /2026\/2027.*ACTIVE/ }),
    ).toBeInTheDocument();
  });

  it("hides activation controls from teachers", async () => {
    const threeTerms: AcademicTerm[] = [1, 2, 3].map((sequence) => ({
      ...firstTerm,
      id: `term-${sequence}`,
      sequence,
    }));

    vi.mocked(listAcademicSessions).mockResolvedValue([
      { ...firstSession, status: "DRAFT" },
    ]);
    vi.mocked(listAcademicTerms).mockResolvedValue(threeTerms);

    render(
      <AcademicsClient
        school={{ ...school, current_role: "TEACHER", can_manage_academics: false }}
        csrfToken="fictional-csrf"
      />,
    );

    await waitFor(() => {
      expect(listAcademicTerms).toHaveBeenCalledWith(
        "demo-school",
        "session-1",
      );
    });

    expect(
      screen.queryByRole("button", { name: "Activate academic session" }),
    ).not.toBeInTheDocument();
  });

  it("does not show an activation notice under another selected session", async () => {
    const user = userEvent.setup();
    const draft = { ...firstSession, status: "DRAFT" as const };

    const threeTerms: AcademicTerm[] = [1, 2, 3].map((sequence) => ({
      ...firstTerm,
      id: `term-${sequence}`,
      sequence,
    }));

    vi.mocked(listAcademicSessions).mockResolvedValue([
      draft,
      secondSession,
    ]);

    vi.mocked(listAcademicTerms).mockImplementation(
      async (_slug, sessionID) =>
        sessionID === "session-1" ? threeTerms : [],
    );

    const { activateAcademicSession } = await import(
      "@/lib/academics-client"
    );

    let resolveActivation!: (value: AcademicSession) => void;

    vi.mocked(activateAcademicSession).mockReturnValue(
      new Promise<AcademicSession>((resolve) => {
        resolveActivation = resolve;
      }),
    );

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    await screen.findByRole("button", {
      name: "Activate academic session",
    });

    await user.click(
      screen.getByRole("checkbox", {
        name: /I confirm that I want to activate the academic session/,
      }),
    );
    await user.click(
      screen.getByRole("button", {
        name: "Activate academic session",
      }),
    );

    await waitFor(() => {
      expect(activateAcademicSession).toHaveBeenCalledWith(
        "demo-school",
        "session-1",
      );
    });

    await user.click(screen.getByRole("button", { name: /2027\/2028/ }));

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /2027\/2028/ }),
      ).toHaveAttribute("aria-pressed", "true");
    });

    resolveActivation({ ...draft, status: "ACTIVE" });

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /2026\/2027.*ACTIVE/ }),
      ).toBeInTheDocument();
    });

    expect(
      screen.queryByText("Academic session activated."),
    ).not.toBeInTheDocument();
  });

  it("does not restart loading when reselecting the current session", async () => {
    const user = userEvent.setup();

    vi.mocked(listAcademicSessions).mockResolvedValue([firstSession]);
    vi.mocked(listAcademicTerms).mockResolvedValue([firstTerm]);

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    await screen.findByText("1. First Term");

    await user.click(screen.getByRole("button", { name: /2026\/2027/ }));

    expect(screen.queryByText("Loading terms...")).not.toBeInTheDocument();
    expect(screen.getByText("1. First Term")).toBeInTheDocument();
    expect(listAcademicTerms).toHaveBeenCalledTimes(1);
  });
  it("does not apply a completed term save to another selected session", async () => {
    const user = userEvent.setup();

    vi.mocked(listAcademicSessions).mockResolvedValue([
      { ...firstSession, status: "DRAFT" },
      secondSession,
    ]);

    vi.mocked(listAcademicTerms).mockResolvedValue([]);

    const { configureAcademicTerms } = await import(
      "@/lib/academics-client"
    );

    let resolveSave!: (value: AcademicTerm[]) => void;

    vi.mocked(configureAcademicTerms).mockReturnValue(
      new Promise<AcademicTerm[]>((resolve) => {
        resolveSave = resolve;
      }),
    );

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    await screen.findByRole("button", { name: "Save three terms" });

    const values = [
      ["First Term", "2026-09-07", "2026-12-18"],
      ["Second Term", "2027-01-11", "2027-04-09"],
      ["Third Term", "2027-04-26", "2027-07-23"],
    ];

    for (const [name, start, end] of values) {
      fireEvent.change(screen.getByLabelText(`${name} start date`), {
        target: { value: start },
      });
      fireEvent.change(screen.getByLabelText(`${name} end date`), {
        target: { value: end },
      });
    }

    await user.click(
      screen.getByRole("button", { name: "Save three terms" }),
    );

    await waitFor(() => {
      expect(configureAcademicTerms).toHaveBeenCalledWith(
        "demo-school",
        "session-1",
        expect.any(Array),
      );
    });

    await user.click(screen.getByRole("button", { name: /2027\/2028/ }));

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /2027\/2028/ }),
      ).toHaveAttribute("aria-pressed", "true");

      expect(screen.getByLabelText("First Term start date"))
        .toHaveValue("");
    });

    const savedTerms: AcademicTerm[] = values.map(
      ([name, start, end], index) => ({
        ...firstTerm,
        id: `saved-${index + 1}`,
        sequence: index + 1,
        session_id: "session-1",
        name,
        starts_on: start,
        ends_on: end,
      }),
    );

    await act(async () => {
      resolveSave(savedTerms);
    });

    expect(
      screen.queryByText("Academic terms saved."),
    ).not.toBeInTheDocument();

    expect(
      screen.queryByText("1. First Term"),
    ).not.toBeInTheDocument();

    expect(
      screen.getByLabelText("First Term start date"),
    ).toHaveValue("");

    expect(
      screen.getByRole("button", { name: /2027\/2028/ }),
    ).toHaveAttribute("aria-pressed", "true");
  });
  it("archives a session and updates the displayed status", async () => {
    const user = userEvent.setup();

    vi.mocked(listAcademicSessions).mockResolvedValue([firstSession]);
    vi.mocked(listAcademicTerms).mockResolvedValue([firstTerm]);

    const { archiveAcademicSession } = await import(
      "@/lib/academics-client"
    );

    vi.mocked(archiveAcademicSession).mockResolvedValue({
      ...firstSession,
      status: "ARCHIVED",
    });

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    const archive = await screen.findByRole("button", {
      name: "Archive academic session",
    });

    expect(archive).toBeDisabled();

    await user.click(screen.getByRole("checkbox"));

    await user.click(archive);

    expect(
      await screen.findByText("Academic session archived."),
    ).toBeInTheDocument();

    expect(archiveAcademicSession).toHaveBeenCalledWith(
      "demo-school",
      "session-1",
    );

    expect(
      screen.getByRole("button", { name: /2026\/2027.*ARCHIVED/ }),
    ).toBeInTheDocument();

    expect(
      screen.queryByRole("button", { name: "Archive academic session" }),
    ).not.toBeInTheDocument();

    expect(
      screen.queryByRole("button", { name: "Save three terms" }),
    ).not.toBeInTheDocument();
  });

  it("does not show archive controls to teachers", async () => {
    vi.mocked(listAcademicSessions).mockResolvedValue([firstSession]);
    vi.mocked(listAcademicTerms).mockResolvedValue([firstTerm]);

    render(
      <AcademicsClient
        school={{ ...school, current_role: "TEACHER", can_manage_academics: false }}
        csrfToken="fictional-csrf"
      />,
    );

    await screen.findByText("1. First Term");

    expect(
      screen.queryByRole("button", { name: "Archive academic session" }),
    ).not.toBeInTheDocument();
  });

  it("does not show an archiving notice under another selected session", async () => {
    const user = userEvent.setup();

    vi.mocked(listAcademicSessions).mockResolvedValue([
      firstSession,
      secondSession,
    ]);

    vi.mocked(listAcademicTerms).mockImplementation(
      async (_slug, sessionID) =>
        sessionID === "session-1" ? [firstTerm] : [],
    );

    const { archiveAcademicSession } = await import(
      "@/lib/academics-client"
    );

    let resolveArchive!: (value: AcademicSession) => void;

    vi.mocked(archiveAcademicSession).mockReturnValue(
      new Promise<AcademicSession>((resolve) => {
        resolveArchive = resolve;
      }),
    );

    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    await screen.findByRole("button", {
      name: "Archive academic session",
    });

    await user.click(
      screen.getByRole("checkbox", {
        name: /I understand that archiving academic session/,
      }),
    );

    await user.click(
      screen.getByRole("button", {
        name: "Archive academic session",
      }),
    );

    await waitFor(() => {
      expect(archiveAcademicSession).toHaveBeenCalledWith(
        "demo-school",
        "session-1",
      );
    });

    await user.click(
      screen.getByRole("button", { name: /2027\/2028/ }),
    );

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /2027\/2028/ }),
      ).toHaveAttribute("aria-pressed", "true");
    });

    await act(async () => {
      resolveArchive({
        ...firstSession,
        status: "ARCHIVED",
      });
    });

    await waitFor(() => {
      expect(
        screen.getByRole("button", {
          name: /2026\/2027.*ARCHIVED/,
        }),
      ).toBeInTheDocument();
    });

    expect(
      screen.getByRole("button", { name: /2027\/2028/ }),
    ).toHaveAttribute("aria-pressed", "true");

    expect(
      screen.queryByText("Academic session archived."),
    ).not.toBeInTheDocument();

    expect(archiveAcademicSession).toHaveBeenCalledTimes(1);
  });
  it("does not display the management badge to teacher accounts", async () => {
    render(
      <AcademicsClient
        school={{ ...school, current_role: "TEACHER", can_manage_academics: false }}
        csrfToken="fictional-csrf"
      />,
    );

    expect(
      await screen.findByText("No academic sessions configured"),
    ).toBeInTheDocument();

    expect(screen.queryByText("Management access")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Create draft session" }),
    ).not.toBeInTheDocument();
  });

  it("connects the Grade Levels panel to the verified school", () => {
    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    const panel = screen.getByTestId("grade-levels-panel");

    expect(panel).toHaveAttribute("data-school-slug", "demo-school");
    expect(panel).toHaveAttribute("data-can-manage", "true");
  });

  it("passes read-only permissions for teachers", () => {
    render(
      <AcademicsClient
        school={{ ...school, current_role: "TEACHER", can_manage_academics: false }}
        csrfToken="fictional-csrf"
      />,
    );

    const panel = screen.getByTestId("grade-levels-panel");

    expect(panel).toHaveAttribute("data-school-slug", "demo-school");
    expect(panel).toHaveAttribute("data-can-manage", "false");
  });

  it("uses the new school context when changing schools", () => {
    const { rerender } = render(
      <AcademicsClient school={school} csrfToken="fictional-csrf" />,
    );

    rerender(
      <AcademicsClient
        school={{
          ...school,
          id: "school-2",
          slug: "other-school",
          current_role: "TEACHER", can_manage_academics: false,
        }}
        csrfToken="fictional-csrf"
      />,
    );

    const panel = screen.getByTestId("grade-levels-panel");

    expect(panel).toHaveAttribute("data-school-slug", "other-school");
    expect(panel).toHaveAttribute("data-can-manage", "false");
  });

  it("connects Classes to the verified tenant school", () => {
    render(<AcademicsClient school={school} csrfToken="fictional-csrf" />);

    const panel = screen.getByTestId("classes-panel");

    expect(panel).toHaveAttribute("data-school-slug", "demo-school");
    expect(panel).toHaveAttribute("data-can-manage", "true");
  });

  it("passes read-only permissions to Classes for teachers", () => {
    render(
      <AcademicsClient
        school={{ ...school, current_role: "TEACHER", can_manage_academics: false }}
        csrfToken="fictional-csrf"
      />,
    );

    const panel = screen.getByTestId("classes-panel");

    expect(panel).toHaveAttribute("data-school-slug", "demo-school");
    expect(panel).toHaveAttribute("data-can-manage", "false");
  });

  it("updates Classes context when another school is selected", () => {
    const { rerender } = render(
      <AcademicsClient school={school} csrfToken="fictional-csrf" />,
    );

    rerender(
      <AcademicsClient
        school={{
          ...school,
          id: "school-2",
          slug: "other-school",
          current_role: "TEACHER", can_manage_academics: false,
        }}
        csrfToken="fictional-csrf"
      />,
    );

    const panel = screen.getByTestId("classes-panel");

    expect(panel).toHaveAttribute("data-school-slug", "other-school");
    expect(panel).toHaveAttribute("data-can-manage", "false");
  });
});