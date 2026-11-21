import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import StudentClient from "./student-client";

const refresh = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));

const props = {
  slug: "fictional-school",
  csrfToken: "fixture-csrf",
  students: [{ id: "student-a", display_name: "Fictional Learner", admission_no: "P-1", stage: "PRIMARY", status: "ACTIVE" }] as const,
};

afterEach(() => {
  cleanup();
  refresh.mockClear();
  vi.unstubAllGlobals();
});

it("registers a trimmed profile using the active school and CSRF", async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 201 }));
  vi.stubGlobal("fetch", fetcher);
  render(<StudentClient {...props} students={[...props.students]} />);
  fireEvent.change(screen.getByLabelText("Admission number"), { target: { value: " P-2 " } });
  fireEvent.change(screen.getByLabelText("Display name"), { target: { value: " Fictional New Learner " } });
  fireEvent.click(screen.getByRole("button", { name: "Save student" }));
  expect(await screen.findByText("Student registered.")).toBeVisible();
  expect(fetcher).toHaveBeenCalledExactlyOnceWith("/api/v1/tenants/fictional-school/students", expect.objectContaining({
    method: "POST", credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": "fixture-csrf" },
    body: JSON.stringify({ admission_no: "P-2", display_name: "Fictional New Learner", stage: "PRIMARY" }),
  }));
  expect(refresh).toHaveBeenCalledOnce();
});

it("edits the selected profile and hides conflict diagnostics", async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response("internal fixture details", { status: 409 }));
  vi.stubGlobal("fetch", fetcher);
  render(<StudentClient {...props} students={[...props.students]} />);
  fireEvent.click(screen.getByRole("button", { name: "Edit Fictional Learner" }));
  fireEvent.change(screen.getByLabelText("Admission number"), { target: { value: "P-2" } });
  fireEvent.click(screen.getByRole("button", { name: "Save student" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("already in use");
  expect(screen.queryByText("internal fixture details")).not.toBeInTheDocument();
  expect(fetcher).toHaveBeenCalledWith("/api/v1/tenants/fictional-school/students/student-a", expect.objectContaining({ method: "PUT" }));
});

it("prevents further writes after a server denial", async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 404 }));
  vi.stubGlobal("fetch", fetcher);
  render(<StudentClient {...props} students={[...props.students]} />);
  fireEvent.click(screen.getByRole("button", { name: "Edit Fictional Learner" }));
  const save = screen.getByRole("button", { name: "Save student" });
  fireEvent.click(save);
  expect(await screen.findByRole("alert")).toHaveTextContent("school access");
  expect(save).toBeDisabled();
  fireEvent.click(save);
  expect(fetcher).toHaveBeenCalledOnce();
});