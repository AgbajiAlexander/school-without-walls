import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import AcademicManagementPage from "./page";

const { mockHeaders, mockRedirect } = vi.hoisted(() => ({
  mockHeaders: vi.fn(),
  mockRedirect: vi.fn(),
}));

vi.mock("next/headers", () => ({
  headers: mockHeaders,
}));

vi.mock("next/navigation", () => ({
  redirect: mockRedirect,
}));

vi.mock("./academics-client", () => ({
  default: ({
    school,
    csrfToken,
  }: {
    school: { slug: string };
    csrfToken: string;
  }) => (
    <div data-testid="verified-academic-dashboard">
      {school.slug}:{csrfToken}
    </div>
  ),
}));

function response(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function mockAPI(
  meStatus: number,
  schoolStatus: number,
  schoolSlug = "demo-school",
) {
  const fetchMock = vi.fn()
    .mockResolvedValueOnce(response({
      data: { csrf_token: "fictional-csrf" },
    }, meStatus))
    .mockResolvedValueOnce(response({
      data: {
        id: "school-id",
        slug: schoolSlug,
        name: "Demo School",
        current_role: "SCHOOL_ADMIN",
      },
    }, schoolStatus));

  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

async function renderPage(slug = "demo-school") {
  const component = await AcademicManagementPage({
    params: Promise.resolve({ slug }),
  });

  render(component);
}

describe("Academic Management server page security", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    mockHeaders.mockResolvedValue({
      get: () => "fictional-session-cookie",
    });

    mockRedirect.mockImplementation((path: string) => {
      throw new Error(`REDIRECT:${path}`);
    });
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("redirects unauthenticated visitors before requesting school data", async () => {
    mockHeaders.mockResolvedValue({ get: () => null });
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(renderPage()).rejects.toThrow("REDIRECT:/login");

    expect(fetchMock).not.toHaveBeenCalled();
    expect(mockRedirect).toHaveBeenCalledWith("/login");
  });

  it("redirects when the backend rejects the session", async () => {
    mockAPI(401, 200);

    await expect(renderPage()).rejects.toThrow("REDIRECT:/login");

    expect(mockRedirect).toHaveBeenCalledWith("/login");
  });

  it("denies access when school membership is forbidden", async () => {
    mockAPI(200, 403);

    await renderPage();

    expect(screen.getByText("School access unavailable")).toBeInTheDocument();
    expect(screen.queryByTestId("verified-academic-dashboard")).not.toBeInTheDocument();
  });

  it("rejects school data that does not match the requested slug", async () => {
    mockAPI(200, 200, "another-school");

    await renderPage();

    expect(screen.getByText("School verification incomplete")).toBeInTheDocument();
    expect(screen.queryByTestId("verified-academic-dashboard")).not.toBeInTheDocument();
  });

  it("renders the dashboard only with a verified school context", async () => {
    const fetchMock = mockAPI(200, 200);

    await renderPage();

    expect(screen.getByTestId("verified-academic-dashboard"))
      .toHaveTextContent("demo-school:fictional-csrf");

    expect(fetchMock).toHaveBeenCalledTimes(2);

    const schoolRequest = fetchMock.mock.calls[1][0] as URL;
    expect(schoolRequest.pathname).toBe(
      "/api/v1/tenants/demo-school/settings",
    );
  });

  it("shows an error instead of exposing data when the backend fails", async () => {
    mockAPI(200, 500);

    await renderPage();

    expect(screen.getByText("Unable to verify school")).toBeInTheDocument();
    expect(screen.queryByTestId("verified-academic-dashboard")).not.toBeInTheDocument();
  });
});