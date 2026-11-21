import Link from "next/link";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import GuardianClient, { type GuardianPageData } from "./guardian-client";

export default async function GuardiansPage({ params, searchParams }: {
  params: Promise<{ slug: string }>;
  searchParams: Promise<{ student_id?: string; after?: string; linked_after?: string }>;
}) {
  const { slug } = await params;
  const { student_id: studentID, after, linked_after: linkedAfter } = await searchParams;
  const cookie = (await headers()).get("cookie");
  if (!cookie) redirect("/login");
  const root = `/schools/${encodeURIComponent(slug)}`;
  const api = `/api/v1/tenants/${encodeURIComponent(slug)}`;
  const base = process.env.API_INTERNAL_URL || "http://localhost:8080";
  const query = (values: Record<string, string | undefined>) => {
    const search = new URLSearchParams();
    for (const [name, value] of Object.entries(values)) if (value) search.set(name, value);
    return search.size ? `?${search}` : "";
  };
  const get = async <T,>(path: string): Promise<{ status: number; data?: T }> => {
    const response = await fetch(new URL(path, base), {
      headers: { Accept: "application/json", Cookie: cookie },
      cache: "no-store",
      redirect: "error",
    });
    return { status: response.status, data: response.ok ? (await response.json()).data as T : undefined };
  };
  const data = await Promise.all([
    get<{ csrf_token: string }>("/api/v1/auth/me"),
    get<GuardianPageData>(api + "/guardians" + query({ after })),
    studentID ? get<GuardianPageData>(api + "/guardians" + query({ student_id: studentID, after: linkedAfter })) : null,
    studentID ? get<{ id: string; display_name: string }>(api + "/students/" + encodeURIComponent(studentID)) : null,
  ]).catch(() => null);

  if (data?.some((item) => item?.status === 401)) redirect("/login");
  const [identity, directory, relationshipPage, student] = data ?? [];
  const csrf = identity?.data?.csrf_token;
  const guardians = directory?.data?.guardians;
  const links = relationshipPage?.data?.guardians;
  const studentName = student?.data?.display_name;
  const authorized = typeof csrf === "string" && csrf && Array.isArray(guardians) &&
    (!studentID || (Array.isArray(links) && student?.data?.id === studentID && typeof studentName === "string"));
  const path = root + "/guardians";
  return (
    <main className="mx-auto w-full max-w-4xl space-y-6 px-4 py-8 sm:px-6">
      <Link href={root + "/students"} className="text-sm font-medium text-green-800 underline">Back to students</Link>
      <h1 className="text-2xl font-semibold text-slate-900">Guardians</h1>
      {authorized ? (
        <GuardianClient key={path + "/" + (studentID ?? "")} slug={slug} csrf={csrf} guardians={guardians}
          student={studentID && studentName ? { id: studentID, name: studentName } : undefined} linked={links ?? []}
          next={directory?.data?.next_cursor ? path + query({ student_id: studentID, after: directory.data.next_cursor, linked_after: linkedAfter }) : undefined}
          nextLinked={relationshipPage?.data?.next_cursor ? path + query({ student_id: studentID, after, linked_after: relationshipPage.data.next_cursor }) : undefined}
          first={after || linkedAfter ? path + query({ student_id: studentID }) : undefined} />
      ) : (
        <p role="status" className="rounded-lg border border-slate-200 bg-white p-4">
          {data?.some((item) => item?.status === 403 || item?.status === 404)
            ? "Guardian management is unavailable to your account for this school."
            : "Unable to load guardian records. Refresh the page to try again."}
        </p>
      )}
    </main>
  );
}