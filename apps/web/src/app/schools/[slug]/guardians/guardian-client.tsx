"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";

interface Guardian {
  id: string;
  display_name: string;
  contact_email: string | null;
  relationship?: string;
}

export interface GuardianPageData { guardians: Guardian[]; next_cursor?: string }

export default function GuardianClient({ slug, csrf, guardians, student, linked, next, nextLinked, first }: {
  slug: string;
  csrf: string;
  guardians: Guardian[];
  student?: { id: string; name: string };
  linked: Guardian[];
  next?: string;
  nextLinked?: string;
  first?: string;
}) {
  const router = useRouter();
  const pending = useRef(false);
  const accessLost = useRef(false);
  const [busy, setBusy] = useState(false);
  const [denied, setDenied] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [editing, setEditing] = useState<string | null>(null);
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [guardian, setGuardian] = useState("");
  const [relationship, setRelationship] = useState("");
  const [unlinking, setUnlinking] = useState<string | null>(null);

  const api = `/api/v1/tenants/${encodeURIComponent(slug)}`;
  function resetProfile() {
    setEditing(null);
    setDisplayName("");
    setEmail("");
  }

  async function mutate(method: string, path: string, message: string, body?: Record<string, string | null>) {
    if (pending.current || accessLost.current) return false;
    pending.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const response = await fetch(api + path, {
        method,
        credentials: "same-origin",
        redirect: "error",
        headers: { "X-CSRF-Token": csrf, ...(body ? { "Content-Type": "application/json" } : {}) },
        body: body ? JSON.stringify(body) : undefined,
      });
      if ([401, 403, 404].includes(response.status)) {
        accessLost.current = true;
        setDenied(true);
        setError("Your session, school access, or record changed. Refresh before trying again.");
        return false;
      }
      if (!response.ok) {
        setError(response.status === 422 ? "Check the guardian details and try again." : "Unable to save the guardian change. Try again.");
        return false;
      }
      setNotice(message);
      router.refresh();
      return true;
    } catch {
      setError("Unable to reach the service. Try again.");
      return false;
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  async function saveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!displayName.trim()) {
      setError("Enter a guardian display name.");
      return;
    }
    const saved = await mutate(editing ? "PUT" : "POST", editing ? "/guardians/" + encodeURIComponent(editing) : "/guardians",
      "Guardian profile saved.", { display_name: displayName.trim(), contact_email: email.trim() || null });
    if (saved) resetProfile();
  }

  function relationshipPath(id: string) {
    return `/students/${encodeURIComponent(student?.id ?? "")}/guardians/${encodeURIComponent(id)}`;
  }

  async function linkGuardian(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!student || !guardians.some(({ id }) => id === guardian) || !relationship.trim()) {
      setError("Choose a guardian and enter their relationship to the student.");
      return;
    }
    if (await mutate("PUT", relationshipPath(guardian), "Guardian relationship saved.", { relationship: relationship.trim() })) {
      setGuardian("");
      setRelationship("");
    }
  }

  const field = "mt-1 w-full rounded-lg border border-slate-400 bg-white px-3 py-2";
  const panel = "space-y-3 rounded-xl border border-slate-200 bg-white p-4 sm:p-6";
  const button = "min-h-11 rounded-lg bg-green-800 px-4 text-white disabled:opacity-50";
  return (
    <>
      {student && (
        <section className={panel} aria-labelledby="linked-guardians">
          <h2 id="linked-guardians" className="text-lg font-semibold">Guardians for {student.name}</h2>
          {linked.length === 0 ? <p>No linked guardians on this page.</p> : (
            <ul className="divide-y divide-slate-200">
              {linked.map((record) => (
                <li key={record.id} className="py-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <p>{record.display_name} · {record.relationship}</p>
                    <button type="button" disabled={busy || denied} onClick={() => setUnlinking(record.id)} className="min-h-11 px-2 text-red-800 underline">
                      Unlink <span className="sr-only">{record.display_name}</span>
                    </button>
                  </div>
                  {unlinking === record.id && (
                    <div className="mt-2 flex flex-wrap items-center gap-2">
                      <p>This removes access from the guardian relationship.</p>
                      <button type="button" disabled={busy || denied} className={button}
                        onClick={async () => {
                          if (await mutate("DELETE", relationshipPath(record.id), "Guardian unlinked.")) setUnlinking(null);
                        }}>Confirm unlink</button>
                      <button type="button" disabled={busy} onClick={() => setUnlinking(null)} className="min-h-11 px-2 underline">Cancel</button>
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}
          {nextLinked && <Link className="text-green-800 underline" href={nextLinked}>Next linked guardians</Link>}
          <form onSubmit={linkGuardian} className="space-y-3 border-t border-slate-200 pt-4">
            <fieldset disabled={busy || denied} className="space-y-3">
              <label className="block">Guardian to link<select required value={guardian} className={field} onChange={(event) => setGuardian(event.target.value)}>
                <option value="">Select a guardian</option>
                {guardians.map((record) => <option key={record.id} value={record.id}>{record.display_name}{record.contact_email ? ` — ${record.contact_email}` : ` — ${record.id.slice(0, 8)}`}</option>)}
              </select></label>
              <label className="block">Relationship<input required maxLength={40} value={relationship} className={field} onChange={(event) => setRelationship(event.target.value)} /></label>
              <button type="submit" disabled={!guardian} className={button}>Save relationship</button>
            </fieldset>
          </form>
        </section>
      )}
      <section className={panel} aria-labelledby="guardian-directory">
        <h2 id="guardian-directory" className="text-lg font-semibold">Guardian directory</h2>
        {guardians.length === 0 ? <p>No guardian profiles on this page.</p> : (
          <ul className="divide-y divide-slate-200">
            {guardians.map((record) => (
              <li key={record.id} className="flex flex-wrap items-center justify-between gap-2 py-3">
                <div><p className="font-medium">{record.display_name}</p>{record.contact_email && <p className="break-all text-sm text-slate-700">{record.contact_email}</p>}</div>
                <button type="button" disabled={busy || denied} className="min-h-11 px-2 text-green-800 underline"
                  onClick={() => { setEditing(record.id); setDisplayName(record.display_name); setEmail(record.contact_email ?? ""); setError(""); setNotice(""); }}>
                  Edit <span className="sr-only">{record.display_name}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
        <div className="flex flex-wrap gap-3">
          {first && <Link className="text-green-800 underline" href={first}>First page</Link>}
          {next && <Link className="text-green-800 underline" href={next}>Next guardian profiles</Link>}
        </div>
      </section>
      <section className={panel} aria-labelledby="guardian-profile">
        <h2 id="guardian-profile" className="text-lg font-semibold">{editing ? "Edit guardian profile" : "Register guardian"}</h2>
        <form onSubmit={saveProfile} className="space-y-3">
          <fieldset disabled={busy || denied} className="space-y-3">
            <label className="block">Guardian display name<input required maxLength={150} className={field} value={displayName} onChange={(event) => setDisplayName(event.target.value)} /></label>
            <label className="block">Contact email<input type="email" maxLength={254} className={field} value={email} onChange={(event) => setEmail(event.target.value)} /></label>
            <button type="submit" className={button}>Save guardian profile</button>
            {editing && <button type="button" className="min-h-11 px-3 underline" onClick={resetProfile}>Cancel edit</button>}
          </fieldset>
        </form>
      </section>
      {error && <p role="alert" className="text-red-800">{error}</p>}
      {notice && <p role="status" className="text-green-900">{notice}</p>}
    </>
  );
}