import Link from "next/link";
import { readLearningApi } from "../../learning/lesson-api";
import AttendanceRefresh from "./attendance-refresh";

type Child = { id: string; display_name: string };
type Mark = { date: string; class_name: string; mode: string; status: string };
const statusLabels: Record<string, string> = { PRESENT: "Present", ABSENT: "Absent", LATE: "Late", EXCUSED: "Excused" };

export default async function FamilyAttendancePage({ params, searchParams }: {
  params: Promise<{ slug: string }>;
  searchParams: Promise<{ child?: string; after?: string }>;
}) {
  const { slug } = await params;
  const query = await searchParams;
  const child = typeof query.child === "string" ? query.child : "";
  const after = typeof query.after === "string" ? query.after : "";
  const path = "/api/v1/tenants/" + encodeURIComponent(slug) + "/family/attendance";
  const [children, attendance] = await Promise.all([
    readLearningApi<{ items: Child[]; next?: string }>(path + (after ? "?" + new URLSearchParams({ after }) : "")),
    child ? readLearningApi<{ child: Child; marks: Mark[] }>(path + "/" + encodeURIComponent(child)) : null,
  ]);

  return (
    <main className="mx-auto max-w-3xl space-y-5 px-4 py-8 sm:px-6">
      <AttendanceRefresh />
      <Link href="/dashboard" className="text-green-800 underline">Back to My Schools</Link>
      <h1 className="text-2xl font-semibold">Family attendance</h1>
      <section className="space-y-2 rounded-xl border p-4">
        <h2 className="text-lg font-semibold">Linked children</h2>
        {!children ? <p role="status">Linked children are temporarily unavailable.</p>
          : children.items.length ? (
            <ul>{children.items.map((item) => (
              <li key={item.id}><Link prefetch={false}
                href={"?" + new URLSearchParams({ child: item.id, ...(after ? { after } : {}) })}
                className="inline-block min-h-11 break-words py-2 text-green-800 underline">{item.display_name}</Link></li>
            ))}</ul>
          ) : <p role="status">No linked children on this page.</p>}
        {children?.next && <Link prefetch={false} className="text-green-800 underline"
          href={"?" + new URLSearchParams({ after: children.next })}>Next children</Link>}
      </section>
      {child && (!attendance ? <p role="status">Attendance is unavailable for this child.</p> : (
        <section className="space-y-2 rounded-xl border p-4">
          <h2 className="break-words text-lg font-semibold">{attendance.child.display_name}</h2>
          {!attendance.marks.length ? <p>No attendance has been recorded.</p> : (
            <>
              <p className="text-sm text-slate-700">Most recent attendance marks</p>
              <ul className="divide-y">{attendance.marks.map((mark, index) => (
                <li key={mark.date + "-" + index} className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 py-2">
                  <span className="break-words">{mark.class_name} <time dateTime={mark.date}>{mark.date}</time></span>
                  <span>{statusLabels[mark.status]} · {mark.mode === "REMOTE" ? "Remote" : "On site"}</span>
                </li>
              ))}</ul>
            </>
          )}
        </section>
      ))}
    </main>
  );
}
