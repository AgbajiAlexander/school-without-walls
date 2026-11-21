import Link from "next/link";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import ReviewQueue, { type DraftAlignment } from "./review-queue";

export default async function AlignmentReviewPage({ searchParams }: {
  searchParams: Promise<{ after?: string | string[] }>;
}) {
  const cookie = (await headers()).get("cookie");
  if (!cookie) redirect("/login");
  const query = await searchParams;
  const after = typeof query.after === "string" ? query.after : "";
  const root = "/platform/curriculum";
  const endpoint = "/api/v1/curriculum/alignments/drafts";
  const get = (path: string) => fetch(new URL(path, process.env.API_INTERNAL_URL || "http://localhost:8080"), {
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
    results[1].json() as Promise<{ data?: { items?: DraftAlignment[]; next?: string } }>,
  ]).catch(() => null) : null;
  const csrf = payload?.[0].data?.csrf_token;
  const page = payload?.[1].data;
  return (
    <main className="mx-auto w-full max-w-5xl space-y-5 px-4 py-8 sm:px-6">
      <Link href="/platform/tenants" className="text-green-800 underline">Back to platform schools</Link>
      <h1 className="text-2xl font-semibold">Curriculum alignment review</h1>
      {csrf && Array.isArray(page?.items) ? (
        <ReviewQueue key={after} csrf={csrf} drafts={page.items}
          first={after ? root : undefined}
          next={page.next ? `${root}?${new URLSearchParams({ after: page.next })}` : undefined} />
      ) : (
        <p role="status">{results?.some((result) => result.status === 403)
          ? "Global curriculum administrator access required."
          : "Unable to load the alignment review queue. Refresh the page to try again."}</p>
      )}
    </main>
  );
}
