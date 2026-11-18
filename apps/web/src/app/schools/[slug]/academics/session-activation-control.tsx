"use client";

import { useState } from "react";
import {
  activateAcademicSession,
  type AcademicSession,
  type AcademicTerm,
} from "@/lib/academics-client";

interface Props {
  schoolSlug: string;
  session: AcademicSession;
  terms: AcademicTerm[];
  onActivated: (session: AcademicSession) => void;
}

export default function SessionActivationControl({
  schoolSlug,
  session,
  terms,
  onActivated,
}: Props) {
  const [confirmed, setConfirmed] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const hasThreeTerms =
    terms.length === 3 &&
    [1, 2, 3].every((sequence) =>
      terms.some(
        (term) =>
          term.sequence === sequence &&
          term.session_id === session.id,
      ),
    );

  if (session.status !== "DRAFT") {
    return null;
  }

  async function handleActivate() {
    if (submitting || !confirmed || !hasThreeTerms) {
      return;
    }

    setError(null);
    setSubmitting(true);

    try {
      const updated = await activateAcademicSession(
        schoolSlug,
        session.id,
      );

      if (updated.id !== session.id || updated.status !== "ACTIVE") {
        throw new Error(
          "The server did not confirm session activation.",
        );
      }

      onActivated(updated);
    } catch (cause: unknown) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to activate the academic session.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <section
      aria-label="Academic session activation"
      className="mt-5 rounded-xl border border-amber-200 bg-amber-50 p-4"
    >
      <h4 className="font-semibold text-amber-950">
        Activate academic session
      </h4>

      <p className="mt-2 text-sm text-amber-900">
        Selected session: <strong>{session.name}</strong>
      </p>

      {!hasThreeTerms ? (
        <p role="status" className="mt-3 text-sm text-amber-900">
          Configure all three academic terms before activation.
        </p>
      ) : (
        <>
          <p className="mt-3 text-sm text-amber-900">
            Activation makes this academic session active.
            The school calendar and any existing active
            session are subject to backend validation.
          </p>

          <label className="mt-4 flex items-start gap-3 text-sm text-amber-950">
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
              I confirm that I want to activate the academic
              session {session.name}.
            </span>
          </label>

          <button
            type="button"
            onClick={handleActivate}
            disabled={!confirmed || submitting}
            className="mt-4 min-h-11 rounded-lg bg-green-700 px-5 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-50"
          >
            {submitting
              ? "Activating session..."
              : "Activate academic session"}
          </button>
        </>
      )}

      {error && (
        <p role="alert" className="mt-3 text-sm text-red-700">
          {error}
        </p>
      )}
    </section>
  );
}