"use client";

import { useEffect, useRef, useState } from "react";
import {
  archiveClassArm,
  type ClassArm,
} from "@/lib/structure-client";

interface Props {
  schoolSlug: string;
  arm: ClassArm;
  onArchived: (archived: ClassArm) => void;
}

export default function ClassArmArchiveControl({
  schoolSlug,
  arm,
  onArchived,
}: Props) {
  const [confirming, setConfirming] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const mountedRef = useRef(false);
  const pendingRef = useRef(false);

  useEffect(() => {
    mountedRef.current = true;

    return () => {
      mountedRef.current = false;
    };
  }, []);

  async function handleArchive() {
    if (
      !confirming ||
      pendingRef.current ||
      arm.status !== "ACTIVE"
    ) {
      return;
    }

    pendingRef.current = true;
    setPending(true);
    setError(null);

    try {
      const archived = await archiveClassArm(
        schoolSlug,
        arm.id,
      );

      if (!mountedRef.current) return;

      if (
        archived.id !== arm.id ||
        archived.class_id !== arm.class_id ||
        archived.code !== arm.code ||
        archived.name !== arm.name ||
        archived.status !== "ARCHIVED"
      ) {
        throw new Error(
          "The server did not confirm class arm archiving.",
        );
      }

      onArchived(archived);
      setConfirming(false);
    } catch (cause: unknown) {
      if (!mountedRef.current) return;

      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to archive class arm.",
      );
    } finally {
      pendingRef.current = false;

      if (mountedRef.current) {
        setPending(false);
      }
    }
  }

  if (arm.status !== "ACTIVE") return null;

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
          Archive arm {arm.name}
        </button>
      ) : (
        <div className="rounded-lg border border-red-200 bg-red-50 p-4">
          <p className="text-sm text-red-900">
            Archive arm {arm.name}? The arm will become
            inactive. Existing academic records will not
            be deleted by this action.
          </p>

          <div className="mt-3 flex flex-wrap gap-2">
            <button
              type="button"
              disabled={pending}
              onClick={handleArchive}
              className="rounded-lg bg-red-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
            >
              {pending
                ? "Archiving arm..."
                : "Confirm arm archive"}
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
              Cancel arm archive
            </button>
          </div>
        </div>
      )}

      {error && (
        <p role="alert" className="mt-3 text-sm text-red-700">
          {error}
        </p>
      )}
    </div>
  );
}