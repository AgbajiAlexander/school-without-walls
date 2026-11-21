import Link from "next/link";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import StaffClient, { type StaffRecord, type StaffCandidate } from "./staff-client";

interface Envelope<T> { data?: T }

export default async function StaffPage({ params, searchParams }: {
  params: Promise<{ slug: string }>;
  searchParams: Promise<{ staff_after?: string; candidate_after?: string }>;
}) {
  const { slug } = await params;
  const { staff_after: staffAfter, candidate_after: candidateAfter } = await searchParams;
  const cookie = (await headers()).get("cookie");
  if (!cookie) redirect("/login");
  const base = process.env.API_INTERNAL_URL || "http://localhost:8080";
  const prefix = `/api/v1/tenants/${encodeURIComponent(slug)}/staff`;
  const options = { headers: { Accept: "application/json", Cookie: cookie }, cache: "no-store", redirect: "error" } as const;
  const query = (after?: string) => after ? "?" + new URLSearchParams({ after }) : "";
  const get = async (route: string) => fetch(new URL(route, base), options);
  const result = await Promise.all([
    get("/api/v1/auth/me"),
    get(prefix + query(staffAfter)),
    get(prefix + "/candidates" + query(candidateAfter)),
  ]).catch(() => null);
  if (result?.some((response) => response.status === 401)) redirect("/login");
  const payload = result && result.every((response) => response.ok) ? await Promise.all([
    result[0].json() as Promise<Envelope<{csrf_token: string}>>,
    result[1].json() as Promise<Envelope<{staff: StaffRecord[]; next_cursor?: string}>>,
    result[2].json() as Promise<Envelope<{candidates: StaffCandidate[]; next_cursor?: string}>>,
  ]).catch(() => null) : null;
  const [identity, profiles, candidates] = payload ?? [];
  const csrfToken = identity?.data?.csrf_token;
  const staff = profiles?.data?.staff;
  const membershipChoices = candidates?.data?.candidates;
  const path = `/schools/${encodeURIComponent(slug)}/staff`;
  const pageLink = (staffCursor?: string, candidateCursor?: string) => {
    const search = new URLSearchParams();
    if (staffCursor) search.set("staff_after",staffCursor);
    if (candidateCursor) search.set("candidate_after",candidateCursor);
    return search.size ? `${path}?${search}` : path;
  };
  return (
    <main className="mx-auto w-full max-w-4xl space-y-6 px-4 py-8 sm:px-6">
      <Link href="/dashboard" className="text-sm font-medium text-green-800 underline">Back to My Schools</Link>
      <h1 className="text-2xl font-semibold text-slate-900">Staff profiles</h1>
      {typeof csrfToken === "string" && csrfToken && Array.isArray(staff) && Array.isArray(membershipChoices) ? (
        <StaffClient key={path} slug={slug} csrfToken={csrfToken} staff={staff} candidates={membershipChoices}
          staffNext={profiles?.data?.next_cursor ? pageLink(profiles.data.next_cursor,candidateAfter) : undefined}
          candidateNext={candidates?.data?.next_cursor ? pageLink(staffAfter,candidates.data.next_cursor) : undefined}
          resetPages={staffAfter || candidateAfter ? path : undefined} />
      ) : (
        <p role="status" className="rounded-lg border border-slate-200 bg-white p-4 text-slate-800">
          {result?.some((response) => response.status === 403 || response.status === 404)
            ? "Staff profiles are unavailable to your account for this school."
            : "Unable to load staff profiles. Refresh the page to try again."}
        </p>
      )}
    </main>
  );
}