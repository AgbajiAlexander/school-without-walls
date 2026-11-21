import { headers } from "next/headers";
import { redirect } from "next/navigation";

export type LessonSummary = { id: string; title: string; subject: string; objective_code: string };

export async function readStudentLessons<T>(slug: string, suffix = ""): Promise<T | null> {
  return readLearningApi<T>(`/api/v1/tenants/${encodeURIComponent(slug)}/learning/lessons${suffix}`);
}

export function readLessonSession() {
  return readLearningApi<{ csrf_token: string }>("/api/v1/auth/me");
}

async function readLearningApi<T>(path: string): Promise<T | null> {
  const cookie = (await headers()).get("cookie");
  if (!cookie) redirect("/login");
  let response: Response;
  try {
    response = await fetch(new URL(
      path,
      process.env.API_INTERNAL_URL || "http://localhost:8080",
    ), {
      headers: { Accept: "application/json", Cookie: cookie },
      cache: "no-store",
      redirect: "error",
    });
  } catch {
    return null;
  }
  if (response.status === 401) redirect("/login");
  if (!response.ok) return null;
  const payload = await response.json().catch(() => null) as { data?: T } | null;
  return payload?.data ?? null;
}
