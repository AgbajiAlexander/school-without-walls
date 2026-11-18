"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  updateClassArm,
  type ClassArm,
} from "@/lib/structure-client";

interface Props {
  schoolSlug: string;
  arm: ClassArm;
  onUpdated: (updated: ClassArm) => void;
}

export default function ClassArmEditControl({
  schoolSlug,
  arm,
  onUpdated,
}: Props) {
  const [editing, setEditing] = useState(false);
  const [code, setCode] = useState(arm.code);
  const [name, setName] = useState(arm.name);
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

  function openEditing() {
    if (pendingRef.current) return;
    setCode(arm.code);
    setName(arm.name);
    setError(null);
    setEditing(true);
  }

  function cancelEditing() {
    if (pendingRef.current) return;
    setEditing(false);
    setError(null);
  }

  async function handleSave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (pendingRef.current || arm.status !== "ACTIVE") return;

    const normalizedCode = code.trim().toUpperCase();
    const normalizedName = name.trim();

    setError(null);

    if (!/^[A-Z0-9][A-Z0-9_-]{0,31}$/.test(normalizedCode)) {
      setError("Enter a valid class arm code.");
      return;
    }

    if (
      !normalizedName ||
      new TextEncoder().encode(normalizedName).length > 100
    ) {
      setError("Class arm name must contain 1 to 100 UTF-8 bytes.");
      return;
    }

    pendingRef.current = true;
    setPending(true);

    try {
      const updated = await updateClassArm(
        schoolSlug,
        arm.id,
        {
          code: normalizedCode,
          name: normalizedName,
        },
      );

      if (!mountedRef.current) return;

      if (
        updated.id !== arm.id ||
        updated.class_id !== arm.class_id ||
        updated.code !== normalizedCode ||
        updated.name !== normalizedName ||
        updated.status !== "ACTIVE"
      ) {
        throw new Error(
          "The server did not confirm the class arm update.",
        );
      }

      onUpdated(updated);
      setEditing(false);
    } catch (cause: unknown) {
      if (!mountedRef.current) return;

      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to update class arm.",
      );
    } finally {
      pendingRef.current = false;
      if (mountedRef.current) setPending(false);
    }
  }

  if (arm.status !== "ACTIVE") return null;

  if (!editing) {
    return (
      <button
        type="button"
        onClick={openEditing}
        className="mt-3 rounded-lg border border-slate-300 px-3 py-2 text-sm"
      >
        Edit arm {arm.name}
      </button>
    );
  }

  return (
    <form
      onSubmit={handleSave}
      className="mt-3 space-y-3 rounded-lg border border-slate-200 bg-slate-50 p-4"
    >
      <h4 className="font-semibold">
        Edit arm {arm.name}
      </h4>

      <label className="block text-sm">
        Edit arm code
        <input
          value={code}
          onChange={(event) => setCode(event.target.value)}
          required
          maxLength={32}
          disabled={pending}
          className="mt-1 block w-full rounded-lg border p-2"
        />
      </label>

      <label className="block text-sm">
        Edit arm name
        <input
          value={name}
          onChange={(event) => setName(event.target.value)}
          required
          maxLength={100}
          disabled={pending}
          className="mt-1 block w-full rounded-lg border p-2"
        />
      </label>

      {error && (
        <p role="alert" className="text-sm text-red-700">
          {error}
        </p>
      )}

      <div className="flex flex-wrap gap-2">
        <button
          type="submit"
          disabled={pending}
          className="rounded-lg bg-green-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
        >
          {pending ? "Saving arm..." : "Save arm"}
        </button>

        <button
          type="button"
          onClick={cancelEditing}
          disabled={pending}
          className="rounded-lg border border-slate-300 px-4 py-2 text-sm"
        >
          Cancel arm editing
        </button>
      </div>
    </form>
  );
}