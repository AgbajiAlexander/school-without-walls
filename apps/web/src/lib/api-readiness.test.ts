import { afterEach, describe, expect, it, vi } from "vitest";
import { checkApiReadiness } from "./api-readiness";

describe("checkApiReadiness", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("reports ready only when the Go readiness response is healthy", async () => {
    const fetcher: typeof fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ data: { status: "ready" } }), { status: 200 }),
    );

    await expect(checkApiReadiness("http://api.internal", 200, fetcher)).resolves.toBe("ready");
    expect(fetcher).toHaveBeenCalledWith(
      new URL("/ready", "http://api.internal"),
      expect.objectContaining({ cache: "no-store" }),
    );
  });

  it("sanitizes non-ready backend responses", async () => {
    const fetcher: typeof fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({
        error: {
          message: "Database unavailable at postgres.internal",
          details: { password: "must-not-escape" },
        },
      }), { status: 503 }),
    );

    await expect(checkApiReadiness("http://api.internal", 200, fetcher)).resolves.toBe("unavailable");
  });

  it("returns timeout when the bounded request is aborted", async () => {
    vi.useFakeTimers();
    const fetcher: typeof fetch = vi.fn(
      (_input: RequestInfo | URL, init?: RequestInit): Promise<Response> =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () => {
            reject(new DOMException("Aborted", "AbortError"));
          }, { once: true });
        }),
    );

    const result = checkApiReadiness("http://api.internal", 100, fetcher);
    await vi.advanceTimersByTimeAsync(100);
    await expect(result).resolves.toBe("timeout");
  });
});
