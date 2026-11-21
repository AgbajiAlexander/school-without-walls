import Link from "next/link";
import { readStudentLessons, type LessonSummary } from "./lesson-api";

export default async function LearningPage({ params, searchParams }: {
  params: Promise<{ slug: string }>;
  searchParams: Promise<{ after?: string }>;
}) {
  const { slug } = await params;
  const { after } = await searchParams;
  const root = `/schools/${encodeURIComponent(slug)}/learning`;
  const query = typeof after === "string" ? `?${new URLSearchParams({ after })}` : "";
  const page = await readStudentLessons<{ items: LessonSummary[]; next?: string }>(slug, query);

  return (
    <main className="mx-auto w-full max-w-3xl space-y-5 px-4 py-8 sm:px-6">
      <Link href="/dashboard" className="text-green-800 underline">Back to My Schools</Link>
      <h1 className="text-2xl font-semibold">Lessons</h1>
      {!Array.isArray(page?.items) ? <p role="status">Unable to load lessons. Refresh the page to try again.</p>
        : page.items.length === 0 ? <p role="status">No published lessons are available for your current enrollment.</p>
          : (
            <ul className="space-y-3">
              {page.items.map((lesson) => (
                <li key={lesson.id} className="break-words rounded-xl border border-slate-200 bg-white p-4">
                  <p className="text-sm text-slate-700">{lesson.subject} · {lesson.objective_code}</p>
                  <Link className="inline-block min-h-11 py-2 font-semibold text-green-800 underline"
                    href={`${root}/${encodeURIComponent(lesson.id)}`}>{lesson.title}</Link>
                </li>
              ))}
            </ul>
          )}
      {page && typeof page.next === "string" && (
        <Link href={`${root}?${new URLSearchParams({ after: page.next })}`} className="inline-block py-2 text-green-800 underline">Next lessons</Link>
      )}
    </main>
  );
}
