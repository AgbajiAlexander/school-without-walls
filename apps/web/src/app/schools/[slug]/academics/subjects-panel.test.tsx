import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import SubjectsPanel from "./subjects-panel";
import {
  archiveSubject,
  createSubject,
  listSubjects,
  updateSubject,
  type SchoolSubject,
} from "@/lib/academics-client";

vi.mock("@/lib/academics-client", () => ({
  listSubjects: vi.fn(),
  createSubject: vi.fn(),
  updateSubject: vi.fn(),
  archiveSubject: vi.fn(),
}));

const subject: SchoolSubject = {
  id: "subject-1",
  code: "MATH",
  name: "Mathematics",
  description: "Core Mathematics",
  status: "ACTIVE",
  created_at: "2026-09-01T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
};

function renderPanel(role = "SCHOOL_ADMIN", slug = "school-one") {
  return render(<SubjectsPanel schoolSlug={slug} currentRole={role} />);
}

function fillCreateForm(code: string, name: string) {
  fireEvent.change(screen.getByLabelText("Subject code"), {
    target: { value: code },
  });
  fireEvent.change(screen.getByLabelText("Subject name"), {
    target: { value: name },
  });
}

describe("SubjectsPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listSubjects).mockResolvedValue([]);
  });

  afterEach(() => {
    cleanup();
  });

  it("shows subjects but no management controls to school owners", async () => {
    vi.mocked(listSubjects).mockResolvedValue([subject]);

    renderPanel("SCHOOL_OWNER");

    expect(await screen.findByText(/MATH.*Mathematics/)).toBeTruthy();
    expect(screen.getByText("Read-only access")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Add Subject" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Archive" })).toBeNull();
    expect(createSubject).not.toHaveBeenCalled();
  });

  it("creates and displays a new subject in the selected school", async () => {
    vi.mocked(createSubject).mockResolvedValue(subject);

    renderPanel();

    await screen.findByText("No subjects available for this school.");

    fillCreateForm("math", "Mathematics");
    fireEvent.click(screen.getByRole("button", { name: "Add Subject" }));

    await waitFor(() => {
      expect(createSubject).toHaveBeenCalledWith("school-one", {
        code: "MATH",
        name: "Mathematics",
        description: undefined,
      });
    });

    expect(await screen.findByText(/MATH.*Mathematics/)).toBeTruthy();
    expect(screen.getByText("Subject created successfully.")).toBeTruthy();
  });

  it("updates an existing subject through its school-scoped API", async () => {
    vi.mocked(listSubjects).mockResolvedValue([subject]);
    vi.mocked(updateSubject).mockResolvedValue({
      ...subject,
      name: "General Mathematics",
    });

    renderPanel("PRINCIPAL");

    await screen.findByText(/MATH.*Mathematics/);

    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    fireEvent.change(screen.getByLabelText("Edit subject name"), {
      target: { value: "General Mathematics" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save Subject" }));

    await waitFor(() => {
      expect(updateSubject).toHaveBeenCalledWith(
        "school-one",
        "subject-1",
        expect.objectContaining({
          code: "MATH",
          name: "General Mathematics",
        }),
      );
    });

    expect(await screen.findByText(/MATH.*General Mathematics/)).toBeTruthy();
    expect(screen.getByText("Subject updated successfully.")).toBeTruthy();
  });

  it("requires confirmation before archiving a subject", async () => {
    vi.mocked(listSubjects).mockResolvedValue([subject]);
    vi.mocked(archiveSubject).mockResolvedValue({
      ...subject,
      status: "ARCHIVED",
    });

    renderPanel();

    await screen.findByText(/MATH.*Mathematics/);

    fireEvent.click(screen.getByRole("button", { name: "Archive" }));

    expect(archiveSubject).not.toHaveBeenCalled();
    expect(screen.getByText(/Archive Mathematics\?/)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Confirm Archive" }));

    await waitFor(() => {
      expect(archiveSubject).toHaveBeenCalledWith("school-one", "subject-1");
    });

    expect(await screen.findByText("Subject archived successfully.")).toBeTruthy();
    expect(screen.getByText("ARCHIVED")).toBeTruthy();
  });

  it("displays server errors without claiming successful creation", async () => {
    vi.mocked(createSubject).mockRejectedValue(
      new Error("Subject code already exists."),
    );

    renderPanel();

    await screen.findByText("No subjects available for this school.");

    fillCreateForm("MATH", "Mathematics");
    fireEvent.click(screen.getByRole("button", { name: "Add Subject" }));

    await waitFor(() => {
      expect(screen.getByRole("alert").textContent).toContain(
        "Subject code already exists.",
      );
    });

    expect(screen.queryByText("Subject created successfully.")).toBeNull();
  });

  it("ignores a delayed creation response after switching schools", async () => {
    let resolveCreate: ((value: SchoolSubject) => void) | undefined;

    vi.mocked(createSubject).mockImplementation(
      () => new Promise<SchoolSubject>((resolve) => {
        resolveCreate = resolve;
      }),
    );

    const view = renderPanel("SCHOOL_ADMIN", "school-one");

    await screen.findByText("No subjects available for this school.");

    fillCreateForm("MATH", "Mathematics");
    fireEvent.click(screen.getByRole("button", { name: "Add Subject" }));

    await waitFor(() => {
      expect(createSubject).toHaveBeenCalledWith(
        "school-one",
        expect.any(Object),
      );
    });

    view.rerender(
      <SubjectsPanel schoolSlug="school-two" currentRole="SCHOOL_ADMIN" />,
    );

    await waitFor(() => {
      expect(listSubjects).toHaveBeenCalledWith("school-two");
    });

    await act(async () => {
      resolveCreate?.(subject);
    });

    expect(screen.queryByText(/MATH.*Mathematics/)).toBeNull();
    expect(screen.queryByText("Subject created successfully.")).toBeNull();
  });
  it("resets a pending creation when switching schools", async () => {
    vi.mocked(createSubject).mockImplementation(
      () => new Promise<SchoolSubject>(() => {}),
    );

    const view = renderPanel();

    await screen.findByText("No subjects available for this school.");

    fillCreateForm("MATH", "Mathematics");
    fireEvent.click(screen.getByRole("button", { name: "Add Subject" }));

    await waitFor(() => {
      expect(createSubject).toHaveBeenCalledTimes(1);
    });

    expect(
      (screen.getByRole("button", { name: "Saving..." }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);

    view.rerender(
      <SubjectsPanel schoolSlug="school-two" currentRole="SCHOOL_ADMIN" />,
    );

    await waitFor(() => {
      expect(listSubjects).toHaveBeenCalledWith("school-two");
    });

    await waitFor(() => {
      expect(
        (screen.getByRole("button", { name: "Add Subject" }) as HTMLButtonElement)
          .disabled,
      ).toBe(false);
    });

    expect(
      (screen.getByLabelText("Subject code") as HTMLInputElement).value,
    ).toBe("");
    expect(
      (screen.getByLabelText("Subject name") as HTMLInputElement).value,
    ).toBe("");
  });

  it("ignores a delayed subject edit after changing schools", async () => {
    let finishUpdate: ((value: SchoolSubject) => void) | undefined;

    vi.mocked(listSubjects).mockImplementation(
      async (slug) => slug === "school-one" ? [subject] : [],
    );

    vi.mocked(updateSubject).mockImplementation(
      () => new Promise<SchoolSubject>((resolve) => {
        finishUpdate = resolve;
      }),
    );

    const view = renderPanel();

    await screen.findByText(/MATH.*Mathematics/);

    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    fireEvent.change(screen.getByLabelText("Edit subject name"), {
      target: { value: "Advanced Mathematics" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save Subject" }));

    await waitFor(() => {
      expect(updateSubject).toHaveBeenCalledTimes(1);
    });

    view.rerender(
      <SubjectsPanel schoolSlug="school-two" currentRole="SCHOOL_ADMIN" />,
    );

    await screen.findByText("No subjects available for this school.");

    await act(async () => {
      finishUpdate?.({ ...subject, name: "Advanced Mathematics" });
    });

    expect(screen.queryByText(/Advanced Mathematics/)).toBeNull();
    expect(screen.queryByText("Subject updated successfully.")).toBeNull();
  });

  it("ignores a delayed archive after changing schools", async () => {
    let finishArchive: ((value: SchoolSubject) => void) | undefined;

    vi.mocked(listSubjects).mockImplementation(
      async (slug) => slug === "school-one" ? [subject] : [],
    );

    vi.mocked(archiveSubject).mockImplementation(
      () => new Promise<SchoolSubject>((resolve) => {
        finishArchive = resolve;
      }),
    );

    const view = renderPanel();

    await screen.findByText(/MATH.*Mathematics/);

    fireEvent.click(screen.getByRole("button", { name: "Archive" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm Archive" }));

    await waitFor(() => {
      expect(archiveSubject).toHaveBeenCalledTimes(1);
    });

    view.rerender(
      <SubjectsPanel schoolSlug="school-two" currentRole="SCHOOL_ADMIN" />,
    );

    await screen.findByText("No subjects available for this school.");

    await act(async () => {
      finishArchive?.({ ...subject, status: "ARCHIVED" });
    });

    expect(screen.queryByText("Subject archived successfully.")).toBeNull();
    expect(screen.queryByText("ARCHIVED")).toBeNull();
  });
});