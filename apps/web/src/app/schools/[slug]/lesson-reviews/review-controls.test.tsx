/** @vitest-environment jsdom */

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import ReviewControls from "./review-controls";

vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn() }) }));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("requests an attachment only for the displayed review revision", async () => {
  const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 404 }));
  vi.stubGlobal("fetch", fetch);
  const base = "/api/v1/tenants/fictional-school";
  const item = { id: "draft-fixture", title: "Scanned practice", content: "Practice on paper.", revision: 7 };
  const { rerender } = render(<ReviewControls base={base} csrf="fixture" required canConfigure={false}
    items={[{ ...item, has_attachment: true }, { ...item, id: "text-only", title: "Text practice" }]} />);
  const review = screen.getByRole("heading", { name: item.title }).closest("article")!;
  fireEvent.click(within(review).getByRole("button", { name: "Download lesson attachment" }));
  expect((await within(review).findByRole("alert")).textContent).toBe("The attachment is unavailable. Try again later.");
  expect(fetch).toHaveBeenCalledExactlyOnceWith(base + "/lesson-reviews/draft-fixture/media?revision=7", expect.objectContaining({
    credentials: "same-origin", redirect: "error", cache: "no-store",
  }));
  expect(screen.getAllByRole("button", { name: "Download lesson attachment" })).toHaveLength(1);

  fireEvent.click(within(review).getByRole("button", { name: "Review lesson" }));
  expect(within(review).getByRole("button", { name: "Confirm approval" })).toBeDefined();

  rerender(<ReviewControls base={base} csrf="fixture" required canConfigure={false}
    items={[{ ...item, revision: 8, has_attachment: true }]} />);
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.queryByRole("button", { name: "Confirm approval" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Download lesson attachment" }));
  expect((await screen.findByRole("alert")).textContent).toBe("The attachment is unavailable. Try again later.");
  expect(fetch).toHaveBeenLastCalledWith(base + "/lesson-reviews/draft-fixture/media?revision=8", expect.anything());
});
