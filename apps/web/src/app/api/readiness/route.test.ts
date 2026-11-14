import { afterEach, describe, expect, it, vi } from "vitest";
import { GET } from "./route";

describe("GET /api/readiness", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
    vi.useRealTimers();
  });

  it("returns a sanitized no-store ready status", async () => {
    vi.stubEnv("API_INTERNAL_URL", "http://api.internal");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ data: { status: "ready" } }), { status: 200 }),
    ));

    const response = await GET();

    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toContain("no-store");
    await expect(response.json()).resolves.toEqual({ status: "ready" });
    expect(fetch).toHaveBeenCalledWith(
      new URL("/ready", "http://api.internal"),
      expect.objectContaining({ cache: "no-store" }),
    );
  });

  it("does not relay database errors or connection details", async () => {
    vi.stubEnv("API_INTERNAL_URL", "http://api.internal");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response(JSON.stringify({
        error: { message: "postgres.internal password=secret" },
      }), { status: 503 }),
    ));

    const response = await GET();

    expect(response.status).toBe(503);
    await expect(response.json()).resolves.toEqual({ status: "unavailable" });
  });

  it("maps an upstream timeout to a generic 504 response", async () => {
    vi.useFakeTimers();
    vi.stubEnv("API_INTERNAL_URL", "http://api.internal");
    vi.stubGlobal("fetch", vi.fn(
      (_input: RequestInfo | URL, init?: RequestInit): Promise<Response> =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () => {
            reject(new DOMException("Aborted", "AbortError"));
          }, { once: true });
        }),
    ));

    const result = GET();
    await vi.advanceTimersByTimeAsync(2000);
    const response = await result;

    expect(response.status).toBe(504);
    await expect(response.json()).resolves.toEqual({ status: "timeout" });
  });
});
