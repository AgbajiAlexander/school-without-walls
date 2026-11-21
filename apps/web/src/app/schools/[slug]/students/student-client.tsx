"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";

const stages = {
  EARLY_YEARS: "Early Years",
  PRIMARY: "Primary",
  JUNIOR_SECONDARY: "Junior Secondary",
  SENIOR_SECONDARY: "Senior Secondary",
};

interface StudentFields {
  admission_no: string;
  display_name: string;
  stage: keyof typeof stages;
}

export interface StudentRecord extends StudentFields {
  id: string;
  status: "ACTIVE" | "INACTIVE";
}

const empty: StudentFields = { admission_no: "", display_name: "", stage: "PRIMARY" };

export default function StudentClient({ slug, csrfToken, students, nextPage, firstPage }: {
  slug: string;
  csrfToken: string;
  students: StudentRecord[];
  nextPage?: string;
  firstPage?: string;
}) {
  const router = useRouter();
  const inFlight = useRef(false);
  const denied = useRef(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [fields, setFields] = useState<StudentFields>(empty);
  const [busy, setBusy] = useState(false);
  const [unavailable, setUnavailable] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  function reset() {
    setEditing(null);
    setFields(empty);
    setError("");
    setMessage("");
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (inFlight.current || denied.current) return;
    const body = { ...fields, admission_no: fields.admission_no.trim(), display_name: fields.display_name.trim() };
    if (!body.admission_no || !body.display_name || !Object.hasOwn(stages, body.stage)) {
      setError("Enter an admission number, display name, and education stage.");
      return;
    }
    inFlight.current = true;
    setBusy(true);
    setError("");
    setMessage("");

    try {
      const root = `/api/v1/tenants/${encodeURIComponent(slug)}/students`;
      const result = await fetch(editing ? `${root}/${encodeURIComponent(editing)}` : root, {
        method: editing ? "PUT" : "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken },
        credentials: "same-origin",
        redirect: "error",
        body: JSON.stringify(body),
      });
      if (result.status === 401 || result.status === 403 || result.status === 404) {
        denied.current = true;
        setUnavailable(true);
        setError("Your session, school access, or record changed. Refresh before trying again.");
      } else if (result.status === 409) {
        setError("This admission number is already in use.");
      } else if (result.status === 422) {
        setError("Check the student details and try again.");
      } else if (!result.ok) {
        setError("Unable to save the student record. Try again.");
      } else {
        reset();
        setMessage(editing ? "Student profile updated." : "Student registered.");
        router.refresh();
      }
    } catch {
      setError("Unable to reach the service. Try again.");
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }

  const field = "mt-1 w-full rounded-lg border border-slate-400 bg-white px-3 py-2";
  return (
    <>
      <section aria-labelledby="student-list-title" className="space-y-3 rounded-xl border border-slate-200 bg-white p-4 sm:p-6">
        <h2 id="student-list-title" className="text-lg font-semibold">Student records</h2>
        {students.length === 0 ? <p>No students on this page.</p> : (
          <ul className="divide-y divide-slate-200">
            {students.map((student) => (
              <li key={student.id} className="flex flex-wrap items-center justify-between gap-2 py-3">
                <div>
                  <p className="font-medium">{student.display_name}</p>
                  <p className="text-sm text-slate-700">{student.admission_no} · {stages[student.stage]} · {student.status === "ACTIVE" ? "Active" : "Inactive"}</p>
                </div>
                <button type="button" disabled={busy || unavailable} className="min-h-11 px-2 text-green-800 underline disabled:opacity-50"
                  onClick={() => {
                    reset();
                    setEditing(student.id);
                    setFields({ admission_no: student.admission_no, display_name: student.display_name, stage: student.stage });
                  }}>
                  Edit<span className="sr-only"> {student.display_name}</span>
                </button>
                <Link href={`/schools/${encodeURIComponent(slug)}/guardians?${new URLSearchParams({ student_id: student.id })}`} className="min-h-11 px-2 py-2 text-green-800 underline">
                  Guardians<span className="sr-only"> for {student.display_name}</span>
                </Link>
                <Link href={`/schools/${encodeURIComponent(slug)}/students/${encodeURIComponent(student.id)}/enrollments`} className="min-h-11 px-2 py-2 text-green-800 underline">
                  Enrollment<span className="sr-only"> for {student.display_name}</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
        <div className="flex flex-wrap gap-3">
          {firstPage && <Link href={firstPage} className="text-green-800 underline">First page</Link>}
          {nextPage && <Link href={nextPage} className="text-green-800 underline">Next students</Link>}
        </div>
      </section>
      <section aria-labelledby="student-form-title" className="rounded-xl border border-slate-200 bg-white p-4 sm:p-6">
        <h2 id="student-form-title" className="mb-4 text-lg font-semibold">{editing ? "Edit student profile" : "Register student"}</h2>
        <form onSubmit={save} className="space-y-4">
          <fieldset disabled={busy || unavailable} className="space-y-4">
            <label className="block">Admission number<input className={field} required maxLength={40} value={fields.admission_no} onChange={(event) => setFields({ ...fields, admission_no: event.target.value })} /></label>
            <label className="block">Display name<input className={field} required maxLength={150} value={fields.display_name} onChange={(event) => setFields({ ...fields, display_name: event.target.value })} /></label>
            <label className="block">Education stage<select className={field} value={fields.stage} onChange={(event) => setFields({ ...fields, stage: event.target.value as StudentFields["stage"] })}>
              {Object.entries(stages).map(([stage, label]) => <option key={stage} value={stage}>{label}</option>)}
            </select></label>
          </fieldset>
          <div className="flex flex-wrap gap-3">
            <button type="submit" disabled={busy || unavailable} className="min-h-11 rounded-lg bg-green-800 px-4 text-white disabled:opacity-50">
              {busy ? "Saving…" : "Save student"}
            </button>
            {editing && <button type="button" disabled={busy} onClick={reset} className="min-h-11 px-2 underline">Cancel edit</button>}
          </div>
          {error && <p role="alert" className="text-red-800">{error}</p>}
          {message && <p role="status" className="text-green-900">{message}</p>}
        </form>
      </section>
    </>
  );
}