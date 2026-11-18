import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getCSRFToken } from "@/lib/auth-client";
import {
  archiveClass,
  archiveClassArm,
  archiveGradeLevel,
  createClass,
  createClassArm,
  createGradeLevel,
  listClassArms,
  listClasses,
  listGradeLevels,
  updateClass,
  updateClassArm,
  updateGradeLevel,
} from "./structure-client";

vi.mock("@/lib/auth-client", () => ({
  getCSRFToken: vi.fn(),
}));

function jsonResponse(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const level = {
  id: "level-1",
  stage: "PRIMARY",
  code: "PRI_3",
  name: "Primary 3",
  sort_order: 30,
  status: "ACTIVE",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const schoolClass = {
  id: "class-1",
  session_id: "session-1",
  grade_level_id: "level-1",
  code: "PRI_3_2026",
  name: "Primary 3",
  status: "ACTIVE",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const arm = {
  id: "arm-1",
  class_id: "class-1",
  code: "A",
  name: "Arm A",
  status: "ACTIVE",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("academic structure API client", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(getCSRFToken).mockReturnValue("test-csrf");
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("lists school-scoped grade levels", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ data: { grade_levels: [level] } }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(listGradeLevels("demo-school")).resolves.toEqual([level]);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/tenants/demo-school/grade-levels",
      expect.objectContaining({ credentials: "same-origin" }),
    );
  });

  it("requires session_id when listing classes", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ data: { classes: [schoolClass] } }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      listClasses("demo-school", "session-1"),
    ).resolves.toEqual([schoolClass]);

    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/tenants/demo-school/classes?session_id=session-1",
    );
  });

  it("requires class_id when listing class arms", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ data: { class_arms: [arm] } }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      listClassArms("demo-school", "class-1"),
    ).resolves.toEqual([arm]);

    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/tenants/demo-school/class-arms?class_id=class-1",
    );
  });

  it("creates a grade level with CSRF protection", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ data: level }, 201),
    );
    vi.stubGlobal("fetch", fetchMock);

    await createGradeLevel("demo-school", {
      stage: "PRIMARY",
      code: "PRI_3",
      name: "Primary 3",
      sort_order: 30,
    });

    const [path, options] = fetchMock.mock.calls[0] as [
      string,
      RequestInit,
    ];

    expect(path).toBe("/api/v1/tenants/demo-school/grade-levels");
    expect(options.method).toBe("POST");
    expect(new Headers(options.headers).get("X-CSRF-Token"))
      .toBe("test-csrf");

    expect(JSON.parse(options.body as string)).toEqual({
      stage: "PRIMARY",
      code: "PRI_3",
      name: "Primary 3",
      sort_order: 30,
    });
  });

  it("creates classes and class arms with their required parents", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ data: schoolClass }, 201))
      .mockResolvedValueOnce(jsonResponse({ data: arm }, 201));
    vi.stubGlobal("fetch", fetchMock);

    await createClass("demo-school", {
      session_id: "session-1",
      grade_level_id: "level-1",
      code: "PRI_3_2026",
      name: "Primary 3",
    });

    await createClassArm("demo-school", {
      class_id: "class-1",
      code: "A",
      name: "Arm A",
    });

    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/tenants/demo-school/classes",
    );
    expect(fetchMock.mock.calls[1][0]).toBe(
      "/api/v1/tenants/demo-school/class-arms",
    );

    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toMatchObject({
      session_id: "session-1",
      grade_level_id: "level-1",
    });

    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toMatchObject({
      class_id: "class-1",
    });
  });

  it("uses PATCH for all three update operations", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ data: level }))
      .mockResolvedValueOnce(jsonResponse({ data: schoolClass }))
      .mockResolvedValueOnce(jsonResponse({ data: arm }));
    vi.stubGlobal("fetch", fetchMock);

    await updateGradeLevel("demo-school", "level-1", {
      name: "Primary Three",
    });
    await updateClass("demo-school", "class-1", {
      name: "Primary Three",
    });
    await updateClassArm("demo-school", "arm-1", {
      name: "Blue",
    });

    expect(fetchMock.mock.calls.map((call) => call[1].method))
      .toEqual(["PATCH", "PATCH", "PATCH"]);

    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      "/api/v1/tenants/demo-school/grade-levels/level-1",
      "/api/v1/tenants/demo-school/classes/class-1",
      "/api/v1/tenants/demo-school/class-arms/arm-1",
    ]);
  });

  it("archives all three resource types via POST", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ data: { ...level, status: "ARCHIVED" } }))
      .mockResolvedValueOnce(jsonResponse({ data: { ...schoolClass, status: "ARCHIVED" } }))
      .mockResolvedValueOnce(jsonResponse({ data: { ...arm, status: "ARCHIVED" } }));
    vi.stubGlobal("fetch", fetchMock);

    await archiveGradeLevel("demo-school", "level-1");
    await archiveClass("demo-school", "class-1");
    await archiveClassArm("demo-school", "arm-1");

    expect(fetchMock.mock.calls.map((call) => call[1].method))
      .toEqual(["POST", "POST", "POST"]);

    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      "/api/v1/tenants/demo-school/grade-levels/level-1/archive",
      "/api/v1/tenants/demo-school/classes/class-1/archive",
      "/api/v1/tenants/demo-school/class-arms/arm-1/archive",
    ]);
  });

  it("refuses mutation requests without a CSRF token", async () => {
    vi.mocked(getCSRFToken).mockReturnValue(null);
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      archiveGradeLevel("demo-school", "level-1"),
    ).rejects.toThrow("Session verification is required");

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("propagates authorization failures", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse(
        { error: { code: "FORBIDDEN", message: "Access denied" } },
        403,
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(listGradeLevels("demo-school")).rejects.toMatchObject({
      status: 403,
      code: "FORBIDDEN",
      message: "Access denied",
    });
  });

  it("rejects malformed listing responses", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse({ data: { classes: null } }),
      ),
    );

    await expect(
      listClasses("demo-school", "session-1"),
    ).rejects.toThrow("Invalid classes response");
  });

  it("rejects empty parent identifiers", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    expect(() =>
      listClasses("demo-school", ""),
    ).toThrow("Session ID is required");

    expect(() =>
      listClassArms("demo-school", ""),
    ).toThrow("Class ID is required");

    expect(fetchMock).not.toHaveBeenCalled();
  });
});