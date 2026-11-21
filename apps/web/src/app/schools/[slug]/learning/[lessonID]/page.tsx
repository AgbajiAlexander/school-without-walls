import Link from "next/link";
import { readStudentLessons, type LessonSummary } from "../lesson-api";

export default async function LessonPage({ params }: { params: Promise<{ slug: string; lessonID: string }> }) {
  const { slug, lessonID } = await params;
  const lesson = await readStudentLessons<LessonSummary & { content: string; enrichment?: { code: string; text: string; withdrawn: boolean }[] }>(slug, `/${encodeURIComponent(lessonID)}`);
  return (
    <main className="mx-auto w-full max-w-3xl space-y-5 px-4 py-8 sm:px-6">
      <Link href={`/schools/${encodeURIComponent(slug)}/learning`} className="text-green-800 underline">Back to lessons</Link>
      {typeof lesson?.content === "string" ? (
        <article className="space-y-4 break-words rounded-xl border border-slate-200 bg-white p-4 sm:p-6">
          <p className="text-sm text-slate-700">{lesson.subject} · {lesson.objective_code}</p>
          <h1 className="text-2xl font-semibold">{lesson.title}</h1>
          <div className="whitespace-pre-wrap leading-relaxed">{lesson.content}</div>
          {lesson.enrichment?.length ? <section className="space-y-2 border-t pt-3">
            <h2 className="text-lg font-semibold">Optional enrichment</h2>
            <ul className="space-y-2">{lesson.enrichment.map((item, index) => (
              <li key={`${item.code}-${index}`}><strong>{item.code}</strong> · {item.text}
                {item.withdrawn && <span> (edition withdrawn)</span>}</li>
            ))}</ul>
          </section> : null}
        </article>
      ) : (
        <h1 className="text-2xl font-semibold">Lesson unavailable</h1>
      )}
    </main>
  );
}
