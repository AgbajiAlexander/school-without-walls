/** @vitest-environment jsdom */

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import LessonDownload from "./lesson-download";

const endpoint = "/api/v1/tenants/fictional/learning/lessons/lesson-id/media";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("lesson attachment download", () => {
  it("requests a fresh grant and opens it without sending a referrer", async () => {
    const url = "https://objects.example.test/ready/example?signature=fixture";
    const fetch = vi.fn().mockResolvedValue(Response.json({ data: { download: { url } } }));
    vi.stubGlobal("fetch", fetch);
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
      expect(this.href).toBe(url);
      expect(this.rel).toBe("noopener noreferrer");
      expect(this.target).toBe("");
    });

    render(<LessonDownload endpoint={endpoint} />);
    fireEvent.click(screen.getByRole("button", { name: "Download lesson attachment" }));
    await waitFor(() => expect(click).toHaveBeenCalledOnce());
    expect(fetch).toHaveBeenCalledWith(endpoint, {
      cache: "no-store",
      credentials: "same-origin",
      redirect: "error",
    });
    expect(document.querySelector("a")).toBeNull();
  });

  it.each([
    new Response("private diagnostic", { status: 403 }),
    Response.json({ data: { download: { url: "javascript:alert(1)" } } }),
    Response.json({ data: { download: { url: "https://" + window.location.hostname + ":9443/ready/fixture" } } }),
  ])("shows a generic failure without opening an unsafe or denied grant", async (response) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response));
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    render(<LessonDownload endpoint={endpoint} />);
    fireEvent.click(screen.getByRole("button", { name: "Download lesson attachment" }));
    expect((await screen.findByRole("alert")).textContent).toBe("The attachment is unavailable. Try again later.");
    expect(click).not.toHaveBeenCalled();
  });
});
