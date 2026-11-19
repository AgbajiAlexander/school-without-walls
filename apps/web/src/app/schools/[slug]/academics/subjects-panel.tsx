"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  archiveSubject,
  createSubject,
  listSubjects,
  updateSubject,
  type SchoolSubject,
} from "@/lib/academics-client";

interface Props {
  schoolSlug: string;
  currentRole: string | null;
}

type Editor = {
  id: string;
  code: string;
  name: string;
  description: string;
};

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Something went wrong.";
}

function validSubject(code: string, name: string, description: string): string | null {
  if (!/^[A-Z0-9][A-Z0-9_-]{0,31}$/.test(code)) {
    return "Subject code must be 1–32 characters using uppercase letters, numbers, _ or -.";
  }
  if (!name.trim() || name.trim().length > 100) {
    return "Subject name must contain 1–100 characters.";
  }
  if (description.length > 500) {
    return "Description must not exceed 500 characters.";
  }
  return null;
}

export default function SubjectsPanel({ schoolSlug, currentRole }: Props) {
  const canManage = currentRole === "SCHOOL_ADMIN" || currentRole === "PRINCIPAL";
  const [subjects, setSubjects] = useState<SchoolSubject[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [refresh, setRefresh] = useState(0);
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [editor, setEditor] = useState<Editor | null>(null);
  const [archiveID, setArchiveID] = useState<string | null>(null);
  const activeRef = useRef(false);
  const requestContext = `${schoolSlug}:${refresh}`;
  const contextRef = useRef(requestContext);
  contextRef.current = requestContext;

  useEffect(() => {
    let active = true;
    activeRef.current = true;
    setLoading(true);
    setError("");
    setNotice("");
    setSubjects([]);
    setBusy(false);
    setCode("");
    setName("");
    setDescription("");
    setEditor(null);
    setArchiveID(null);

    listSubjects(schoolSlug)
      .then((values) => {
        if (active) setSubjects(values);
      })
      .catch((cause: unknown) => {
        if (active) setError(errorMessage(cause));
      })
      .finally(() => {
        if (active) setLoading(false);
      });

    return () => {
      active = false;
      activeRef.current = false;
    };
  }, [schoolSlug, refresh]);

  async function handleCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canManage || busy) return;

    const normalizedCode = code.trim().toUpperCase();
    const normalizedName = name.trim();
    const issue = validSubject(normalizedCode, normalizedName, description.trim());
    if (issue) {
      setError(issue);
      return;
    }

    setBusy(true);
    setError("");
    setNotice("");

    try {
      const created = await createSubject(schoolSlug, {
        code: normalizedCode,
        name: normalizedName,
        description: description.trim() || undefined,
      });
      if (!activeRef.current || contextRef.current !== requestContext) return;
      setSubjects((current) => [...current, created].sort((a, b) => a.name.localeCompare(b.name)));
      setCode("");
      setName("");
      setDescription("");
      setNotice("Subject created successfully.");
    } catch (cause) {
      if (activeRef.current && contextRef.current === requestContext) setError(errorMessage(cause));
    } finally {
      if (activeRef.current && contextRef.current === requestContext) setBusy(false);
    }
  }

  async function handleUpdate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canManage || busy || !editor) return;

    const normalizedCode = editor.code.trim().toUpperCase();
    const normalizedName = editor.name.trim();
    const issue = validSubject(normalizedCode, normalizedName, editor.description.trim());
    if (issue) {
      setError(issue);
      return;
    }

    setBusy(true);
    setError("");
    setNotice("");

    try {
      // Empty descriptions are not sent: backend clearing is not yet supported.
      const updated = await updateSubject(schoolSlug, editor.id, {
        code: normalizedCode,
        name: normalizedName,
        ...(editor.description.trim() ? { description: editor.description.trim() } : {}),
      });
      if (!activeRef.current || contextRef.current !== requestContext) return;
      setSubjects((current) => current.map((item) => item.id === updated.id ? updated : item));
      setEditor(null);
      setNotice("Subject updated successfully.");
    } catch (cause) {
      if (activeRef.current && contextRef.current === requestContext) setError(errorMessage(cause));
    } finally {
      if (activeRef.current && contextRef.current === requestContext) setBusy(false);
    }
  }

  async function handleArchive(subjectID: string) {
    if (!canManage || busy || archiveID !== subjectID) return;

    setBusy(true);
    setError("");
    setNotice("");

    try {
      const archived = await archiveSubject(schoolSlug, subjectID);
      if (!activeRef.current || contextRef.current !== requestContext) return;
      setSubjects((current) => current.map((item) => item.id === archived.id ? archived : item));
      setArchiveID(null);
      setNotice("Subject archived successfully.");
    } catch (cause) {
      if (activeRef.current && contextRef.current === requestContext) setError(errorMessage(cause));
    } finally {
      if (activeRef.current && contextRef.current === requestContext) setBusy(false);
    }
  }

  const fieldClass = "w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm";

  return (
    <section aria-label="Subject Management" className="mt-8 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-7">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold text-slate-900">Subject Management</h2>
          <p className="mt-1 text-sm text-slate-600">Manage your school&apos;s subject catalog.</p>
        </div>
        <span className="text-sm text-slate-600">
          {canManage ? "Subject administration" : "Read-only access"}
        </span>
      </div>

      {error && <p role="alert" className="mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-800">{error}</p>}
      {notice && <p role="status" className="mt-4 rounded-lg bg-green-50 p-3 text-sm text-green-800">{notice}</p>}

      {canManage && (
        <form onSubmit={handleCreate} className="mt-6 grid gap-3 rounded-xl bg-slate-50 p-4">
          <h3 className="font-semibold text-slate-900">Add subject</h3>
          <label className="text-sm font-medium">Subject code
            <input className={fieldClass} value={code} maxLength={32} onChange={(event) => setCode(event.target.value)} required />
          </label>
          <label className="text-sm font-medium">Subject name
            <input className={fieldClass} value={name} maxLength={100} onChange={(event) => setName(event.target.value)} required />
          </label>
          <label className="text-sm font-medium">Description (optional)
            <textarea className={fieldClass} value={description} maxLength={500} onChange={(event) => setDescription(event.target.value)} />
          </label>
          <button type="submit" disabled={busy || loading} className="rounded-lg bg-green-800 px-4 py-2 text-sm font-semibold text-white disabled:opacity-50">
            {busy ? "Saving..." : "Add Subject"}
          </button>
        </form>
      )}

      <div className="mt-6 flex items-center justify-between">
        <h3 className="font-semibold text-slate-900">School subjects</h3>
        <button type="button" onClick={() => setRefresh((value) => value + 1)} disabled={loading || busy} className="text-sm font-medium text-green-800 disabled:opacity-50">Refresh subjects</button>
      </div>

      {loading ? (
        <p role="status" className="mt-4 text-sm text-slate-600">Loading subjects...</p>
      ) : subjects.length === 0 ? (
        <p className="mt-4 text-sm text-slate-600">No subjects available for this school.</p>
      ) : (
        <ul className="mt-4 space-y-3">
          {subjects.map((subject) => (
            <li key={subject.id} className="rounded-xl border border-slate-200 p-4">
              <div className="flex flex-wrap justify-between gap-3">
                <div>
                  <p className="font-semibold text-slate-900">{subject.code} — {subject.name}</p>
                  {subject.description && <p className="mt-1 text-sm text-slate-600">{subject.description}</p>}
                  <p className="mt-1 text-xs text-slate-500">{subject.status}</p>
                </div>
                {canManage && subject.status === "ACTIVE" && (
                  <div className="flex items-start gap-3">
                    <button type="button" disabled={busy} onClick={() => {
                      setEditor({ id: subject.id, code: subject.code, name: subject.name, description: subject.description ?? "" });
                      setArchiveID(null);
                      setError("");
                      setNotice("");
                    }} className="text-sm font-semibold text-green-800 disabled:opacity-50">Edit</button>
                    <button type="button" disabled={busy} onClick={() => {
                      setArchiveID(subject.id);
                      setEditor(null);
                      setError("");
                      setNotice("");
                    }} className="text-sm font-semibold text-red-700 disabled:opacity-50">Archive</button>
                  </div>
                )}
              </div>

              {canManage && editor?.id === subject.id && (
                <form onSubmit={handleUpdate} className="mt-4 grid gap-3 border-t border-slate-200 pt-4">
                  <label className="text-sm font-medium">Edit subject code
                    <input className={fieldClass} value={editor.code} maxLength={32} onChange={(event) => setEditor({ ...editor, code: event.target.value })} required />
                  </label>
                  <label className="text-sm font-medium">Edit subject name
                    <input className={fieldClass} value={editor.name} maxLength={100} onChange={(event) => setEditor({ ...editor, name: event.target.value })} required />
                  </label>
                  <label className="text-sm font-medium">Edit subject description
                    <textarea className={fieldClass} value={editor.description} maxLength={500} onChange={(event) => setEditor({ ...editor, description: event.target.value })} />
                  </label>
                  <p className="text-xs text-slate-500">Clearing an existing description is not yet supported.</p>
                  <div className="flex gap-3">
                    <button type="submit" disabled={busy} className="rounded-lg bg-green-800 px-4 py-2 text-sm font-semibold text-white disabled:opacity-50">Save Subject</button>
                    <button type="button" disabled={busy} onClick={() => setEditor(null)} className="text-sm font-medium text-slate-700">Cancel</button>
                  </div>
                </form>
              )}

              {canManage && archiveID === subject.id && (
                <div className="mt-4 border-t border-slate-200 pt-4">
                  <p className="text-sm text-red-800">Archive {subject.name}? This preserves its historical records.</p>
                  <div className="mt-3 flex gap-3">
                    <button type="button" disabled={busy} onClick={() => void handleArchive(subject.id)} className="rounded-lg bg-red-700 px-4 py-2 text-sm font-semibold text-white disabled:opacity-50">Confirm Archive</button>
                    <button type="button" disabled={busy} onClick={() => setArchiveID(null)} className="text-sm font-medium text-slate-700">Cancel</button>
                  </div>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}