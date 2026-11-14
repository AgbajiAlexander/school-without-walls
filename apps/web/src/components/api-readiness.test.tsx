import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import ApiReadiness from "./api-readiness";

describe("ApiReadiness", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows a checking state until the same-origin readiness request completes", async () => {
    let resolveRequest!: (response: Response) => void;
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>((resolve) => {
      resolveRequest = resolve;
    })));

    render(<ApiReadiness />);
    expect(screen.getByRole("status")).toHaveTextContent("Checking API");
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");

    resolveRequest(new Response(JSON.stringify({ status: "ready" }), { status: 200 }));
    expect(await screen.findByRole("status")).toHaveTextContent("API ready");
  });

  it("shows an unavailable state without infrastructure details", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ status: "unavailable" }), { status: 503 }),
    ));

    render(<ApiReadiness />);
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("API unavailable"));
    expect(screen.getByRole("status")).not.toHaveTextContent("postgres");
  });

  it("shows a timeout state", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ status: "timeout" }), { status: 504 }),
    ));

    render(<ApiReadiness />);
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Request timed out"));
  });

  it("shows unavailable when the readiness endpoint cannot be reached", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("network failure")));

    render(<ApiReadiness />);
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("API unavailable"));
  });
});
