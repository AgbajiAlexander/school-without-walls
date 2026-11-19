import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import SessionEditForm from "./session-edit-form";
import { updateAcademicSession, type AcademicSession } from "@/lib/academics-client";

vi.mock("@/lib/academics-client", () => ({
  updateAcademicSession: vi.fn(),
}));

const session: AcademicSession = {
  id: "session-1",
  name: "2026/2027",
  starts_on: "2026-09-04",
  ends_on: "2026-12-18",
  status: "DRAFT",
  created_at: "2026-09-01T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
};

function openEditor() {
  fireEvent.click(screen.getByRole("button", { name: "Edit Session" }));
}

function fillDates(start: string, end: string) {
  fireEvent.change(screen.getByLabelText("Edit session start date"), {
    target: { value: start },
  });
  fireEvent.change(screen.getByLabelText("Edit session end date"), {
    target: { value: end },
  });
}

describe("SessionEditForm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("opens with the existing session details", () => {
    render(
      <SessionEditForm
        schoolSlug="morfol-demo"
        session={session}
        onUpdated={vi.fn()}
      />,
    );

    openEditor();

    expect(
      (screen.getByLabelText("Edit session name") as HTMLInputElement).value,
    ).toBe("2026/2027");

    expect(
      (screen.getByLabelText("Edit session start date") as HTMLInputElement).value,
    ).toBe("2026-09-04");

    expect(
      (screen.getByLabelText("Edit session end date") as HTMLInputElement).value,
    ).toBe("2026-12-18");
  });

  it("rejects an end date earlier than the start date", () => {
    render(
      <SessionEditForm
        schoolSlug="morfol-demo"
        session={session}
        onUpdated={vi.fn()}
      />,
    );

    openEditor();
    fillDates("2026-09-04", "2026-08-31");
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(screen.getByRole("alert").textContent).toContain(
      "End date must be on or after the start date.",
    );
    expect(updateAcademicSession).not.toHaveBeenCalled();
  });

  it("saves within the selected school and reports success", async () => {
    const updated: AcademicSession = {
      ...session,
      ends_on: "2027-07-30",
    };

    vi.mocked(updateAcademicSession).mockResolvedValue(updated);
    const onUpdated = vi.fn();

    render(
      <SessionEditForm
        schoolSlug="morfol-demo"
        session={session}
        onUpdated={onUpdated}
      />,
    );

    openEditor();
    fillDates("2026-09-04", "2027-07-30");
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(onUpdated).toHaveBeenCalledWith(updated);
    });

    expect(updateAcademicSession).toHaveBeenCalledWith(
      "morfol-demo",
      "session-1",
      {
        name: "2026/2027",
        starts_on: "2026-09-04",
        ends_on: "2027-07-30",
      },
    );

    expect(screen.getByRole("status").textContent).toContain(
      "Academic session updated successfully.",
    );
  });

  it("displays server errors without claiming success", async () => {
    vi.mocked(updateAcademicSession).mockRejectedValue(
      new Error("Session dates conflict with academic terms."),
    );

    const onUpdated = vi.fn();

    render(
      <SessionEditForm
        schoolSlug="morfol-demo"
        session={session}
        onUpdated={onUpdated}
      />,
    );

    openEditor();
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(screen.getByRole("alert").textContent).toContain(
        "Session dates conflict with academic terms.",
      );
    });

    expect(onUpdated).not.toHaveBeenCalled();
    expect(screen.queryByText("Academic session updated successfully.")).toBeNull();
  });

  it("ignores a delayed response after switching to another school", async () => {
    let finish: ((value: AcademicSession) => void) | undefined;

    vi.mocked(updateAcademicSession).mockImplementation(
      () => new Promise<AcademicSession>((resolve) => {
        finish = resolve;
      }),
    );

    const onUpdated = vi.fn();
    const view = render(
      <SessionEditForm
        key="school-one:session-1"
        schoolSlug="school-one"
        session={session}
        onUpdated={onUpdated}
      />,
    );

    openEditor();
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(updateAcademicSession).toHaveBeenCalledWith(
      "school-one",
      "session-1",
      expect.any(Object),
    );

    view.rerender(
      <SessionEditForm
        key="school-two:session-2"
        schoolSlug="school-two"
        session={{ ...session, id: "session-2" }}
        onUpdated={onUpdated}
      />,
    );

    finish?.({ ...session, ends_on: "2027-07-30" });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Edit Session" })).toBeTruthy();
    });

    expect(onUpdated).not.toHaveBeenCalled();
  });
});