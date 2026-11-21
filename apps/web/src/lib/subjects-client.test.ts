import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getCSRFToken } from "@/lib/auth-client";
import {
  listSubjects,
  createSubject,
  updateSubject,
  archiveSubject,
  type SchoolSubject,
} from "./academics-client";

vi.mock("@/lib/auth-client", () => ({
  getCSRFToken: vi.fn(),
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

function apiResponse(data: unknown) {
  return new Response(JSON.stringify({ data }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("Subject Management API", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(getCSRFToken).mockReturnValue("test-csrf-token");
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("lists subjects in the selected school", async () => {
    const fetchMock = vi.fn().mockResolvedValue(apiResponse([subject]));
    vi.stubGlobal("fetch", fetchMock);

    const subjects = await listSubjects("morfol-demo");

    expect(subjects).toEqual([subject]);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/tenants/morfol-demo/subjects",
      expect.objectContaining({
        credentials: "same-origin",
        cache: "no-store",
      }),
    );
  });

  it("creates a subject with CSRF protection", async () => {
    const fetchMock = vi.fn().mockResolvedValue(apiResponse(subject));
    vi.stubGlobal("fetch", fetchMock);

    const result = await createSubject("morfol-demo", {
      code: "MATH",
      name: "Mathematics",
    });

    expect(result).toEqual(subject);

    const [url, options] = fetchMock.mock.calls[0];

    expect(url).toBe("/api/v1/tenants/morfol-demo/subjects");
    expect(options.method).toBe("POST");
    expect(options.headers.get("X-CSRF-Token")).toBe("test-csrf-token");
    expect(JSON.parse(options.body)).toEqual({
      code: "MATH",
      name: "Mathematics",
    });
  });

  it("updates a subject within the specified school", async () => {
    const updated = { ...subject, name: "General Mathematics" };
    const fetchMock = vi.fn().mockResolvedValue(apiResponse(updated));
    vi.stubGlobal("fetch", fetchMock);

    const result = await updateSubject(
      "morfol-demo",
      "subject-1",
      { name: "General Mathematics" },
    );

    expect(result.name).toBe("General Mathematics");

    const [url, options] = fetchMock.mock.calls[0];

    expect(url).toBe(
      "/api/v1/tenants/morfol-demo/subjects/subject-1",
    );
    expect(options.method).toBe("PATCH");
    expect(options.headers.get("X-CSRF-Token")).toBe("test-csrf-token");
  });

  it("archives a subject through its school-scoped endpoint", async () => {
    const archived = { ...subject, status: "ARCHIVED" as const };
    const fetchMock = vi.fn().mockResolvedValue(apiResponse(archived));
    vi.stubGlobal("fetch", fetchMock);

    const result = await archiveSubject("morfol-demo", "subject-1");

    expect(result.status).toBe("ARCHIVED");

    const [url, options] = fetchMock.mock.calls[0];

    expect(url).toBe(
      "/api/v1/tenants/morfol-demo/subjects/subject-1/archive",
    );
    expect(options.method).toBe("POST");
    expect(options.headers.get("X-CSRF-Token")).toBe("test-csrf-token");
  });

  it("rejects malformed subject-list responses", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(apiResponse({ subjects: [subject] })),
    );

    await expect(listSubjects("morfol-demo")).rejects.toThrow(
      "Invalid subjects response.",
    );
  });

  it("blocks mutations when the CSRF token is missing", async () => {
    vi.mocked(getCSRFToken).mockReturnValue(null);
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      createSubject("morfol-demo", {
        code: "ENG",
        name: "English Language",
      }),
    ).rejects.toThrow("Session verification is required");

    expect(fetchMock).not.toHaveBeenCalled();
  });
});