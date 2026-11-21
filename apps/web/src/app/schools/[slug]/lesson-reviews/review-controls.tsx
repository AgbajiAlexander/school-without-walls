"use client";

import { useRouter } from "next/navigation";
import { useRef, useState } from "react";

import LessonDownload from "../learning/[lessonID]/lesson-download";

export type ReviewItem = { id: string; title: string; content: string; revision: number; has_attachment?: boolean; enrichment?: { code: string; text: string; withdrawn: boolean }[] };

export default function ReviewControls({ base, csrf, items, required, canConfigure }: {
  base: string; csrf: string; items: ReviewItem[]; required: boolean; canConfigure: boolean;
}) {
  const router = useRouter();
  const lock = useRef(false);
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [confirmed, setConfirmed] = useState<{ id: string; revision: number }>();
  const [blocked, setBlocked] = useState(false);
  async function write(path: string, method: "POST" | "PUT", body: object, success: string) {
    if (lock.current || blocked) return;
    lock.current = true;
    setPending(true);
    setMessage("");
    setError("");
    try {
      const response = await fetch(`${base}/${path}`, { method, credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(body) });
      if (response.status === 200) {
        setConfirmed(undefined);
        setMessage(success);
        router.refresh();
      } else {
        if ([401, 403, 404].includes(response.status)) setBlocked(true);
        setError("The change could not be saved. Refresh to check the current review and your access.");
      }
    } catch {
      setError("Could not confirm the change. Refresh before trying again.");
    } finally {
      lock.current = false;
      setPending(false);
    }
  }
  return (
    <>
      <section className="space-y-2 rounded-xl border p-4">
        <h2 className="text-lg font-semibold">Publication policy</h2>
        <p>{required ? "Approval is required before publication." : "Approval is optional."}</p>
        {canConfigure && <button type="button" disabled={pending || blocked} className="min-h-11 underline"
          onClick={() => void write("lesson-review-policy", "PUT", { required: !required }, "Review policy saved.")}>
          {required ? "Make approval optional" : "Require lesson approval"}
        </button>}
      </section>
      <h2 className="text-lg font-semibold">Submitted lessons</h2>
      {!items.length && <p>No lessons are awaiting review.</p>}
      {!blocked && items.map((item) => (
        <article key={item.id} className="space-y-2 rounded-xl border p-4">
          <h3 className="font-semibold">{item.title}</h3>
          <p className="whitespace-pre-wrap break-words">{item.content}</p>
          {item.enrichment?.length ? <ul aria-label="Optional enrichment" className="space-y-1">
            {item.enrichment.map((mapping, index) => <li key={index}>{mapping.code} · {mapping.text}{mapping.withdrawn && " (edition withdrawn)"}</li>)}
          </ul> : null}
          {item.has_attachment && (
            <LessonDownload key={item.revision}
              endpoint={base + "/lesson-reviews/" + encodeURIComponent(item.id) + "/media?" + new URLSearchParams({ revision: String(item.revision) })} />
          )}
          {confirmed?.id === item.id && confirmed.revision === item.revision ? <div className="flex flex-wrap items-center gap-3">
            <p>Approve this exact lesson revision?</p>
            <button type="button" disabled={pending} className="min-h-11 rounded bg-green-800 px-3 text-white"
              onClick={() => void write(`lesson-reviews/${encodeURIComponent(item.id)}/approve`, "POST", { revision: item.revision }, "Lesson revision approved.")}>Confirm approval</button>
            <button type="button" disabled={pending} className="min-h-11 underline" onClick={() => setConfirmed(undefined)}>Cancel</button>
          </div> : <button type="button" disabled={pending} className="min-h-11 underline" onClick={() => setConfirmed({ id: item.id, revision: item.revision })}>Review lesson</button>}
        </article>
      ))}
      {message && <p role="status" className="text-green-800">{message}</p>}
      {error && <p role="alert" className="text-red-800">{error}</p>}
    </>
  );
}
