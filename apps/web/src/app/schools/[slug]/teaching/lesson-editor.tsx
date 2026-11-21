"use client";

import { useRouter } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";

export type EnrichmentOption = { id: string; code: string; text: string; selected: boolean; eligible: boolean };

export default function LessonEditor({ endpoint, root, csrf, offering, objectives, lesson }: {
  endpoint: string; root: string; csrf: string; offering?: string;
  objectives?: { id: string; code: string; objective_text: string; enrichment?: EnrichmentOption[] }[];
  lesson?: { id: string; title: string; status: string; content?: string; objective_code: string; review_required: boolean; review_status: string; enrichment?: EnrichmentOption[] };
}) {
  const router = useRouter();
  const [title, setTitle] = useState(lesson?.title ?? "");
  const [content, setContent] = useState(lesson?.content ?? "");
  const [objective, setObjective] = useState("");
  const [enrichment, setEnrichment] = useState(() => lesson?.enrichment?.filter((item) => item.selected).map((item) => item.id) ?? []);
  const options = (lesson ? lesson.enrichment : objectives?.find((item) => item.id === objective)?.enrichment) ?? [];
  const savedEnrichment = lesson?.enrichment?.filter((item) => item.selected).map((item) => item.id) ?? [];
  const enrichmentChanged = enrichment.length !== savedEnrichment.length || enrichment.some((id) => !savedEnrichment.includes(id));
  const stale = options.some((item) => !item.eligible && enrichment.includes(item.id));
  const [confirmation, setConfirmation] = useState<"publish" | "archive">();
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [blocked, setBlocked] = useState(false);
  const [previousStatus, setPreviousStatus] = useState<string>();
  const waitingForState = previousStatus === lesson?.status && previousStatus !== undefined;
  const unsubmitted = lesson?.status === "DRAFT" && (title !== lesson.title || content !== lesson.content || enrichmentChanged);
  const inFlight = useRef(false);
  const url = lesson ? `${endpoint}/${encodeURIComponent(lesson.id)}` : endpoint;

  async function write(action: "save" | "publish" | "archive" | "submit") {
    if (blocked || waitingForState || inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const result = await fetch(action === "save" ? url : `${url}/${action}`, {
        method: action === "save" && lesson ? "PATCH" : "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: action === "save" ? JSON.stringify(lesson ? { title, content, enrichment_alignment_ids: enrichment } :
          { title, content, offering_id: offering, primary_objective_id: objective, enrichment_alignment_ids: enrichment }) : undefined,
      });
      if (result.status === 201) {
        const payload = await result.json();
        const id = payload?.data?.id;
        if (typeof id !== "string") throw new Error("Missing saved lesson identifier");
        setBlocked(true);
        router.replace(`${root}?${new URLSearchParams({ lesson: id })}`);
        router.refresh();
      } else if (result.status === 200) {
        setConfirmation(undefined);
        setNotice(action === "publish" ? "Lesson published." : action === "archive" ? "Lesson archived." : action === "submit" ? "Submitted for review." : "Draft saved.");
        if (action === "publish" || action === "archive") setPreviousStatus(lesson?.status);
        router.refresh();
      } else if ([401, 403, 404, 409].includes(result.status)) {
        setBlocked(true);
        setError("This lesson or action is unavailable. Refresh to check your access and the lesson state.");
      } else {
        setError(result.status === 422 ? "Check the text and curriculum objective." : "The change could not be confirmed. Refresh before trying again.");
      }
    } catch {
      setError("The change could not be confirmed. Refresh before trying again.");
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }
  function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void write("save");
  }

  return (
    <section className="space-y-3 rounded-xl border p-4">
      <h2 className="text-lg font-semibold">{lesson ? lesson.title : "Create lesson draft"}</h2>
      {lesson && <p className="text-sm">{lesson.status} Â· {lesson.objective_code}</p>}
      {(!lesson || lesson.status === "DRAFT") ? <form onSubmit={save}>
        <fieldset disabled={busy || blocked} className="space-y-3">
          {!lesson && <label>Primary objective<select required value={objective} onChange={(e) => { setObjective(e.target.value); setEnrichment([]); }} className="block min-h-11 w-full rounded border">
            <option value="">Select an objective</option>
            {objectives?.map((item) => <option key={item.id} value={item.id}>{item.code} Â· {item.objective_text}</option>)}
          </select></label>}
          {options.length > 0 && <fieldset className="space-y-2">
            <legend className="font-medium">Optional enrichment</legend>
            {options.map((item) => (
              <label key={item.id} className="flex min-h-11 items-start gap-2">
                <input type="checkbox" className="mt-1" checked={enrichment.includes(item.id)}
                  disabled={!enrichment.includes(item.id) && (!item.eligible || enrichment.length >= 10)}
                  onChange={(event) => setEnrichment((current) => event.target.checked
                    ? [...current, item.id] : current.filter((id) => id !== item.id))} />
                <span>{item.code} · {item.text}{!item.eligible && " (no longer eligible)"}</span>
              </label>
            ))}
          </fieldset>}
          <label>Lesson title<input required maxLength={160} value={title} onChange={(e) => setTitle(e.target.value)} className="block min-h-11 w-full rounded border px-2" /></label>
          <label>Lesson text<textarea required maxLength={24000} rows={6} value={content} onChange={(e) => setContent(e.target.value)} className="block w-full rounded border p-2" /></label>
          <button type="submit" className="min-h-11 rounded bg-green-800 px-3 text-white">{busy ? "Savingâ€¦" : lesson ? "Save changes" : "Save lesson draft"}</button>
        </fieldset>
      </form> : <div className="whitespace-pre-wrap break-words">{lesson.content}</div>}
      {lesson?.status === "DRAFT" && lesson.review_required && (
        <div className="space-y-2">
          {lesson.review_status === "PENDING" ? <p>Awaiting approval.</p>
            : lesson.review_status === "APPROVED" ? <p>Current revision approved.</p>
              : <button type="button" disabled={busy || blocked || unsubmitted} className="min-h-11 text-green-800 underline"
                onClick={() => void write("submit")}>Submit for review</button>}
        </div>
      )}
      {lesson && lesson.status !== "ARCHIVED" && !blocked && !waitingForState &&
        (lesson.status !== "DRAFT" || !lesson.review_required || lesson.review_status === "APPROVED") && (
        confirmation ? <div className="flex flex-wrap items-center gap-3">
          <p>{confirmation === "publish" ? "Publish this lesson for enrolled students?" : "Remove this lesson from student reading?"}</p>
          <button type="button" disabled={busy} onClick={() => void write(confirmation)} className="min-h-11 rounded bg-green-800 px-3 text-white">
            {confirmation === "publish" ? "Confirm publication" : "Confirm archive"}
          </button>
          <button type="button" disabled={busy} onClick={() => setConfirmation(undefined)} className="min-h-11 underline">Cancel</button>
        </div> : <button type="button" disabled={busy || unsubmitted || (lesson.status === "DRAFT" && stale)} onClick={() => setConfirmation(lesson.status === "DRAFT" ? "publish" : "archive")} className="min-h-11 text-green-800 underline">
          {lesson.status === "DRAFT" ? "Publish lesson" : "Archive lesson"}
        </button>
      )}
      {notice && <p role="status" className="text-green-800">{notice}</p>}
      {error && <p role="alert" className="text-red-800">{error}</p>}
    </section>
  );
}
