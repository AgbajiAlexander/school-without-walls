import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";
import TermConfigureForm from "./term-configure-form";
import {
  configureAcademicTerms,
  type AcademicSession,
  type AcademicTerm,
} from "@/lib/academics-client";

vi.mock("@/lib/academics-client", () => ({
  configureAcademicTerms: vi.fn(),
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

const dates = [
  ["2026-09-07", "2026-12-18"],
  ["2027-01-11", "2027-04-09"],
  ["2027-04-26", "2027-07-23"],
] as const;

const names = ["First Term", "Second Term", "Third Term"];

function fillValidDates() {
  names.forEach((name, index) => {
    fireEvent.change(screen.getByLabelText(`${name} start date`), {
      target: { value: dates[index][0] },
    });

    fireEvent.change(screen.getByLabelText(`${name} end date`), {
      target: { value: dates[index][1] },
    });
  });
}

function submitForm() {
  const form = screen
    .getByRole("button", { name: "Save three terms" })
    .closest("form");

  if (!form) {
    throw new Error("Academic term form not found.");
  }

  fireEvent.submit(form);
}

describe("TermConfigureForm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders three term sections for draft sessions", () => {
    render(
      <TermConfigureForm
        schoolSlug="demo-school"
        session={session}
        existingTerms={[]}
        onSaved={vi.fn()}
      />,
    );

    for (const name of names) {
      expect(screen.getByLabelText(`${name} name`)).toBeRequired();
      expect(screen.getByLabelText(`${name} start date`)).toBeRequired();
      expect(screen.getByLabelText(`${name} end date`)).toBeRequired();
    }

    expect(screen.getByRole("button", { name: "Save three terms" }))
      .toBeInTheDocument();
  });

  it("hides editing for an archived session", () => {
    render(
      <TermConfigureForm
        schoolSlug="demo-school"
        session={{ ...session, status: "ARCHIVED" }}
        existingTerms={[]}
        onSaved={vi.fn()}
      />,
    );

    expect(screen.queryByRole("button", { name: "Save three terms" }))
      .not.toBeInTheDocument();

    expect(configureAcademicTerms).not.toHaveBeenCalled();
  });

  it("rejects missing dates", async () => {
    render(
      <TermConfigureForm
        schoolSlug="demo-school"
        session={session}
        existingTerms={[]}
        onSaved={vi.fn()}
      />,
    );

    submitForm();

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("Complete the name and dates for all three terms");

    expect(configureAcademicTerms).not.toHaveBeenCalled();
  });

  it("rejects term dates outside the academic session", async () => {
    render(
      <TermConfigureForm
        schoolSlug="demo-school"
        session={session}
        existingTerms={[]}
        onSaved={vi.fn()}
      />,
    );

    fillValidDates();

    fireEvent.change(screen.getByLabelText("Third Term end date"), {
      target: { value: "2027-08-01" },
    });

    submitForm();

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("dates must fall within the academic session");

    expect(configureAcademicTerms).not.toHaveBeenCalled();
  });

  it("rejects overlapping academic terms", async () => {
    render(
      <TermConfigureForm
        schoolSlug="demo-school"
        session={session}
        existingTerms={[]}
        onSaved={vi.fn()}
      />,
    );

    fillValidDates();

    fireEvent.change(screen.getByLabelText("Second Term start date"), {
      target: { value: "2026-12-01" },
    });

    submitForm();

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("must not overlap");

    expect(configureAcademicTerms).not.toHaveBeenCalled();
  });

  it("submits all three valid terms to the selected school", async () => {
    const onSaved = vi.fn();

    const savedTerms: AcademicTerm[] = names.map((name, index) => ({
      id: `term-${index + 1}`,
      session_id: session.id,
      sequence: index + 1,
      name,
      starts_on: dates[index][0],
      ends_on: dates[index][1],
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    }));

    vi.mocked(configureAcademicTerms).mockResolvedValue(savedTerms);

    render(
      <TermConfigureForm
        schoolSlug="demo-school"
        session={session}
        existingTerms={[]}
        onSaved={onSaved}
      />,
    );

    fillValidDates();
    submitForm();

    await waitFor(() => {
      expect(onSaved).toHaveBeenCalledWith(savedTerms);
    });

    expect(configureAcademicTerms).toHaveBeenCalledWith(
      "demo-school",
      "session-1",
      [
        {
          sequence: 1,
          name: "First Term",
          starts_on: "2026-09-07",
          ends_on: "2026-12-18",
        },
        {
          sequence: 2,
          name: "Second Term",
          starts_on: "2027-01-11",
          ends_on: "2027-04-09",
        },
        {
          sequence: 3,
          name: "Third Term",
          starts_on: "2027-04-26",
          ends_on: "2027-07-23",
        },
      ],
    );
  });

  it("shows backend errors without reporting success", async () => {
    const onSaved = vi.fn();

    vi.mocked(configureAcademicTerms).mockRejectedValue(
      new Error("Academic term configuration conflicts with existing data"),
    );

    render(
      <TermConfigureForm
        schoolSlug="demo-school"
        session={session}
        existingTerms={[]}
        onSaved={onSaved}
      />,
    );

    fillValidDates();
    submitForm();

    expect(await screen.findByRole("alert"))
      .toHaveTextContent("configuration conflicts");

    expect(onSaved).not.toHaveBeenCalled();
  });
});