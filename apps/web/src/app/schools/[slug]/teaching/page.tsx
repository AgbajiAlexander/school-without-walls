import Link from "next/link";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { parseMediaUploadOrigin } from "@/lib/security-headers.mjs";
import LessonEditor, { type EnrichmentOption } from "./lesson-editor";

import AssignmentControls, { type Assignment } from "./assignment-controls";
import AttendanceRegister, { type RosterItem } from "./attendance-register";
import SubmissionFeedback, { type ReviewSubmission } from "./submission-feedback";

type Page<T> = { items: T[]; next?: string };
type AttendanceTotals = { present: number; absent: number; late: number; excused: number };
type AttendanceSummary = { eligible: number; unrecorded: number; on_site: AttendanceTotals; remote: AttendanceTotals };
type Offering = { id: string; subject: string; class_name: string; arm_name?: string; term_name: string };
type Objective = { id: string; code: string; objective_text: string; enrichment?: EnrichmentOption[] };
type Lesson = { id: string; title: string; status: string; content?: string; objective_code: string; review_required: boolean; review_status: string; media_upload_enabled?: boolean; file_submission_enabled?: boolean; enrichment?: EnrichmentOption[] };

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
  const attendance = typeof q.attendance === "string" ? q.attendance : "";
  const date = typeof q.attendance_date === "string" ? q.attendance_date : "";
  const rosterQuery = new URLSearchParams({ date });
  if (typeof q.roster_after === "string") rosterQuery.set("after", q.roster_after);
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
  const [assignments, lessons, session, objectives, selected, homework, roster, participation] = await Promise.all([
    get<Page<Offering>>(`${endpoint}/offerings${cursor("offering_after")}`),
    get<Page<Lesson>>(`${endpoint}/lessons${cursor("lesson_after")}`),
    get<{ csrf_token: string }>("/api/v1/auth/me"),
    offering && !lesson ? get<Page<Objective>>(`${endpoint}/offerings/${encodeURIComponent(offering)}/objectives${cursor("objective_after")}`) : null,
    lesson ? get<Lesson>(`${endpoint}/lessons/${encodeURIComponent(lesson)}`) : null,
    lesson ? get<Page<Assignment>>(`${endpoint}/lessons/${encodeURIComponent(lesson)}/assignments${cursor("assignment_after")}`) : null,
    attendance && date ? get<Page<RosterItem>>(endpoint + "/offerings/" + encodeURIComponent(attendance) + "/attendance?" + rosterQuery) : null,
    attendance && date ? get<AttendanceSummary>(endpoint + "/offerings/" + encodeURIComponent(attendance) + "/attendance/summary?" + new URLSearchParams({ date })) : null,
  ]);
  const csrf = session?.csrf_token;
  const review = homework?.items.find((item) => item.id === q.review_assignment && item.status !== "DRAFT");
  const reviewEndpoint = review && selected ? endpoint + "/lessons/" + encodeURIComponent(selected.id) +
    "/assignments/" + encodeURIComponent(review.id) + "/submissions" : null;
  const submissions = reviewEndpoint ? await get<Page<ReviewSubmission>>(reviewEndpoint + cursor("submission_after")) : null;
  const assignmentQuery: Record<string, string> = typeof q.assignment_after === "string" ? { assignment_after: q.assignment_after } : {};
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
              <Link className="ml-3 inline-block min-h-11 py-2 text-green-800 underline"
                href={href({ attendance: item.id })}>Attendance roster</Link>
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
          {attendance && (
            <section className="space-y-3 rounded-xl border p-4">
              <h2 className="text-lg font-semibold">Attendance roster</h2>
              <form method="get" action={root} className="flex flex-wrap items-end gap-3">
                <input type="hidden" name="attendance" value={attendance} />
                <label>Attendance date<input name="attendance_date" type="date" required defaultValue={date}
                  className="block min-h-11 rounded border px-2" /></label>
                <button type="submit" className="min-h-11 rounded bg-green-800 px-3 text-white">Load roster</button>
              </form>
              {participation && (
                <section className="space-y-2 rounded-lg border bg-slate-50 p-3">
                  <h3 className="font-semibold">Daily participation</h3>
                  <p>Eligible learners: {participation.eligible}. No mark recorded: {participation.unrecorded}.</p>
                  <div className="overflow-x-auto">
                    <table aria-label="Participation by attendance mode" className="w-full text-sm">
                      <thead><tr>
                        <th scope="col" className="p-2 text-left">Mode</th>
                        <th scope="col" className="p-2">Present</th><th scope="col" className="p-2">Late</th>
                        <th scope="col" className="p-2">Absent</th><th scope="col" className="p-2">Excused</th>
                      </tr></thead>
                      <tbody>
                        {([["On site", participation.on_site], ["Remote", participation.remote]] as const).map(([label, counts]) => (
                          <tr key={label} className="border-t">
                            <th scope="row" className="p-2 text-left">{label}</th>
                            <td className="p-2 text-center">{counts.present}</td><td className="p-2 text-center">{counts.late}</td>
                            <td className="p-2 text-center">{counts.absent}</td><td className="p-2 text-center">{counts.excused}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </section>
              )}
              {date && (!roster ? <p role="status">The attendance roster is unavailable.</p>
                : !roster.items.length ? <p role="status">No eligible learners for this offering and date.</p>
                : <AttendanceRegister key={attendance + ":" + date + ":" + (q.roster_after ?? "")} endpoint={endpoint + "/offerings/" + encodeURIComponent(attendance) + "/attendance"}
                    date={date} csrf={csrf} items={roster.items} />)}
              {roster?.next && <Link className="text-green-800 underline"
                href={href({ attendance, attendance_date: date, roster_after: roster.next })}>Next learners</Link>}
            </section>
          )}
          {lesson ? (selected ? <LessonEditor key={selected.id} endpoint={`${endpoint}/lessons`} root={root} csrf={csrf} lesson={selected} uploadOrigin={selected.media_upload_enabled ? parseMediaUploadOrigin(process.env.LESSON_MEDIA_BROWSER_ORIGIN) : ""} /> :
            <p role="status">Teaching lesson unavailable.</p>) : offering ? (
            <>
              {objectives?.items.length ? <LessonEditor key={offering} endpoint={`${endpoint}/lessons`} root={root} csrf={csrf}
                offering={offering} objectives={objectives.items} /> : <p role="status">No eligible curriculum objectives on this page.</p>}
              {objectives?.next && <Link href={href({ offering, objective_after: objectives.next })} className="text-green-800 underline">Next objectives</Link>}
            </>
          ) : null}
          {review && reviewEndpoint && (
            <section className="space-y-3 rounded-xl border p-4">
              <h2 className="text-lg font-semibold">Submissions for {review.title}</h2>
              {!submissions ? <p>Submissions are temporarily unavailable.</p>
                : !submissions.items.length ? <p>No submissions on this page.</p> : submissions.items.map((submission) => (
                  <SubmissionFeedback key={submission.id} submission={submission} csrf={csrf} maxScore={review.max_score}
                    endpoint={reviewEndpoint + "/" + encodeURIComponent(submission.id) + "/feedback"} />
                ))}
              {submissions?.next && <Link className="text-green-800 underline"
                href={href({ lesson, ...assignmentQuery, review_assignment: review.id, submission_after: submissions.next })}>Next submissions</Link>}
            </section>
          )}
          {selected && homework && (
            <>
              <AssignmentControls key={selected.id} endpoint={endpoint + "/lessons/" + encodeURIComponent(selected.id) + "/assignments"} csrf={csrf}
                items={homework.items} fileSubmissionEnabled={selected.file_submission_enabled === true} reviewRoot={href({ lesson: selected.id, ...assignmentQuery })} lessonPublished={selected.status === "PUBLISHED"} archived={selected.status === "ARCHIVED"} />
              {homework.next && <Link href={href({ lesson: selected.id, assignment_after: homework.next })} className="text-green-800 underline">Next assignments</Link>}
            </>
          )}
        </>
      )}
    </main>
  );
}
