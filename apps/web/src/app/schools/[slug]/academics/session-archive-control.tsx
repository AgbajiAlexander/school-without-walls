"use client";

import { useRef, useState } from "react";
import {
  archiveAcademicSession,
  type AcademicSession,
} from "@/lib/academics-client";

interface Props {
  schoolSlug: string;
  session: AcademicSession;
  onArchived: (session: AcademicSession) => void;
}

export default function SessionArchiveControl({
  schoolSlug,
  session,
  onArchived,
}: Props) {
  const [confirmed, setConfirmed] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestPending = useRef(false);

  async function handleArchive() {
    if (
      session.status === "ARCHIVED" ||
      !confirmed ||
      requestPending.current
    ) {
      return;
    }

    requestPending.current = true;
    setSubmitting(true);
    setError(null);

    try {
      const updated = await archiveAcademicSession(
        schoolSlug,
        session.id,
      );

      if (
        updated.id !== session.id ||
        updated.status !== "ARCHIVED"
      ) {
        throw new Error(
          "The server did not confirm session archiving.",
        );
      }

      onArchived(updated);
    } catch (cause: unknown) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to archive the academic session.",
      );
    } finally {
      requestPending.current = false;
      setSubmitting(false);
    }
  }

  if (session.status === "ARCHIVED") {
    return null;
  }

  return (
    <section
      aria-label="Academic session archiving"
      className="mt-5 rounded-xl border border-red-200 bg-red-50 p-4"
    >
      <h4 className="font-semibold text-red-950">
        Archive academic session
      </h4>

      <p className="mt-2 text-sm text-red-900">
        Selected session: <strong>{session.name}</strong>
      </p>

      <p className="mt-3 text-sm text-red-900">
        Archiving makes this session read-only. You will not
        be able to reactivate or edit it. Confirm the school
        calendar and any consequences for active academic
        activities before proceeding.
      </p>

      <label className="mt-4 flex items-start gap-3 text-sm text-red-950">
        <input
          type="checkbox"
          checked={confirmed}
          disabled={submitting}
          onChange={(event) =>
            setConfirmed(event.target.checked)
          }
          className="mt-1 h-4 w-4"
        />
        <span>
          I understand that archiving academic session{" "}
          <strong>{session.name}</strong> makes it read-only.
        </span>
      </label>

      <button
        type="button"
        onClick={handleArchive}
        disabled={!confirmed || submitting}
        className="mt-4 min-h-11 rounded-lg bg-red-700 px-5 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-50"
      >
        {submitting
          ? "Archiving session..."
          : "Archive academic session"}
      </button>

      {error && (
        <p role="alert" className="mt-3 text-sm text-red-700">
          {error}
        </p>
      )}
    </section>
  );
}