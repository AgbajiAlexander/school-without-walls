/** @vitest-environment jsdom */

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import AssignmentSubmission from "./assignment-submission";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("clears the previous feedback when a new submission revision is accepted", async () => {
  const date = "2026-11-22T08:00:00Z";
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json({
    data: { revision: 2, submitted_at: date },
  })));
  render(<AssignmentSubmission endpoint="/api/fictional" csrf="fixture" available resubmit
    receipt={{ revision: 1, submitted_at: date }}
    feedback={{ version: 1, score: 7, comment: "Feedback for revision one." }} maxScore={10} />);
  expect(screen.getByText("Score 7 out of 10")).toBeDefined();
  fireEvent.change(screen.getByLabelText("Your answer"), { target: { value: "Fictional revised answer" } });
  fireEvent.submit(screen.getByRole("button", { name: "Resubmit answer" }).closest("form")!);
  await screen.findByText(/Submission 2 received on/);
  expect(screen.queryByText("Score 7 out of 10")).toBeNull();
});
