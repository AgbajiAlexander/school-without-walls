"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";

export interface StaffRecord { id: string; display_name: string; staff_code: string }
export interface StaffCandidate { membership_id: string; display_name: string; role: string; account_email: string }

export default function StaffClient({ slug, csrfToken, staff, candidates, staffNext, candidateNext, resetPages }: {
  slug: string; csrfToken: string; staff: StaffRecord[]; candidates: StaffCandidate[];
  staffNext?: string; candidateNext?: string; resetPages?: string;
}) {
  const router = useRouter();
  const submitting = useRef(false);
  const [editing,setEditing] = useState<string | null>(null);
  const [membership,setMembership] = useState("");
  const [name,setName] = useState("");
  const [code,setCode] = useState("");
  const [busy,setBusy] = useState(false);
  const [message,setMessage] = useState("");
  const [error,setError] = useState("");
  const [denied,setDenied] = useState(false);
  const canSave = !denied && (editing !== null || candidates.some((candidate) => candidate.membership_id === membership));
  function reset() {
    setEditing(null); setMembership(""); setName(""); setCode("");
  }
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canSave || submitting.current) return;
    if (!name.trim() || !code.trim()) {
      setError("Enter a display name and staff code.");
      return;
    }
    submitting.current = true;
    setBusy(true); setMessage(""); setError("");
    try {
      const base = `/api/v1/tenants/${encodeURIComponent(slug)}/staff`;
      const profile = { display_name: name.trim(), staff_code: code.trim() };
      const response = await fetch(editing ? `${base}/${encodeURIComponent(editing)}` : base, {
        method: editing ? "PUT" : "POST",
        credentials: "same-origin",
        redirect: "error",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken },
        body: JSON.stringify(editing ? profile : { ...profile, membership_id: membership }),
      });
      if (response.status === 401 || response.status === 403) {
        setDenied(true);
        setError("Your session or permissions changed. Refresh the page before trying again.");
      } else if (response.status === 404) {
        setError("This profile or membership is no longer available.");
        router.refresh();
      } else if (response.status === 409) {
        setError("The staff code or membership already has a profile.");
      } else if (response.status === 422) {
        setError("Check the staff profile details and try again.");
      } else if (!response.ok) {
        setError("Unable to save the staff profile. Try again.");
      } else {
        reset();
        setMessage("Staff profile saved.");
        router.refresh();
      }
    } catch {
      setError("Unable to reach the service. Try again.");
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }
  const field = "mt-1 w-full rounded-lg border border-slate-400 bg-white px-3 py-2";
  return (
    <>
      <section aria-labelledby="staff-list-title" className="space-y-3 rounded-xl border border-slate-200 bg-white p-4 sm:p-6">
        <h2 id="staff-list-title" className="text-lg font-semibold">Current profiles</h2>
        {staff.length === 0 ? <p>No staff profiles on this page.</p> : (
          <ul className="divide-y divide-slate-200">
            {staff.map((record) => (
              <li key={record.id} className="flex flex-wrap items-center justify-between gap-2 py-3">
                <div><div className="font-medium">{record.display_name}</div><div className="text-sm text-slate-700">{record.staff_code}</div></div>
                <button type="button" disabled={busy || denied} className="min-h-11 px-2 text-green-800 underline disabled:opacity-50"
                  onClick={() => {
                    reset(); setError(""); setMessage("");
                    setEditing(record.id); setName(record.display_name); setCode(record.staff_code);
                  }}>
                  Edit <span className="sr-only">{record.display_name}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
        {staffNext && <Link href={staffNext} className="text-green-800 underline">Next staff profiles</Link>}
      </section>
      <section aria-labelledby="staff-form-title" className="rounded-xl border border-slate-200 bg-white p-4 sm:p-6">
        <h2 id="staff-form-title" className="mb-4 text-lg font-semibold">{editing ? "Edit staff profile" : "Create staff profile"}</h2>
        <form onSubmit={save} className="space-y-4">
          <fieldset disabled={busy || denied} className="space-y-4">
            {!editing && (
              <>
                <label className="block">
                  Staff membership
                  <select className={field} value={membership} required onChange={(event) => {
                    const selected = candidates.find((candidate) => candidate.membership_id === event.target.value);
                    setMembership(event.target.value);
                    setName(selected?.display_name ?? "");
                  }}>
                    <option value="">Select an active staff member</option>
                    {candidates.map((candidate) => (
                      <option key={candidate.membership_id} value={candidate.membership_id}>
                        {candidate.display_name} ({candidate.role.replaceAll("_"," ").toLowerCase()}) — {candidate.account_email}
                      </option>
                    ))}
                  </select>
                </label>
                {candidates.length === 0 && <p>No eligible staff memberships on this page.</p>}
                {candidateNext && <Link href={candidateNext} className="text-green-800 underline">Next available staff memberships</Link>}
              </>
            )}
            <label className="block">Display name<input className={field} value={name} required maxLength={150} onChange={(event) => setName(event.target.value)} /></label>
            <label className="block">Staff code<input className={field} value={code} required maxLength={40} onChange={(event) => setCode(event.target.value)} /></label>
          </fieldset>
          <div className="flex flex-wrap gap-3">
            <button type="submit" disabled={!canSave || busy} className="min-h-11 rounded-lg bg-green-800 px-4 text-white disabled:opacity-50">
              {busy ? "Saving…" : "Save staff profile"}
            </button>
            {editing && <button type="button" disabled={busy} onClick={reset} className="min-h-11 px-2 underline">Cancel edit</button>}
          </div>
          {error && <p role="alert" className="text-red-800">{error}</p>}
          {message && <p role="status" className="text-green-900">{message}</p>}
        </form>
      </section>
      {resetPages && <Link href={resetPages} className="text-green-800 underline">Back to first page</Link>}
    </>
  );
}