"use client";

import { useRef, useState } from "react";

export type ProgressState = "NOT_STARTED" | "IN_PROGRESS" | "COMPLETED";
const labels: Record<ProgressState, string> = {
  NOT_STARTED: "Not started",
  IN_PROGRESS: "In progress",
  COMPLETED: "Completed",
};

export default function LessonProgress({ endpoint, initialState, csrf }: {
  endpoint: string;
  initialState: ProgressState;
  csrf: string;
}) {
  const [state, setState] = useState(initialState);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const pending = useRef(false);

  async function save(next: Exclude<ProgressState, "NOT_STARTED">) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError("");
    try {
      const response = await fetch(endpoint, {
        method: "PUT",
        credentials: "same-origin",
        redirect: "error",
        cache: "no-store",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ state: next }),
      });
      if (!response.ok) throw new Error();
      const saved = (await response.json())?.data?.state;
      if (saved !== "IN_PROGRESS" && saved !== "COMPLETED") throw new Error();
      setState(saved);
    } catch {
      setError("Progress could not be confirmed. Reload the lesson before trying again.");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  return (
    <section className="space-y-2 border-t pt-3">
      <h2 className="font-semibold">Your progress</h2>
      <p role="status">{labels[state]}</p>
      {state !== "COMPLETED" && <div className="flex flex-wrap gap-2">
        {state === "NOT_STARTED" && (
          <button type="button" disabled={busy} onClick={() => void save("IN_PROGRESS")} className="min-h-11 rounded border px-3">Start lesson</button>
        )}
        <button type="button" disabled={busy} onClick={() => void save("COMPLETED")} className="min-h-11 rounded bg-green-800 px-3 text-white">Mark complete</button>
      </div>}
      {error && <p role="alert">{error}</p>}
    </section>
  );
}
