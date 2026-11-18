"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  updateClass,
  type SchoolClass,
} from "@/lib/structure-client";

interface Props {
  schoolSlug: string;
  schoolClass: SchoolClass;
  onUpdated: (updated: SchoolClass) => void;
}

export default function ClassEditControl({
  schoolSlug,
  schoolClass,
  onUpdated,
}: Props) {
  const [editing, setEditing] = useState(false);
  const [code, setCode] = useState(schoolClass.code);
  const [name, setName] = useState(schoolClass.name);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const pendingRef = useRef(false);
  const mountedRef = useRef(false);

  useEffect(() => {
    mountedRef.current = true;

    return () => {
      mountedRef.current = false;
    };
  }, []);

  function cancelEditing() {
    if (pendingRef.current) return;

    setCode(schoolClass.code);
    setName(schoolClass.name);
    setError(null);
    setEditing(false);
  }

  async function handleSave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (pendingRef.current || schoolClass.status !== "ACTIVE") return;

    const normalizedCode = code.trim().toUpperCase();
    const normalizedName = name.trim();

    setError(null);

    if (!/^[A-Z0-9][A-Z0-9_-]{0,31}$/.test(normalizedCode)) {
      setError("Enter a valid class code.");
      return;
    }

    if (
      !normalizedName ||
      new TextEncoder().encode(normalizedName).length > 100
    ) {
      setError("Class name must contain 1 to 100 UTF-8 bytes.");
      return;
    }

    pendingRef.current = true;
    setPending(true);

    try {
      const updated = await updateClass(
        schoolSlug,
        schoolClass.id,
        {
          code: normalizedCode,
          name: normalizedName,
        },
      );

      if (!mountedRef.current) return;

      if (
        updated.id !== schoolClass.id ||
        updated.session_id !== schoolClass.session_id ||
        updated.grade_level_id !== schoolClass.grade_level_id ||
        updated.code !== normalizedCode ||
        updated.name !== normalizedName ||
        updated.status !== "ACTIVE"
      ) {
        throw new Error("The server did not confirm the class update.");
      }

      onUpdated(updated);
      setEditing(false);
    } catch (cause: unknown) {
      if (!mountedRef.current) return;

      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to update class.",
      );
    } finally {
      pendingRef.current = false;

      if (mountedRef.current) {
        setPending(false);
      }
    }
  }

  if (schoolClass.status !== "ACTIVE") return null;

  if (!editing) {
    return (
      <button
        type="button"
        onClick={() => setEditing(true)}
        className="mt-3 rounded-lg border border-slate-300 px-3 py-2 text-sm"
      >
        Edit class {schoolClass.name}
      </button>
    );
  }

  return (
    <form
      onSubmit={handleSave}
      className="mt-3 rounded-lg border border-slate-200 bg-slate-50 p-4"
    >
      <h4 className="font-semibold">
        Edit class {schoolClass.name}
      </h4>

      <label className="mt-3 block text-sm">
        Edit class code
        <input
          value={code}
          onChange={(event) => setCode(event.target.value)}
          maxLength={32}
          required
          disabled={pending}
          className="mt-1 block w-full rounded-lg border p-2"
        />
      </label>

      <label className="mt-3 block text-sm">
        Edit class name
        <input
          value={name}
          onChange={(event) => setName(event.target.value)}
          maxLength={100}
          required
          disabled={pending}
          className="mt-1 block w-full rounded-lg border p-2"
        />
      </label>

      {error && (
        <p role="alert" className="mt-3 text-sm text-red-700">
          {error}
        </p>
      )}

      <div className="mt-4 flex flex-wrap gap-2">
        <button
          type="submit"
          disabled={pending}
          className="rounded-lg bg-green-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
        >
          {pending ? "Saving class..." : "Save class"}
        </button>

        <button
          type="button"
          onClick={cancelEditing}
          disabled={pending}
          className="rounded-lg border border-slate-300 px-4 py-2 text-sm"
        >
          Cancel class editing
        </button>
      </div>
    </form>
  );
}