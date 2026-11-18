"use client";

import { useState, type FormEvent } from "react";
import {
  createAcademicSession,
  type AcademicSession,
} from "@/lib/academics-client";

interface Props {
  schoolSlug: string;
  onCreated: (session: AcademicSession) => void;
}

export default function SessionCreateForm({
  schoolSlug,
  onCreated,
}: Props) {
  const [name, setName] = useState("");
  const [startsOn, setStartsOn] = useState("");
  const [endsOn, setEndsOn] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (submitting) return;

    setError(null);

    const trimmedName = name.trim();

    if (!trimmedName || !startsOn || !endsOn) {
      setError("Complete all academic session fields.");
      return;
    }

    if (endsOn <= startsOn) {
      setError("The end date must be after the start date.");
      return;
    }

    setSubmitting(true);

    try {
      const session = await createAcademicSession(schoolSlug, {
        name: trimmedName,
        starts_on: startsOn,
        ends_on: endsOn,
      });

      setName("");
      setStartsOn("");
      setEndsOn("");

      onCreated(session);
    } catch (cause: unknown) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to create the academic session.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="mt-6 space-y-4 rounded-xl border border-slate-200 bg-slate-50 p-5"
    >
      <h3 className="text-lg font-semibold text-slate-900">
        Create academic session
      </h3>

      <p className="text-sm text-slate-600">
        New sessions begin in draft status. Configure three valid
        terms before activation.
      </p>

      <div>
        <label htmlFor="session-name" className="block text-sm font-medium">
          Session name
        </label>
        <input
          id="session-name"
          required
          maxLength={32}
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder="2026/2027"
          className="mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2"
        />
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <label htmlFor="session-start" className="block text-sm font-medium">
            Start date
          </label>
          <input
            id="session-start"
            type="date"
            required
            value={startsOn}
            onChange={(event) => setStartsOn(event.target.value)}
            className="mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2"
          />
        </div>

        <div>
          <label htmlFor="session-end" className="block text-sm font-medium">
            End date
          </label>
          <input
            id="session-end"
            type="date"
            required
            value={endsOn}
            onChange={(event) => setEndsOn(event.target.value)}
            className="mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2"
          />
        </div>
      </div>

      {error && (
        <p role="alert" className="text-sm text-red-700">
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={submitting}
        className="min-h-11 rounded-lg bg-green-700 px-5 py-2 text-sm font-semibold text-white disabled:opacity-60"
      >
        {submitting ? "Creating session..." : "Create draft session"}
      </button>
    </form>
  );
}