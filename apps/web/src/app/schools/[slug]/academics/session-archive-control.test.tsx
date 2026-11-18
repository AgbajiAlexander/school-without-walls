import {
  act,
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
import SessionArchiveControl from "./session-archive-control";
import {
  archiveAcademicSession,
  type AcademicSession,
} from "@/lib/academics-client";

vi.mock("@/lib/academics-client", () => ({
  archiveAcademicSession: vi.fn(),
}));

const session: AcademicSession = {
  id: "session-1",
  name: "2026/2027",
  starts_on: "2026-09-01",
  ends_on: "2027-07-31",
  status: "ACTIVE",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("SessionArchiveControl", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("requires explicit confirmation", async () => {
    const user = userEvent.setup();

    render(
      <SessionArchiveControl
        schoolSlug="demo-school"
        session={session}
        onArchived={vi.fn()}
      />,
    );

    const button = screen.getByRole("button", {
      name: "Archive academic session",
    });

    expect(button).toBeDisabled();

    await user.click(screen.getByRole("checkbox"));

    expect(button).toBeEnabled();
    expect(archiveAcademicSession).not.toHaveBeenCalled();
  });

  it("archives the correct school session after confirmation", async () => {
    const user = userEvent.setup();
    const onArchived = vi.fn();
    const archived = {
      ...session,
      status: "ARCHIVED" as const,
    };

    vi.mocked(archiveAcademicSession).mockResolvedValue(archived);

    render(
      <SessionArchiveControl
        schoolSlug="demo-school"
        session={session}
        onArchived={onArchived}
      />,
    );

    await user.click(screen.getByRole("checkbox"));
    await user.click(
      screen.getByRole("button", {
        name: "Archive academic session",
      }),
    );

    await waitFor(() => {
      expect(onArchived).toHaveBeenCalledWith(archived);
    });

    expect(archiveAcademicSession).toHaveBeenCalledWith(
      "demo-school",
      "session-1",
    );
    expect(archiveAcademicSession).toHaveBeenCalledTimes(1);
  });

  it("reports backend rejection without changing the session", async () => {
    const user = userEvent.setup();
    const onArchived = vi.fn();

    vi.mocked(archiveAcademicSession).mockRejectedValue(
      new Error("Archiving is not permitted"),
    );

    render(
      <SessionArchiveControl
        schoolSlug="demo-school"
        session={session}
        onArchived={onArchived}
      />,
    );

    await user.click(screen.getByRole("checkbox"));
    await user.click(
      screen.getByRole("button", {
        name: "Archive academic session",
      }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("Archiving is not permitted");

    expect(onArchived).not.toHaveBeenCalled();
  });

  it("rejects a response that does not confirm archiving", async () => {
    const user = userEvent.setup();
    const onArchived = vi.fn();

    vi.mocked(archiveAcademicSession).mockResolvedValue(session);

    render(
      <SessionArchiveControl
        schoolSlug="demo-school"
        session={session}
        onArchived={onArchived}
      />,
    );

    await user.click(screen.getByRole("checkbox"));
    await user.click(
      screen.getByRole("button", {
        name: "Archive academic session",
      }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("did not confirm session archiving");

    expect(onArchived).not.toHaveBeenCalled();
  });

  it("prevents another archive request while one is pending", async () => {
    const user = userEvent.setup();

    let resolveArchive!: (value: AcademicSession) => void;

    vi.mocked(archiveAcademicSession).mockReturnValue(
      new Promise<AcademicSession>((resolve) => {
        resolveArchive = resolve;
      }),
    );

    render(
      <SessionArchiveControl
        schoolSlug="demo-school"
        session={session}
        onArchived={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("checkbox"));
    await user.click(
      screen.getByRole("button", {
        name: "Archive academic session",
      }),
    );

    expect(
      screen.getByRole("button", {
        name: "Archiving session...",
      }),
    ).toBeDisabled();

    expect(archiveAcademicSession).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveArchive({
        ...session,
        status: "ARCHIVED",
      });
    });

    expect(archiveAcademicSession).toHaveBeenCalledTimes(1);
  });

  it("does not render controls for an archived session", () => {
    render(
      <SessionArchiveControl
        schoolSlug="demo-school"
        session={{ ...session, status: "ARCHIVED" }}
        onArchived={vi.fn()}
      />,
    );

    expect(
      screen.queryByRole("button", {
        name: "Archive academic session",
      }),
    ).not.toBeInTheDocument();

    expect(archiveAcademicSession).not.toHaveBeenCalled();
  });
});