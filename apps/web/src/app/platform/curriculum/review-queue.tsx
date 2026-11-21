"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState } from "react";

export type DraftAlignment = {
  id: string;
  relation: string;
  primary_code: string;
  primary_text: string;
  primary_framework: string;
  primary_edition: string;
  primary_status: string;
  related_code: string;
  related_text: string;
  related_framework: string;
  related_edition: string;
  related_status: string;
};

export default function ReviewQueue({ drafts, csrf, first, next }: {
  drafts: DraftAlignment[];
  csrf: string;
  first?: string;
  next?: string;
}) {
  const router = useRouter();
  const request = useRef(false);
  const [selected, setSelected] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [denied, setDenied] = useState(false);
  const [approved, setApproved] = useState<string[]>([]);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  async function approve(id: string) {
    if (request.current || denied || selected !== id) return;
    request.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const response = await fetch(`/api/v1/curriculum/alignments/${encodeURIComponent(id)}/review`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: "{}",
      });
      if (response.status === 200) {
        setApproved((items) => [...items, id]);
        setSelected(undefined);
        setNotice("Alignment approved.");
        router.refresh();
      } else if (response.status === 401 || response.status === 403) {
        setDenied(true);
        setError("Review access is unavailable. Refresh or sign in again.");
      } else if (response.status === 409) {
        setSelected(undefined);
        setError("This draft is no longer ready for approval. The queue has been refreshed.");
        router.refresh();
      } else {
        setError("Approval could not be confirmed. Refresh before trying again.");
      }
    } catch {
      setError("Approval could not be confirmed. Refresh before trying again.");
    } finally {
      request.current = false;
      setBusy(false);
    }
  }

  const pending = drafts.filter((draft) => !approved.includes(draft.id));
  return (
    <>
      {notice && <p role="status" className="text-green-800">{notice}</p>}
      {error && <p role="alert" className="text-red-800">{error}</p>}
      {!denied && <>
        {pending.length === 0 ? <p>No draft mappings on this page.</p> : (
          <ul className="space-y-4">
            {pending.map((draft) => {
              const ready = draft.primary_status === "PUBLISHED" && draft.related_status === "PUBLISHED";
              return (
                <li key={draft.id} className="space-y-3 rounded-xl border border-slate-200 bg-white p-4">
                  <h2 className="font-semibold">{draft.primary_code} → {draft.related_code}</h2>
                  <div className="grid gap-3 break-words sm:grid-cols-2">
                    <div><h3 className="font-medium">Nigerian objective</h3><p>{draft.primary_text}</p>
                      <p className="text-sm">{draft.primary_framework} · {draft.primary_edition} · {draft.primary_status}</p></div>
                    <div><h3 className="font-medium">Optional enrichment</h3><p>{draft.related_text}</p>
                      <p className="text-sm">{draft.related_framework} · {draft.related_edition} · {draft.related_status}</p></div>
                  </div>
                  <p>Relationship: {draft.relation.toLowerCase()}</p>
                  {!ready ? <p>Both editions must be published before approval.</p> : selected === draft.id ? (
                    <div className="flex flex-wrap items-center gap-3">
                      <p>Make this mapping visible as verified enrichment?</p>
                      <button type="button" className="min-h-11 rounded bg-green-800 px-3 text-white"
                        disabled={busy} onClick={() => void approve(draft.id)}>{busy ? "Approving…" : "Confirm approval"}</button>
                      <button type="button" className="min-h-11 underline" disabled={busy} onClick={() => setSelected(undefined)}>Cancel</button>
                    </div>
                  ) : (
                    <button type="button" className="min-h-11 text-green-800 underline" disabled={busy}
                      onClick={() => { setSelected(draft.id); setNotice(""); setError(""); }}>
                      Review mapping <span className="sr-only">{draft.primary_code} to {draft.related_code}</span>
                    </button>
                  )}
                </li>
              );
            })}
          </ul>
        )}
        {(first || next) && <nav aria-label="Review queue pages" className="flex gap-4">
          {first && <Link className="py-2 text-green-800 underline" href={first}>First page</Link>}
          {next && <Link className="py-2 text-green-800 underline" href={next}>Next page</Link>}
        </nav>}
      </>}
    </>
  );
}
