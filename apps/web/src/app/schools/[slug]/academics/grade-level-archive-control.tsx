"use client";

import { useEffect, useRef, useState } from "react";
import {
  archiveGradeLevel,
  type GradeLevel,
} from "@/lib/structure-client";

interface Props {
  schoolSlug: string;
  level: GradeLevel;
  onArchived: (level: GradeLevel) => void;
}

export default function GradeLevelArchiveControl({
  schoolSlug,
  level,
  onArchived,
}: Props) {
  const [confirming, setConfirming] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pendingRef = useRef(false);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;

    return () => {
      mountedRef.current = false;
    };
  }, []);

  async function handleArchive() {
    if (pendingRef.current || level.status !== "ACTIVE") return;

    pendingRef.current = true;
    setPending(true);
    setError(null);

    try {
      const result = await archiveGradeLevel(schoolSlug, level.id);

      if (!mountedRef.current) return;

      if (result.id !== level.id || result.status !== "ARCHIVED") {
        throw new Error("The server did not confirm grade level archiving.");
      }

      onArchived(result);
      setConfirming(false);
    } catch (cause: unknown) {
      if (!mountedRef.current) return;

      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to archive grade level.",
      );
    } finally {
      pendingRef.current = false;

      if (mountedRef.current) {
        setPending(false);
      }
    }
  }

  if (level.status !== "ACTIVE") return null;

  return (
    <div className="mt-3">
      {!confirming ? (
        <button
          type="button"
          onClick={() => {
            setError(null);
            setConfirming(true);
          }}
          className="rounded-lg border border-red-300 px-3 py-2 text-sm font-medium text-red-700"
        >
          Archive {level.name}
        </button>
      ) : (
        <div className="rounded-lg border border-red-200 bg-red-50 p-4">
          <p className="text-sm text-red-900">
            Archive {level.name}? This action may affect future
            class configuration.
          </p>

          <div className="mt-3 flex flex-wrap gap-2">
            <button
              type="button"
              disabled={pending}
              onClick={handleArchive}
              className="rounded-lg bg-red-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
            >
              {pending ? "Archiving..." : "Confirm archive"}
            </button>

            <button
              type="button"
              disabled={pending}
              onClick={() => {
                setConfirming(false);
                setError(null);
              }}
              className="rounded-lg border border-slate-300 bg-white px-4 py-2 text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {error && (
        <p role="alert" className="mt-2 text-sm text-red-700">
          {error}
        </p>
      )}
    </div>
  );
}