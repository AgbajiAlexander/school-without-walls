import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  activateAcademicSession,
  createAcademicSession,
  listAcademicSessions,
  listAcademicTerms,
} from "./academics-client";
import { getCSRFToken } from "@/lib/auth-client";

vi.mock("@/lib/auth-client", () => ({
  getCSRFToken: vi.fn(),
}));

const exampleSession = {
  id: "session-1",
  name: "2026/2027",
  starts_on: "2026-09-01",
  ends_on: "2027-07-31",
  status: "DRAFT",
  created_at: "2026-09-01T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
};

function jsonResponse(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("academic API client", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(getCSRFToken).mockReturnValue("fictional-csrf");
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("lists school-scoped sessions using the API envelope", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ data: { academic_sessions: [exampleSession] } }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const sessions = await listAcademicSessions("demo-school");

    expect(sessions).toHaveLength(1);
    expect(sessions[0].name).toBe("2026/2027");
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/tenants/demo-school/academic-sessions",
      expect.objectContaining({ credentials: "same-origin" }),
    );
  });

  it("loads terms for the selected session", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ data: { terms: [] } }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      listAcademicTerms("demo-school", "session-1"),
    ).resolves.toEqual([]);

    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/tenants/demo-school/academic-sessions/session-1/terms",
    );
  });

  it("includes the CSRF token for session creation", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ data: exampleSession }, 201),
    );
    vi.stubGlobal("fetch", fetchMock);

    await createAcademicSession("demo-school", {
      name: "2026/2027",
      starts_on: "2026-09-01",
      ends_on: "2027-07-31",
    });

    const options = fetchMock.mock.calls[0][1] as RequestInit;
    const headers = new Headers(options.headers);

    expect(options.method).toBe("POST");
    expect(headers.get("X-CSRF-Token")).toBe("fictional-csrf");
  });

  it("does not send mutations without a CSRF token", async () => {
    vi.mocked(getCSRFToken).mockReturnValue(null);
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      activateAcademicSession("demo-school", "session-1"),
    ).rejects.toThrow("Session verification is required");

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("propagates server authorization failures", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        error: {
          code: "FORBIDDEN",
          message: "Access denied",
        },
      }, 403),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      listAcademicSessions("demo-school"),
    ).rejects.toMatchObject({
      message: "Access denied",
      status: 403,
      code: "FORBIDDEN",
    });
  });

  it("rejects invalid academic response shapes", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse({ data: { unexpected: [] } }),
      ),
    );

    await expect(
      listAcademicSessions("demo-school"),
    ).rejects.toThrow("Invalid academic sessions response");
  });
});