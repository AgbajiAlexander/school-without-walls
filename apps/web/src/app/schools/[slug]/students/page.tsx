import Link from "next/link";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import StudentClient, { type StudentRecord } from "./student-client";

export default async function StudentsPage({ params, searchParams }: {
  params: Promise<{ slug: string }>;
  searchParams: Promise<{ after?: string }>;
}) {
  const { slug } = await params;
  const { after } = await searchParams;
  const cookie = (await headers()).get("cookie");
  if (!cookie) redirect("/login");

  const root = `/schools/${encodeURIComponent(slug)}/students`;
  const endpoint = `/api/v1/tenants/${encodeURIComponent(slug)}/students`;
  const base = process.env.API_INTERNAL_URL || "http://localhost:8080";
  const get = async (path: string) => fetch(new URL(path, base), {
    headers: { Accept: "application/json", Cookie: cookie },
    cache: "no-store",
    redirect: "error",
  });
  const results = await Promise.all([
    get("/api/v1/auth/me"),
    get(after ? `${endpoint}?${new URLSearchParams({ after })}` : endpoint),
  ]).catch(() => null);
  if (results?.some((result) => result.status === 401)) redirect("/login");
  const payload = results && results.every((result) => result.ok) ? await Promise.all([
    results[0].json() as Promise<{ data?: { csrf_token?: string } }>,
    results[1].json() as Promise<{ data?: { students?: StudentRecord[]; next_cursor?: string } }>,
  ]).catch(() => null) : null;
  const csrf = payload?.[0].data?.csrf_token;
  const page = payload?.[1].data;

  return (
    <main className="mx-auto w-full max-w-4xl space-y-6 px-4 py-8 sm:px-6">
      <Link href="/dashboard" className="text-sm font-medium text-green-800 underline">Back to My Schools</Link>
      <h1 className="text-2xl font-semibold text-slate-900">Students</h1>
      {typeof csrf === "string" && csrf && Array.isArray(page?.students) ? (
        <>
        <Link href={`/schools/${encodeURIComponent(slug)}/guardians`} className="text-green-800 underline">Guardian directory</Link>
        <StudentClient
          key={root}
          slug={slug}
          csrfToken={csrf}
          students={page.students}
          nextPage={page.next_cursor ? `${root}?${new URLSearchParams({ after: page.next_cursor })}` : undefined}
          firstPage={after ? root : undefined}
        />
        </>
      ) : (
        <p role="status" className="rounded-lg border border-slate-200 bg-white p-4 text-slate-800">
          {results?.some((result) => result.status === 403 || result.status === 404)
            ? "Student management is unavailable to your account for this school."
            : "Unable to load students. Refresh the page to try again."}
        </p>
      )}
    </main>
  );
}