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
import SessionActivationControl from "./session-activation-control";
import {
  activateAcademicSession,
  type AcademicSession,
  type AcademicTerm,
} from "@/lib/academics-client";

vi.mock("@/lib/academics-client", () => ({
  activateAcademicSession: vi.fn(),
}));

const session: AcademicSession = {
  id: "session-1",
  name: "2026/2027",
  starts_on: "2026-09-01",
  ends_on: "2027-07-31",
  status: "DRAFT",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const terms: AcademicTerm[] = [1, 2, 3].map((sequence) => ({
  id: `term-${sequence}`,
  session_id: session.id,
  sequence,
  name: `Term ${sequence}`,
  starts_on: "2026-09-01",
  ends_on: "2027-07-31",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
}));

describe("SessionActivationControl", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("requires three configured terms", () => {
    render(
      <SessionActivationControl
        schoolSlug="demo-school"
        session={session}
        terms={[]}
        onActivated={vi.fn()}
      />,
    );

    expect(
      screen.getByText(/Configure all three academic terms/),
    ).toBeInTheDocument();

    expect(
      screen.queryByRole("button", {
        name: "Activate academic session",
      }),
    ).not.toBeInTheDocument();
  });

  it("requires explicit confirmation before activation", async () => {
    const user = userEvent.setup();

    render(
      <SessionActivationControl
        schoolSlug="demo-school"
        session={session}
        terms={terms}
        onActivated={vi.fn()}
      />,
    );

    const button = screen.getByRole("button", {
      name: "Activate academic session",
    });

    expect(button).toBeDisabled();

    await user.click(screen.getByRole("checkbox"));

    expect(button).toBeEnabled();
    expect(activateAcademicSession).not.toHaveBeenCalled();
  });

  it("activates the correct session after confirmation", async () => {
    const user = userEvent.setup();
    const onActivated = vi.fn();
    const activated = { ...session, status: "ACTIVE" as const };

    vi.mocked(activateAcademicSession)
      .mockResolvedValue(activated);

    render(
      <SessionActivationControl
        schoolSlug="demo-school"
        session={session}
        terms={terms}
        onActivated={onActivated}
      />,
    );

    await user.click(screen.getByRole("checkbox"));
    await user.click(
      screen.getByRole("button", {
        name: "Activate academic session",
      }),
    );

    await waitFor(() => {
      expect(onActivated).toHaveBeenCalledWith(activated);
    });

    expect(activateAcademicSession).toHaveBeenCalledWith(
      "demo-school",
      "session-1",
    );
  });

  it("does not report success when the backend rejects activation", async () => {
    const user = userEvent.setup();
    const onActivated = vi.fn();

    vi.mocked(activateAcademicSession).mockRejectedValue(
      new Error("Another academic session is already active"),
    );

    render(
      <SessionActivationControl
        schoolSlug="demo-school"
        session={session}
        terms={terms}
        onActivated={onActivated}
      />,
    );

    await user.click(screen.getByRole("checkbox"));
    await user.click(
      screen.getByRole("button", {
        name: "Activate academic session",
      }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("already active");

    expect(onActivated).not.toHaveBeenCalled();
  });

  it("rejects an unconfirmed backend response", async () => {
    const user = userEvent.setup();
    const onActivated = vi.fn();

    vi.mocked(activateAcademicSession)
      .mockResolvedValue(session);

    render(
      <SessionActivationControl
        schoolSlug="demo-school"
        session={session}
        terms={terms}
        onActivated={onActivated}
      />,
    );

    await user.click(screen.getByRole("checkbox"));
    await user.click(
      screen.getByRole("button", {
        name: "Activate academic session",
      }),
    );

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("did not confirm session activation");

    expect(onActivated).not.toHaveBeenCalled();
  });

  it("hides activation controls for active sessions", () => {
    render(
      <SessionActivationControl
        schoolSlug="demo-school"
        session={{ ...session, status: "ACTIVE" }}
        terms={terms}
        onActivated={vi.fn()}
      />,
    );

    expect(
      screen.queryByRole("button", {
        name: "Activate academic session",
      }),
    ).not.toBeInTheDocument();
  });
});