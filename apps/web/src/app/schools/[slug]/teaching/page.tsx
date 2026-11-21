import Link from "next/link";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import LessonEditor, { type EnrichmentOption } from "./lesson-editor";

type Page<T> = { items: T[]; next?: string };
type Offering = { id: string; subject: string; class_name: string; arm_name?: string; term_name: string };
type Objective = { id: string; code: string; objective_text: string; enrichment?: EnrichmentOption[] };
type Lesson = { id: string; title: string; status: string; content?: string; objective_code: string; review_required: boolean; review_status: string; enrichment?: EnrichmentOption[] };

export default async function TeachingPage({ params, searchParams }: {
  params: Promise<{ slug: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const { slug } = await params;
  const q = await searchParams;
  const cookie = (await headers()).get("cookie");
  if (!cookie) redirect("/login");
  const school = encodeURIComponent(slug);
  const root = `/schools/${school}/teaching`;
  const endpoint = `/api/v1/tenants/${school}/teaching`;
  const offering = typeof q.offering === "string" ? q.offering : "";
  const lesson = typeof q.lesson === "string" ? q.lesson : "";
  const href = (values: Record<string, string>) => `${root}?${new URLSearchParams(values)}`;
  function cursor(name: string) {
    const value = q[name];
    return typeof value === "string" ? `?${new URLSearchParams({ after: value })}` : "";
  }
  async function get<T>(path: string): Promise<T | null> {
    let result: Response;
    try {
      result = await fetch(new URL(path, process.env.API_INTERNAL_URL || "http://localhost:8080"), {
        headers: { Accept: "application/json", Cookie: cookie ?? "" }, cache: "no-store", redirect: "error",
      });
    } catch {
      return null;
    }
    if (result.status === 401) redirect("/login");
    if (!result.ok) return null;
    return (await result.json().catch(() => null))?.data ?? null;
  }
  const [assignments, lessons, session, objectives, selected] = await Promise.all([
    get<Page<Offering>>(`${endpoint}/offerings${cursor("offering_after")}`),
    get<Page<Lesson>>(`${endpoint}/lessons${cursor("lesson_after")}`),
    get<{ csrf_token: string }>("/api/v1/auth/me"),
    offering && !lesson ? get<Page<Objective>>(`${endpoint}/offerings/${encodeURIComponent(offering)}/objectives${cursor("objective_after")}`) : null,
    lesson ? get<Lesson>(`${endpoint}/lessons/${encodeURIComponent(lesson)}`) : null,
  ]);
  const csrf = session?.csrf_token;
  return (
    <main className="mx-auto w-full max-w-4xl space-y-5 px-4 py-8 sm:px-6">
      <Link href="/dashboard" className="text-green-800 underline">Back to My Schools</Link>
      <h1 className="text-2xl font-semibold">Teaching lessons</h1>
      {(!assignments || !lessons || !csrf) ? <p role="status">Unable to load teaching lessons. Refresh to try again.</p> : (
        <>
          <section className="space-y-2 rounded-xl border p-4">
            <h2 className="text-lg font-semibold">Assigned offerings</h2>
            {assignments.items.length ? <ul>{assignments.items.map((item) => <li key={item.id}>
              <Link className="inline-block min-h-11 break-words py-2 text-green-800 underline" href={href({ offering: item.id })}>
                {item.subject} · {item.class_name}{item.arm_name ? ` · ${item.arm_name}` : ""} · {item.term_name}
              </Link>
            </li>)}</ul> : <p>No active teaching offerings on this page.</p>}
            {assignments.next && <Link href={href({ offering_after: assignments.next })} className="text-green-800 underline">Next offerings</Link>}
          </section>
          <section className="space-y-2 rounded-xl border p-4">
            <h2 className="text-lg font-semibold">Your lessons</h2>
            {lessons.items.length ? <ul>{lessons.items.map((item) => <li key={item.id}>
              <Link className="inline-block min-h-11 break-words py-2 text-green-800 underline" href={href({ lesson: item.id })}>{item.title}</Link>
              <span className="ml-2 text-sm">{item.status}</span>
            </li>)}</ul> : <p>No lessons on this page.</p>}
            {lessons.next && <Link href={href({ lesson_after: lessons.next })} className="text-green-800 underline">Next teaching lessons</Link>}
          </section>
          {lesson ? (selected ? <LessonEditor key={selected.id} endpoint={`${endpoint}/lessons`} root={root} csrf={csrf} lesson={selected} /> :
            <p role="status">Teaching lesson unavailable.</p>) : offering ? (
            <>
              {objectives?.items.length ? <LessonEditor key={offering} endpoint={`${endpoint}/lessons`} root={root} csrf={csrf}
                offering={offering} objectives={objectives.items} /> : <p role="status">No eligible curriculum objectives on this page.</p>}
              {objectives?.next && <Link href={href({ offering, objective_after: objectives.next })} className="text-green-800 underline">Next objectives</Link>}
            </>
          ) : null}
        </>
      )}
    </main>
  );
}
