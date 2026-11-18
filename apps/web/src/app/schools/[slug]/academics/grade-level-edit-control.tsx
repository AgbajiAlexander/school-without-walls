"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  updateGradeLevel,
  type EducationStage,
  type GradeLevel,
} from "@/lib/structure-client";

interface Props {
  schoolSlug: string;
  level: GradeLevel;
  onUpdated: (level: GradeLevel) => void;
}

const stages: { value: EducationStage; label: string }[] = [
  { value: "EARLY_YEARS", label: "Early Years" },
  { value: "PRIMARY", label: "Primary" },
  { value: "JUNIOR_SECONDARY", label: "Junior Secondary" },
  { value: "SENIOR_SECONDARY", label: "Senior Secondary" },
];

export default function GradeLevelEditControl({
  schoolSlug,
  level,
  onUpdated,
}: Props) {
  const [editing, setEditing] = useState(false);
  const [stage, setStage] = useState<EducationStage>(level.stage);
  const [code, setCode] = useState(level.code);
  const [name, setName] = useState(level.name);
  const [sortOrder, setSortOrder] = useState(String(level.sort_order));
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

  function closeEditor() {
    if (pendingRef.current) return;

    setStage(level.stage);
    setCode(level.code);
    setName(level.name);
    setSortOrder(String(level.sort_order));
    setError(null);
    setEditing(false);
  }

  async function handleSave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (pendingRef.current || level.status !== "ACTIVE") return;

    const normalizedCode = code.trim().toUpperCase();
    const normalizedName = name.trim();
    const order = Number(sortOrder);

    setError(null);

    if (!/^[A-Z0-9][A-Z0-9_-]{0,31}$/.test(normalizedCode)) {
      setError("Enter a valid grade level code.");
      return;
    }

    if (
      !normalizedName ||
      new TextEncoder().encode(normalizedName).length > 100
    ) {
      setError("Grade level name must contain 1 to 100 UTF-8 bytes.");
      return;
    }

    if (!Number.isInteger(order) || order < 1 || order > 999) {
      setError("Display order must be between 1 and 999.");
      return;
    }

    pendingRef.current = true;
    setPending(true);

    try {
      const updated = await updateGradeLevel(schoolSlug, level.id, {
        stage,
        code: normalizedCode,
        name: normalizedName,
        sort_order: order,
      });

      if (!mountedRef.current) return;

      if (
        updated.id !== level.id ||
        updated.stage !== stage ||
        updated.code !== normalizedCode ||
        updated.name !== normalizedName ||
        updated.sort_order !== order ||
        updated.status !== "ACTIVE"
      ) {
        throw new Error("The server did not confirm the grade level update.");
      }

      onUpdated(updated);
      setEditing(false);
    } catch (cause: unknown) {
      if (!mountedRef.current) return;

      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to update grade level.",
      );
    } finally {
      pendingRef.current = false;

      if (mountedRef.current) {
        setPending(false);
      }
    }
  }

  if (level.status !== "ACTIVE") return null;

  if (!editing) {
    return (
      <button
        type="button"
        onClick={() => setEditing(true)}
        className="rounded-lg border border-slate-300 px-3 py-2 text-sm font-medium"
      >
        Edit {level.name}
      </button>
    );
  }

  return (
    <form
      onSubmit={handleSave}
      className="rounded-lg border border-slate-200 bg-slate-50 p-4"
    >
      <h4 className="font-semibold">
        Edit {level.name}
      </h4>

      <label className="mt-3 block text-sm">
        Edit education stage
        <select
          value={stage}
          disabled={pending}
          onChange={(event) =>
            setStage(event.target.value as EducationStage)
          }
          className="mt-1 block w-full rounded-lg border p-2"
        >
          {stages.map((item) => (
            <option key={item.value} value={item.value}>
              {item.label}
            </option>
          ))}
        </select>
      </label>

      <label className="mt-3 block text-sm">
        Edit grade level code
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
        Edit grade level name
        <input
          value={name}
          onChange={(event) => setName(event.target.value)}
          maxLength={100}
          required
          disabled={pending}
          className="mt-1 block w-full rounded-lg border p-2"
        />
      </label>

      <label className="mt-3 block text-sm">
        Edit display order
        <input
          type="number"
          min={1}
          max={999}
          step={1}
          value={sortOrder}
          onChange={(event) => setSortOrder(event.target.value)}
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
          {pending ? "Saving..." : "Save grade level"}
        </button>

        <button
          type="button"
          disabled={pending}
          onClick={closeEditor}
          className="rounded-lg border border-slate-300 px-4 py-2 text-sm"
        >
          Cancel editing
        </button>
      </div>
    </form>
  );
}