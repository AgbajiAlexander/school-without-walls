import Link from "next/link";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import EditionForm from "./edition-form";

const stages: Record<string, string> = {
  EARLY_YEARS: "Early Years", PRIMARY: "Primary", JUNIOR_SECONDARY: "Junior Secondary", SENIOR_SECONDARY: "Senior Secondary",
};
const roles: Record<string, string> = { PRIMARY: "Primary curriculum", ENRICHMENT: "Optional enrichment" };
type Selection = {
  id: string; stage: string; curriculum_role: string; revision: number; framework_version_id: string;
  framework_title: string; framework_edition: string; framework_status: string;
};
type Edition = { id: string; title: string; edition: string; publisher: string };

export default async function SessionCurriculumPage({ params, searchParams }: {
  params: Promise<{ slug: string; sessionID: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const cookie = (await headers()).get("cookie");
  if (!cookie) redirect("/login");
  const { slug, sessionID } = await params;
  const q = await searchParams;
  const stage = typeof q.stage === "string" && Object.hasOwn(stages, q.stage) ? q.stage : "PRIMARY";
  const role = q.curriculum_role === "ENRICHMENT" ? "ENRICHMENT" : "PRIMARY";
  const after = typeof q.after === "string" ? q.after : "";
  const academic = `/schools/${encodeURIComponent(slug)}/academics`;
  const root = `${academic}/${encodeURIComponent(sessionID)}/curriculum`;
  const endpoint = `/api/v1/tenants/${encodeURIComponent(slug)}/academic-sessions/${encodeURIComponent(sessionID)}/curriculum`;
  const api = process.env.API_INTERNAL_URL || "http://localhost:8080";
  const get = async (path: string) => fetch(new URL(path, api), {
    headers: { Accept: "application/json", Cookie: cookie }, cache: "no-store", redirect: "error",
  });
  const results = await Promise.all([get(endpoint), get("/api/v1/auth/me")]).catch(() => null);
  if (results?.some((row) => row.status === 401)) redirect("/login");
  const payloads = results?.every((row) => row.ok) ? await Promise.all([
    results[0].json() as Promise<{ data?: { items: Selection[]; session_status: string } }>,
    results[1].json() as Promise<{ data?: { csrf_token: string } }>,
  ]).catch(() => null) : null;
  const selections = payloads?.[0].data;
  const csrf = payloads?.[1].data?.csrf_token;
  const scopeAvailable = Array.isArray(selections?.items) && typeof csrf === "string" && !!csrf;
  const query = { stage, curriculum_role: role };
  const filtered = new URLSearchParams(after ? { ...query, after } : query);
  const options = scopeAvailable && selections?.session_status !== "ARCHIVED"
    ? await get(`/api/v1/curriculum/frameworks?${filtered}`).catch(() => null) : null;
  if (options?.status === 401) redirect("/login");
  const editions = options?.ok ? (await options.json().catch(() => null))?.data as { items: Edition[]; next?: string } | undefined : undefined;
  const current = scopeAvailable ? selections.items.find((item) => item.stage === stage && item.curriculum_role === role) : undefined;

  return (
    <main className="mx-auto w-full max-w-4xl space-y-5 px-4 py-8 sm:px-6">
      <Link href={academic} className="text-green-800 underline">Back to Academic Management</Link>
      <h1 className="text-2xl font-semibold">Session curriculum editions</h1>
      {!scopeAvailable ? <p role="status">{results?.some((row) => [403, 404].includes(row.status))
        ? "Curriculum selection is unavailable for this school session."
        : "Unable to load curriculum selections. Refresh the page to try again."}</p> : (
        <>
          <section className="space-y-2 rounded-xl border border-slate-200 bg-white p-4">
            <h2 className="text-lg font-semibold">Current selections</h2>
            {selections!.items.length ? <ul className="space-y-2">
              {selections!.items.map((item) => <li key={item.id} className="break-words">
                {stages[item.stage]} · {roles[item.curriculum_role]} · {item.framework_title}, {item.framework_edition}
                {item.framework_status === "WITHDRAWN" && <strong> · Withdrawn</strong>}
              </li>)}
            </ul> : <p>No editions selected for this session.</p>}
          </section>
          {selections?.session_status === "ARCHIVED" ? <p role="status">This archived session is read-only.</p> : (
            <section className="space-y-4 rounded-xl border border-slate-200 bg-white p-4">
              <h2 className="text-lg font-semibold">Choose an edition</h2>
              <form action={root} className="flex flex-wrap items-end gap-3">
                <label>Education stage<select name="stage" defaultValue={stage} className="block min-h-11 rounded border px-2">
                  {Object.entries(stages).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                </select></label>
                <label>Curriculum role<select name="curriculum_role" defaultValue={role} className="block min-h-11 rounded border px-2">
                  {Object.entries(roles).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                </select></label>
                <button type="submit" className="min-h-11 px-2 text-green-800 underline">Show eligible editions</button>
              </form>
              {Array.isArray(editions?.items) ? (
                <EditionForm key={`${stage}:${role}:${after}`} endpoint={endpoint} csrf={csrf!} stage={stage} role={role}
                  revision={current?.revision ?? 0} currentID={current?.framework_version_id} editions={editions.items} />
              ) : <p role="status">Unable to load eligible editions.</p>}
              {(after || editions?.next) && <nav aria-label="Edition pages" className="flex gap-4">
                {after && <Link className="text-green-800 underline" href={`${root}?${new URLSearchParams(query)}`}>First page</Link>}
                {editions?.next && <Link className="text-green-800 underline" href={`${root}?${new URLSearchParams({ ...query, after: editions.next })}`}>Next page</Link>}
              </nav>}
            </section>
          )}
        </>
      )}
    </main>
  );
}
