import Link from "next/link";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import EnrollmentClient, { type Choice, type Placement } from "./enrollment-client";

export default async function EnrollmentPage({ params, searchParams }: {
  params: Promise<{ slug: string; studentID: string }>;
  searchParams: Promise<{ before?: string }>;
}) {
  const { slug, studentID } = await params;
  const { before } = await searchParams;
  const cookie = (await headers()).get("cookie");
  if (!cookie) redirect("/login");

  const school = `/schools/${encodeURIComponent(slug)}`;
  const path = `${school}/students/${encodeURIComponent(studentID)}/enrollments`;
  const api = `/api/v1/tenants/${encodeURIComponent(slug)}`;
  const studentPath = `${api}/students/${encodeURIComponent(studentID)}`;
  const base = process.env.API_INTERNAL_URL || "http://localhost:8080";
  const get = async <T,>(url: string) => {
    const result = await fetch(new URL(url, base), {
      headers: { Accept: "application/json", Cookie: cookie },
      cache: "no-store",
      redirect: "error",
    });
    return { status: result.status, data: result.ok ? (await result.json()).data as T : undefined };
  };
  const loaded = await Promise.all([
    get<{ csrf_token: string }>("/api/v1/auth/me"),
    get<{ display_name: string }>(studentPath),
    get<{ enrollments: Placement[]; next_cursor?: string }>(studentPath + "/enrollments" +
      (before ? "?" + new URLSearchParams({ before }) : "")),
  ]).catch(() => null);
  if (loaded?.some(({ status }) => status === 401)) redirect("/login");
  const [identity, student, history] = loaded ?? [];
  const csrf = identity?.data?.csrf_token;
  const placements = history?.data?.enrollments;
  const name = student?.data?.display_name;
  const authorized = typeof csrf === "string" && csrf && typeof name === "string" && Array.isArray(placements);
  let session: (Choice & { status: string }) | undefined;
  let terms: Choice[] = [];
  let classes: Choice[] = [];
  let choicesUnavailable = false;
  if (authorized) {
    try {
      const sessions = await get<{ academic_sessions: (Choice & { status: string })[] }>(api + "/academic-sessions");
      if (!Array.isArray(sessions.data?.academic_sessions)) throw new Error("Sessions unavailable");
      session = sessions.data.academic_sessions.find((item) => item.status === "ACTIVE");
      if (session) {
        const [termResult, classResult] = await Promise.all([
          get<{ terms: Choice[] }>(`${api}/academic-sessions/${encodeURIComponent(session.id)}/terms`),
          get<{ classes: (Choice & { status: string })[] }>(api + "/classes?" + new URLSearchParams({ session_id: session.id })),
        ]);
        if (!Array.isArray(termResult.data?.terms) || !Array.isArray(classResult.data?.classes)) throw new Error("Placements unavailable");
        terms = termResult.data.terms;
        classes = classResult.data.classes.filter((item) => item.status === "ACTIVE");
      }
    } catch {
      choicesUnavailable = true;
    }
  }

  return (
    <main className="mx-auto w-full max-w-4xl space-y-6 px-4 py-8 sm:px-6">
      <Link href={school + "/students"} className="text-sm font-medium text-green-800 underline">Back to students</Link>
      <h1 className="text-2xl font-semibold">Enrollment{authorized ? ` for ${name}` : ""}</h1>
      {authorized ? (
        <EnrollmentClient key={path} slug={slug} studentID={studentID} csrf={csrf} placements={placements}
          terms={terms} classes={classes} activeSession={session?.id} choicesUnavailable={choicesUnavailable}
          next={history?.data?.next_cursor ? path + "?" + new URLSearchParams({ before: history.data.next_cursor }) : undefined}
          first={before ? path : undefined} />
      ) : (
        <p role="status" className="rounded-lg border border-slate-200 bg-white p-4">
          {loaded?.some(({ status }) => status === 403 || status === 404)
            ? "Enrollment is unavailable to your account for this school."
            : "Unable to load enrollment. Refresh the page to try again."}
        </p>
      )}
    </main>
  );
}