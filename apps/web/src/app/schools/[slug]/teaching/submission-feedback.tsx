"use client";

import { useRouter } from "next/navigation";
import { useId, useRef, useState, type FormEvent } from "react";

import LessonDownload from "../learning/[lessonID]/lesson-download";

export type Feedback = { score: number; comment: string; version: number };
export type ReviewSubmission = { id: string; student_name: string; content?: string; has_file?: boolean; revision: number; feedback?: Feedback };

export default function SubmissionFeedback({ submission, endpoint, csrf, maxScore }: {
  submission: ReviewSubmission; endpoint: string; csrf: string; maxScore: number;
}) {
  const router = useRouter();
  const commentID = useId();
  const pending = useRef(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setNotice("");
    setError("");
    const values = new FormData(event.currentTarget);
    try {
      const result = await fetch(endpoint, {
        method: "PUT", cache: "no-store", credentials: "same-origin", redirect: "error",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ score: Number(values.get("score")), comment: values.get("comment"), version: submission.feedback?.version ?? 0 }),
      });
      if (result.status === 409) {
        setError("This submission or its feedback changed. Reload to review the current version.");
      } else if (result.status === 422) {
        setError("Enter a score in range and a feedback comment.");
      } else if (!result.ok) {
        throw new Error();
      } else {
        setNotice("Feedback returned.");
        router.refresh();
      }
    } catch {
      setError("Feedback could not be confirmed. Reload to check the current version.");
    } finally {
      setBusy(false);
      pending.current = false;
    }
  }

  return (
    <article className="space-y-2 rounded border p-3">
      <h3 className="break-words font-semibold">{submission.student_name}</h3>
      <p>Submission {submission.revision}</p>
      <p className="whitespace-pre-wrap break-words">{submission.content}</p>
      {submission.has_file && <LessonDownload endpoint={endpoint.replace(/\/feedback$/, "/file")} />}
      <form onSubmit={(event) => void save(event)}>
        <fieldset disabled={busy} className="grid gap-2">
          <label>Score out of {maxScore}<input name="score" type="number" min={0} max={maxScore}
            required defaultValue={submission.feedback?.score} className="block min-h-11 w-full rounded border px-2" /></label>
          <div>
            <label htmlFor={commentID}>Feedback comment</label>
            <textarea id={commentID} name="comment" required maxLength={4000} rows={4}
              defaultValue={submission.feedback?.comment} className="block w-full rounded border p-2" />
          </div>
          <button type="submit" className="min-h-11 justify-self-start rounded bg-green-800 px-3 text-white">
            {busy ? "Returning..." : submission.feedback ? "Update feedback" : "Return feedback"}
          </button>
        </fieldset>
      </form>
      {notice && <p role="status">{notice}</p>}
      {error && <p role="alert">{error}</p>}
    </article>
  );
}
