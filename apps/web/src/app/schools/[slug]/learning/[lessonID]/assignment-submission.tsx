"use client";

import { useRef, useState, type FormEvent } from "react";

import type { Feedback } from "../../teaching/submission-feedback";

export type Receipt = { revision: number; submitted_at: string };

export default function AssignmentSubmission({ endpoint, csrf, receipt: initialReceipt, available, resubmit, feedback, maxScore, fileMode = false }: {
  endpoint: string;
  csrf: string;
  receipt?: Receipt;
  available: boolean;
  resubmit: boolean;
  feedback?: Feedback;
  maxScore: number;
  fileMode?: boolean;
}) {
  const [receipt, setReceipt] = useState(initialReceipt);
  const [content, setContent] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const pending = useRef(false);
  const attachment = useRef<HTMLInputElement>(null);
  const attempt = useRef<{ content: string | File; requestId: string }>(null);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending.current || !available || (receipt && !resubmit)) return;
    const form = event.currentTarget;
    const file = attachment.current?.files?.[0];
    if (fileMode && (!file || file.size < 1 || file.size > 4 * 1024 * 1024 ||
      !["application/pdf", "image/png", "image/jpeg"].includes(file.type))) {
      setError("Choose a PDF, PNG or JPEG file up to 4 MiB.");
      return;
    }
    const answer = fileMode ? file : content.trim();
    if (!answer) {
      setError("Enter your answer.");
      return;
    }
    if (attempt.current?.content !== answer) attempt.current = { content: answer, requestId: crypto.randomUUID() };
    pending.current = true;
    setBusy(true);
    setError("");
    try {
      const response = await fetch(endpoint, {
        method: "POST",
        credentials: "same-origin",
        cache: "no-store",
        redirect: "error",
        headers: fileMode && file
          ? { "Content-Type": file.type, "X-CSRF-Token": csrf, "Idempotency-Key": attempt.current.requestId }
          : { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: fileMode ? file : JSON.stringify({ content: answer, request_id: attempt.current.requestId }),
      });
      if (response.status === 409) {
        setError("This assignment is no longer accepting this submission. Refresh to check its status.");
        return;
      }
      if (response.status === 422 && fileMode) {
        setError("The file did not pass type, size or security validation.");
        return;
      }
      if (!response.ok) throw new Error();
      const saved = (await response.json())?.data as Receipt | undefined;
      if (!saved || !Number.isInteger(saved.revision) || !Number.isFinite(Date.parse(saved.submitted_at))) throw new Error();
      setReceipt(saved);
      setContent(""); form.reset();
      attempt.current = null;
    } catch {
      setError("Submission could not be confirmed. Try again with the same answer to check the receipt.");
    } finally {
      setBusy(false);
      pending.current = false;
    }
  }

  return (
    <div className="space-y-2">
      {receipt && <p role="status">Submission {receipt.revision} received on <time dateTime={receipt.submitted_at}>
        {new Date(receipt.submitted_at).toISOString().slice(0, 16).replace("T", " ")} UTC</time>.</p>}
      {feedback && receipt?.revision === initialReceipt?.revision && (
        <div className="space-y-1 border-t pt-2">
          <h4 className="font-semibold">Teacher feedback</h4>
          <p>Score {feedback.score} out of {maxScore}</p>
          <p className="whitespace-pre-wrap break-words">{feedback.comment}</p>
        </div>
      )}
      {available && (!receipt || resubmit) && (
        <form onSubmit={(event) => void submit(event)} className="space-y-2">
          <fieldset disabled={busy} className="space-y-2">
            {fileMode ? <label>Your file<input ref={attachment} type="file" accept="application/pdf,image/png,image/jpeg"
              required className="block min-h-11 w-full max-w-full" /><span className="text-sm">PDF, PNG or JPEG, up to 4 MiB.</span></label>
              : <label>Your answer<textarea value={content} onChange={(event) => setContent(event.target.value)}
              required maxLength={24000} rows={5} className="block w-full rounded border p-2" /></label>}
            <button className="min-h-11 rounded bg-green-800 px-3 text-white" type="submit">
              {busy ? "Sending..." : fileMode ? (receipt ? "Resubmit file" : "Submit file") : receipt ? "Resubmit answer" : "Submit answer"}
            </button>
          </fieldset>
        </form>
      )}
      {error && <p role="alert">{error}</p>}
    </div>
  );
}
