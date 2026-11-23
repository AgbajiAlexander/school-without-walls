"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useId, useRef, useState, type FormEvent } from "react";

export type Assignment = {
  id: string;
  title: string;
  instructions: string;
  due_at: string;
  max_score: number;
  submission_type: "TEXT" | "FILE";
  allow_resubmission: boolean;
  status: "DRAFT" | "OPEN" | "CLOSED";
};

export default function AssignmentControls({ endpoint, csrf, items, lessonPublished, archived, reviewRoot, fileSubmissionEnabled }: {
  endpoint: string;
  csrf: string;
  items: Assignment[];
  lessonPublished: boolean;
  archived: boolean;
  reviewRoot: string;
  fileSubmissionEnabled: boolean;
}) {
  const router = useRouter();
  const instructionsId = useId();
  const pending = useRef(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<Assignment>();
  const [confirmClose, setConfirmClose] = useState<string>();

  async function write(path: string, method: "POST" | "PATCH", body?: object) {
    if (pending.current || archived) return;
    pending.current = true;
    setBusy(true);
    setNotice("");
    setError("");
    try {
      const result = await fetch(endpoint + path, {
        method,
        credentials: "same-origin",
        redirect: "error",
        cache: "no-store",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: body ? JSON.stringify(body) : undefined,
      });
      if (!result.ok) {
        setError(result.status === 422 ? "Check the assignment details."
          : result.status === 409 ? "The assignment cannot change in its current state. Check the deadline and lesson status."
          : "The assignment change could not be confirmed. Refresh and check your access.");
        return;
      }
      setEditing(undefined);
      setConfirmClose(undefined);
      setNotice(path.endsWith("/open") ? "Assignment opened." : path.endsWith("/close") ? "Assignment closed." : "Assignment draft saved.");
      router.refresh();
      return true;
    } catch {
      setError("The assignment change could not be confirmed. Refresh before trying again.");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    const due = new Date(String(values.get("due")) + "Z");
    if (!Number.isFinite(due.getTime())) {
      setError("Enter an assignment deadline.");
      return;
    }
    const saved = await write(editing ? "/" + encodeURIComponent(editing.id) : "", editing ? "PATCH" : "POST", {
      title: values.get("title"),
      instructions: values.get("instructions"),
      due_at: due.toISOString(),
      max_score: Number(values.get("score")),
      submission_type: values.get("kind") ?? editing?.submission_type ?? "TEXT",
      allow_resubmission: values.has("resubmission"),
    });
    if (saved) form.reset();
  }

  return (
    <section className="space-y-4 rounded-xl border p-4">
      <h2 className="text-lg font-semibold">Lesson assignments</h2>
      {!items.length && <p>No assignments on this page.</p>}
      <ul className="space-y-3">
        {items.map((item) => {
          const path = "/" + encodeURIComponent(item.id);
          return (
            <li key={item.id} className="space-y-1 border-b pb-2">
              <h3 className="break-words font-semibold">{item.title}</h3>
              <p>{item.status === "DRAFT" ? "Draft" : item.status === "OPEN" ? "Open" : "Closed"}</p>
              {item.status !== "DRAFT" && <Link href={reviewRoot + "&review_assignment=" + encodeURIComponent(item.id)}
                className="inline-block min-h-11 text-green-800 underline">Review submissions</Link>}
              {!archived && item.status === "DRAFT" && <div className="flex flex-wrap gap-3">
                <button type="button" disabled={busy} onClick={() => setEditing(item)} className="min-h-11 underline">Edit draft</button>
                <button type="button" disabled={busy || !lessonPublished || editing?.id === item.id}
                  onClick={() => void write(path + "/open", "POST")} className="min-h-11 underline">Open assignment</button>
              </div>}
              {!archived && item.status === "OPEN" && (confirmClose === item.id ? <div className="flex flex-wrap items-center gap-3">
                <span>Close this assignment permanently?</span>
                <button type="button" disabled={busy} onClick={() => void write(path + "/close", "POST")} className="min-h-11 underline">Confirm closure</button>
                <button type="button" disabled={busy} onClick={() => setConfirmClose(undefined)} className="min-h-11 underline">Cancel closure</button>
              </div> : <button type="button" disabled={busy} onClick={() => setConfirmClose(item.id)} className="min-h-11 underline">Close assignment</button>)}
            </li>
          );
        })}
      </ul>
      {!archived && <form key={editing?.id ?? "new"} onSubmit={(event) => void save(event)}>
        <fieldset disabled={busy} className="grid gap-2">
          <legend className="font-semibold">{editing ? "Edit assignment draft" : "New assignment"}</legend>
          <label>Assignment title<input name="title" defaultValue={editing?.title} required maxLength={160} className="block min-h-11 w-full rounded border px-2" /></label>
          <div><label htmlFor={instructionsId}>Instructions</label><textarea id={instructionsId} name="instructions" defaultValue={editing?.instructions} required maxLength={24000} rows={4} className="block w-full rounded border p-2" /></div>
          <label>Deadline (UTC)<input name="due" type="datetime-local" defaultValue={editing ? new Date(editing.due_at).toISOString().slice(0, 16) : undefined}
            required className="block min-h-11 w-full rounded border px-2" /></label>
          <label>Maximum score<input name="score" type="number" min={1} max={2147483647} defaultValue={editing?.max_score ?? 10} required
            className="block min-h-11 w-full rounded border px-2" /></label>
          {fileSubmissionEnabled && <label>Submission type<select name="kind" defaultValue={editing?.submission_type ?? "TEXT"}
            className="block min-h-11 w-full rounded border px-2">
            <option value="TEXT">Text answer</option>
            <option value="FILE">File upload</option>
          </select></label>}
          <label className="flex min-h-11 items-center gap-2"><input name="resubmission" type="checkbox" defaultChecked={editing?.allow_resubmission} /> Allow resubmission</label>
          <div className="flex flex-wrap gap-3">
            <button type="submit" className="min-h-11 rounded bg-green-800 px-3 text-white">{busy ? "Saving..." : "Save assignment draft"}</button>
            {editing && <button type="button" onClick={() => setEditing(undefined)} className="min-h-11 underline">Cancel edit</button>}
          </div>
        </fieldset>
      </form>}
      {notice && <p role="status">{notice}</p>}
      {error && <p role="alert">{error}</p>}
    </section>
  );
}
