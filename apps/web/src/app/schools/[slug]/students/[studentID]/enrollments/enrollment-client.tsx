"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";

export interface Choice { id: string; name: string }
export interface Placement {
  id: string;
  session_id: string;
  session_name: string;
  term_name: string;
  class_name: string;
  class_arm_name: string | null;
  status: "ACTIVE" | "ENDED";
  created_at: string;
  ended_at: string | null;
}

export default function EnrollmentClient({ slug, studentID, csrf, placements, terms, classes, activeSession, choicesUnavailable, next, first }: {
  slug: string; studentID: string; csrf: string; placements: Placement[];
  terms: Choice[]; classes: Choice[]; activeSession?: string; choicesUnavailable: boolean;
  next?: string; first?: string;
}) {
  const router = useRouter();
  const armRequest = useRef(0);
  const submitting = useRef(false);
  const accessLost = useRef(false);
  const [transfer, setTransfer] = useState<Placement>();
  const [term, setTerm] = useState("");
  const [classID, setClassID] = useState("");
  const [arm, setArm] = useState("");
  const [arms, setArms] = useState<Choice[]>([]);
  const [armState, setArmState] = useState<"idle" | "loading" | "ready" | "failed">("idle");
  const [busy, setBusy] = useState(false);
  const [denied, setDenied] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const api = `/api/v1/tenants/${encodeURIComponent(slug)}`;

  function reset() {
    armRequest.current++;
    setTransfer(undefined);
    setTerm("");
    setClassID("");
    setArm("");
    setArms([]);
    setArmState("idle");
    setError("");
    setNotice("");
  }

  async function loadArms(id: string) {
    const requestID = ++armRequest.current;
    setClassID(id);
    setArm("");
    setArms([]);
    setArmState(id ? "loading" : "idle");
    if (!id) return;
    try {
      const result = await fetch(api + "/class-arms?" + new URLSearchParams({ class_id: id }), {
        credentials: "same-origin", cache: "no-store", redirect: "error",
      });
      if (!result.ok) throw new Error("Class arms unavailable");
      const payload = await result.json() as { data?: { class_arms?: (Choice & { status: string })[] } };
      if (!Array.isArray(payload.data?.class_arms)) throw new Error("Invalid class arms response");
      if (armRequest.current === requestID) {
        setArms(payload.data.class_arms.filter((item) => item.status === "ACTIVE"));
        setArmState("ready");
      }
    } catch {
      if (armRequest.current === requestID) setArmState("failed");
    }
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (accessLost.current || submitting.current || armState !== "ready" ||
      !classes.some((item) => item.id === classID) || (!transfer && !terms.some((item) => item.id === term)) ||
      (arm && !arms.some((item) => item.id === arm))) return;
    submitting.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const path = `${api}/students/${encodeURIComponent(studentID)}/enrollments`;
      const result = await fetch(transfer ? `${path}/${encodeURIComponent(transfer.id)}/transfers` : path, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        credentials: "same-origin",
        redirect: "error",
        body: JSON.stringify({ class_id: classID, class_arm_id: arm || null, ...(!transfer ? { term_id: term } : {}) }),
      });
      if ([401, 403, 404].includes(result.status)) {
        accessLost.current = true;
        setDenied(true);
        setError("Your session, school access, or placement changed. Refresh before trying again.");
      } else if (result.status === 409) {
        reset();
        setError("An active placement already exists or has changed. Review the refreshed history.");
        router.refresh();
      } else if (!result.ok) {
        setError("Unable to save the placement. Check the selected class and try again.");
      } else {
        reset();
        setNotice(transfer ? "Student transferred." : "Enrollment saved.");
        router.refresh();
      }
    } catch {
      setError("Unable to reach the service. Try again.");
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }

  const panel = "space-y-3 rounded-xl border border-slate-200 bg-white p-4 sm:p-6";
  const field = "mt-1 w-full rounded-lg border border-slate-400 bg-white px-3 py-2";
  const formAvailable = !choicesUnavailable && Boolean(activeSession) && terms.length > 0 && classes.length > 0;
  const date = (value: string) => new Intl.DateTimeFormat("en-NG", { dateStyle: "medium", timeZone: "Africa/Lagos" }).format(new Date(value));

  return (
    <>
      <section aria-labelledby="placement-history" className={panel}>
        <h2 id="placement-history" className="text-lg font-semibold">Placement history</h2>
        {placements.length === 0 ? <p>No placements on this page.</p> : (
          <ul className="divide-y divide-slate-200">
            {placements.map((row) => (
              <li key={row.id} className="flex flex-wrap items-center justify-between gap-2 py-3">
                <div>
                  <p className="font-medium">{row.class_name}{row.class_arm_name ? ` · ${row.class_arm_name}` : ""}</p>
                  <p className="text-sm text-slate-700">{row.session_name} · {row.term_name} · {row.status === "ACTIVE" ? "Active" : "Ended"}</p>
                  <p className="text-sm text-slate-700">{date(row.created_at)}{row.ended_at ? ` – ${date(row.ended_at)}` : ""}</p>
                </div>
                {row.status === "ACTIVE" && row.session_id === activeSession && formAvailable && (
                  <button type="button" disabled={busy || denied} className="min-h-11 px-2 text-green-800 underline"
                    onClick={() => { reset(); setTransfer(row); }}>
                    Transfer<span className="sr-only"> {row.class_name}, {row.term_name}</span>
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
        <div className="flex flex-wrap gap-3">
          {first && <Link className="text-green-800 underline" href={first}>Most recent placements</Link>}
          {next && <Link className="text-green-800 underline" href={next}>Older placements</Link>}
        </div>
      </section>
      <section aria-labelledby="placement-form-title" className={panel}>
        <h2 id="placement-form-title" className="text-lg font-semibold">{transfer ? "Transfer student" : "Enroll student"}</h2>
        {!formAvailable ? <p role="status">{choicesUnavailable
          ? "Unable to load placement options. Refresh the page to try again."
          : "An active session, term, and class are required before enrolling a student."}</p> : (
          <form onSubmit={save} className="space-y-3">
            <fieldset disabled={busy || denied} className="space-y-3">
              {transfer ? <p>Transfer within {transfer.term_name}.</p> : (
                <label className="block">Term<select required className={field} value={term} onChange={(event) => setTerm(event.target.value)}>
                  <option value="">Select a term</option>
                  {terms.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
                </select></label>
              )}
              <label className="block">Class<select required className={field} value={classID} onChange={(event) => void loadArms(event.target.value)}>
                <option value="">Select a class</option>
                {classes.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
              </select></label>
              {armState === "loading" && <p role="status">Loading class arms…</p>}
              {armState === "failed" && <div><p role="alert">Unable to load class arms.</p><button type="button" className="min-h-11 underline" onClick={() => void loadArms(classID)}>Retry class arms</button></div>}
              {armState === "ready" && <label className="block">Class arm<select className={field} value={arm} onChange={(event) => setArm(event.target.value)}>
                <option value="">No class arm</option>
                {arms.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
              </select></label>}
            </fieldset>
            <div className="flex flex-wrap gap-2">
              <button type="submit" disabled={busy || denied || armState !== "ready" || (!transfer && !term)} className="min-h-11 rounded-lg bg-green-800 px-4 text-white disabled:opacity-50">Save placement</button>
              {transfer && <button type="button" disabled={busy} onClick={reset} className="min-h-11 px-2 underline">Cancel transfer</button>}
            </div>
          </form>
        )}
        {error && <p role="alert" className="text-red-800">{error}</p>}
        {notice && <p role="status" className="text-green-900">{notice}</p>}
      </section>
    </>
  );
}