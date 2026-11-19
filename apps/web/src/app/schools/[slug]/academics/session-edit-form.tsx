"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  updateAcademicSession,
  type AcademicSession,
} from "@/lib/academics-client";

interface Props {
  schoolSlug: string;
  session: AcademicSession;
  onUpdated: (session: AcademicSession) => void;
}

export default function SessionEditForm({
  schoolSlug,
  session,
  onUpdated,
}: Props) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(session.name);
  const [startsOn, setStartsOn] = useState(session.starts_on.slice(0, 10));
  const [endsOn, setEndsOn] = useState(session.ends_on.slice(0, 10));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const activeRef = useRef(true);

  useEffect(() => {
    activeRef.current = true;
    return () => {
      activeRef.current = false;
    };
  }, [schoolSlug, session.id]);

  function reset() {
    setName(session.name);
    setStartsOn(session.starts_on.slice(0, 10));
    setEndsOn(session.ends_on.slice(0, 10));
    setError(null);
    setEditing(false);
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (saving) return;

    setError(null);
    setNotice(null);

    const trimmedName = name.trim();

    if (!trimmedName || trimmedName.length > 32) {
      setError("Session name must contain 1 to 32 characters.");
      return;
    }

    if (!startsOn || !endsOn || endsOn < startsOn) {
      setError("End date must be on or after the start date.");
      return;
    }

    setSaving(true);

    try {
      const updated = await updateAcademicSession(
        schoolSlug,
        session.id,
        {
          name: trimmedName,
          starts_on: startsOn,
          ends_on: endsOn,
        },
      );

      if (!activeRef.current) return;

      onUpdated(updated);
      setEditing(false);
      setNotice("Academic session updated successfully.");
    } catch (cause: unknown) {
      if (!activeRef.current) return;

      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to update the academic session.",
      );
    } finally {
      if (activeRef.current) setSaving(false);
    }
  }

  return (
    <div className="mt-4">
      {!editing && (
        <button
          type="button"
          onClick={() => {
            setError(null);
            setNotice(null);
            setEditing(true);
          }}
          className="rounded-lg border border-green-700 px-4 py-2 text-sm font-semibold text-green-800 hover:bg-green-50"
        >
          Edit Session
        </button>
      )}

      {editing && (
        <form
          onSubmit={handleSubmit}
          className="space-y-4 rounded-xl border border-slate-200 bg-white p-4"
        >
          <h4 className="font-semibold text-slate-900">
            Edit academic session
          </h4>

          <label className="block text-sm font-medium text-slate-700">
            Session name
            <input
              aria-label="Edit session name"
              type="text"
              value={name}
              maxLength={32}
              onChange={(event) => setName(event.target.value)}
              disabled={saving}
              required
              className="mt-1 w-full rounded-lg border border-slate-300 p-2"
            />
          </label>

          <label className="block text-sm font-medium text-slate-700">
            Start date
            <input
              aria-label="Edit session start date"
              type="date"
              value={startsOn}
              onChange={(event) => setStartsOn(event.target.value)}
              disabled={saving}
              required
              className="mt-1 w-full rounded-lg border border-slate-300 p-2"
            />
          </label>

          <label className="block text-sm font-medium text-slate-700">
            End date
            <input
              aria-label="Edit session end date"
              type="date"
              value={endsOn}
              onChange={(event) => setEndsOn(event.target.value)}
              disabled={saving}
              required
              className="mt-1 w-full rounded-lg border border-slate-300 p-2"
            />
          </label>

          {error && (
            <p role="alert" className="text-sm text-red-700">
              {error}
            </p>
          )}

          <div className="flex flex-wrap gap-3">
            <button
              type="submit"
              disabled={saving}
              className="rounded-lg bg-green-700 px-4 py-2 text-sm font-semibold text-white disabled:opacity-50"
            >
              {saving ? "Saving..." : "Save changes"}
            </button>

            <button
              type="button"
              disabled={saving}
              onClick={reset}
              className="rounded-lg border border-slate-300 px-4 py-2 text-sm"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      {notice && (
        <p role="status" className="mt-3 text-sm text-green-800">
          {notice}
        </p>
      )}
    </div>
  );
}