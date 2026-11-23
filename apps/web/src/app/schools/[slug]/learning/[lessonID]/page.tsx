import Link from "next/link";
import AssignmentSubmission, { type Receipt } from "./assignment-submission";
import type { Feedback } from "../../teaching/submission-feedback";
import type { Assignment } from "../../teaching/assignment-controls";
import LessonDownload from "./lesson-download";
import LessonProgress, { type ProgressState } from "./lesson-progress";
import { readLessonSession, readStudentLessons, type LessonSummary } from "../lesson-api";

export default async function LessonPage({ params, searchParams }: { params: Promise<{ slug: string; lessonID: string }>; searchParams: Promise<{ assignment_after?: string }> }) {
  const { slug, lessonID } = await params;
  const { assignment_after } = await searchParams;
  const assignmentQuery = typeof assignment_after === "string" ? "?" + new URLSearchParams({ after: assignment_after }) : "";
  const lessonPath = "/" + encodeURIComponent(lessonID);
  const [lesson, session, homework] = await Promise.all([
    readStudentLessons<LessonSummary & { content: string; progress_state: ProgressState; has_attachment?: boolean; enrichment?: { code: string; text: string; withdrawn: boolean }[] }>(slug, `/${encodeURIComponent(lessonID)}`),
    readLessonSession(),
    readStudentLessons<{ items: (Assignment & { submission?: Receipt; feedback?: Feedback; file_upload_enabled?: boolean })[]; next?: string }>(slug, lessonPath + "/assignments" + assignmentQuery),
  ]);
  return (
    <main className="mx-auto w-full max-w-3xl space-y-5 px-4 py-8 sm:px-6">
      <Link href={`/schools/${encodeURIComponent(slug)}/learning`} className="text-green-800 underline">Back to lessons</Link>
      {typeof lesson?.content === "string" ? (
        <article className="space-y-4 break-words rounded-xl border border-slate-200 bg-white p-4 sm:p-6">
          <p className="text-sm text-slate-700">{lesson.subject} · {lesson.objective_code}</p>
          <h1 className="text-2xl font-semibold">{lesson.title}</h1>
          <div className="whitespace-pre-wrap leading-relaxed">{lesson.content}</div>
          {lesson.has_attachment && (
            <LessonDownload endpoint={`/api/v1/tenants/${encodeURIComponent(slug)}/learning/lessons/${encodeURIComponent(lessonID)}/media`} />
          )}
          {session?.csrf_token && lesson.progress_state && (
            <LessonProgress endpoint={`/api/v1/tenants/${encodeURIComponent(slug)}/learning/lessons/${encodeURIComponent(lessonID)}/progress`}
              initialState={lesson.progress_state} csrf={session.csrf_token} />
          )}
          {lesson.enrichment?.length ? <section className="space-y-2 border-t pt-3">
            <h2 className="text-lg font-semibold">Optional enrichment</h2>
            <ul className="space-y-2">{lesson.enrichment.map((item, index) => (
              <li key={`${item.code}-${index}`}><strong>{item.code}</strong> · {item.text}
                {item.withdrawn && <span> (edition withdrawn)</span>}</li>
            ))}</ul>
          </section> : null}
          <section className="space-y-2 border-t pt-3">
            <h2 className="text-lg font-semibold">Assignments</h2>
            {!homework ? <p>Assignments are temporarily unavailable.</p> : !homework.items.length ? <p>No released assignments on this page.</p>
              : <ul className="space-y-4">{homework.items.map((item) => (
                <li key={item.id} className="space-y-1 rounded border p-3">
                  <h3 className="font-semibold">{item.title}</h3>
                  <p className="whitespace-pre-wrap">{item.instructions}</p>
                  <p>{item.status === "CLOSED" ? "Closed" : "Open"} · Maximum score {item.max_score}</p>
                  <p>Due <time dateTime={item.due_at}>{new Date(item.due_at).toISOString().slice(0, 16).replace("T", " ")} UTC</time></p>
                  {session?.csrf_token && (item.submission_type === "TEXT" || item.file_upload_enabled || item.submission) && (
                    <AssignmentSubmission endpoint={"/api/v1/tenants/" + encodeURIComponent(slug) + "/learning/lessons" + lessonPath + "/assignments/" + encodeURIComponent(item.id) + (item.submission_type === "FILE" ? "/file" : "/submissions")}
                      csrf={session.csrf_token} receipt={item.submission} fileMode={item.submission_type === "FILE"}
                      available={item.status === "OPEN" && Date.parse(item.due_at) > Date.now() && (item.submission_type === "TEXT" || item.file_upload_enabled === true)}
                      resubmit={item.allow_resubmission} feedback={item.feedback} maxScore={item.max_score} />
                  )}
                </li>
              ))}</ul>}
            {homework?.next && <Link className="text-green-800 underline" href={"?" + new URLSearchParams({ assignment_after: homework.next })}>Next assignments</Link>}
          </section>
        </article>
      ) : (
        <h1 className="text-2xl font-semibold">Lesson unavailable</h1>
      )}
    </main>
  );
}
