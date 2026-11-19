"use client";

import { useEffect, useRef, useState } from "react";
import GradeLevelArchiveControl from "./grade-level-archive-control";
import GradeLevelEditControl from "./grade-level-edit-control";
import {
  createGradeLevel,
  listGradeLevels,
  type EducationStage,
  type GradeLevel,
} from "@/lib/structure-client";

interface Props {
  schoolSlug: string;
  canManage: boolean;
  onCreated?: () => void;
}

const stages: { value: EducationStage; label: string }[] = [
  { value: "EARLY_YEARS", label: "Early Years" },
  { value: "PRIMARY", label: "Primary" },
  { value: "JUNIOR_SECONDARY", label: "Junior Secondary" },
  { value: "SENIOR_SECONDARY", label: "Senior Secondary" },
];

function sortLevels(values: GradeLevel[]): GradeLevel[] {
  return [...values].sort(
    (a, b) =>
      a.sort_order - b.sort_order ||
      a.name.localeCompare(b.name),
  );
}

export default function GradeLevelsPanel({
  schoolSlug,
  canManage,
  onCreated,
}: Props) {
  const [levels, setLevels] = useState<GradeLevel[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  const [stage, setStage] = useState<EducationStage>("PRIMARY");
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [sortOrder, setSortOrder] = useState("1");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const requestPending = useRef(false);

  useEffect(() => {
    let cancelled = false;

    listGradeLevels(schoolSlug)
      .then((values) => {
        if (cancelled) return;
        setLevels(sortLevels(values));
        setLoadError(null);
        setLoading(false);
      })
      .catch((cause: unknown) => {
        if (cancelled) return;
        setLoadError(
          cause instanceof Error
            ? cause.message
            : "Unable to load grade levels.",
        );
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [schoolSlug, reloadKey]);

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (!canManage || requestPending.current) return;

    const normalizedCode = code.trim().toUpperCase();
    const normalizedName = name.trim();
    const order = Number(sortOrder);

    setFormError(null);
    setNotice(null);

    if (!/^[A-Z0-9][A-Z0-9_-]{0,31}$/.test(normalizedCode)) {
      setFormError("Enter a valid grade level code (up to 32 characters).");
      return;
    }

    if (!normalizedName || normalizedName.length > 100) {
      setFormError("Grade level name must contain 1 to 100 characters.");
      return;
    }

    if (!Number.isInteger(order) || order < 1 || order > 999) {
      setFormError("Display order must be between 1 and 999.");
      return;
    }

    requestPending.current = true;
    setSubmitting(true);

    try {
      const created = await createGradeLevel(schoolSlug, {
        stage,
        code: normalizedCode,
        name: normalizedName,
        sort_order: order,
      });

      if (
        !created.id ||
        created.code !== normalizedCode ||
        created.status !== "ACTIVE"
      ) {
        throw new Error("The server did not confirm grade level creation.");
      }

      setLevels((current) => sortLevels([...current, created]));
      onCreated?.();
      setCode("");
      setName("");
      setNotice("Grade level created successfully.");
    } catch (cause: unknown) {
      setFormError(
        cause instanceof Error
          ? cause.message
          : "Unable to create grade level.",
      );
    } finally {
      requestPending.current = false;
      setSubmitting(false);
    }
  }

  return (
    <section
      aria-label="Grade levels management"
      className="mt-8 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-7"
    >
      <h2 className="text-xl font-semibold text-slate-900">
        Grade levels
      </h2>
      <p className="mt-2 text-sm text-slate-600">
        Configure the educational levels used by this school.
        Grade levels can be reused across academic sessions.
      </p>

      {loading && (
        <p role="status" className="mt-4">
          Loading grade levels...
        </p>
      )}

      {!loading && loadError && (
        <div className="mt-4">
          <p role="alert" className="text-sm text-red-700">
            {loadError}
          </p>
          <button
            type="button"
            onClick={() => {
              setLoading(true);
              setLoadError(null);
              setReloadKey((value) => value + 1);
            }}
            className="mt-3 rounded-lg bg-green-700 px-4 py-2 text-white"
          >
            Retry grade levels
          </button>
        </div>
      )}

      {!loading && !loadError && levels.length === 0 && (
        <p className="mt-4 text-sm text-slate-600">
          No grade levels configured for this school.
        </p>
      )}

      {!loading && !loadError && levels.length > 0 && (
        <ul className="mt-5 space-y-3">
          {levels.map((level) => (
            <li
              key={level.id}
              className="rounded-lg border border-slate-200 p-4"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <strong>{level.name}</strong>
                <span className="text-sm text-slate-600">
                  {level.status}
                </span>
              </div>
              <p className="mt-2 text-sm text-slate-600">
                {level.stage.replaceAll("_", " ")} · {level.code}
                {" · "}Order {level.sort_order}
              </p>
              {canManage && level.status === "ACTIVE" && (
                <div className="mt-3">
                  <GradeLevelEditControl
                    key={`edit-${schoolSlug}-${level.id}`}
                    schoolSlug={schoolSlug}
                    level={level}
                    onUpdated={(updated) => {
                      setLevels((current) =>
                        sortLevels(
                          current.map((item) =>
                            item.id === updated.id ? updated : item,
                          ),
                        ),
                      );
                    }}
                  />
                </div>
              )}
              {canManage && level.status === "ACTIVE" && (
                <GradeLevelArchiveControl
                  key={`${schoolSlug}-${level.id}`}
                  schoolSlug={schoolSlug}
                  level={level}
                  onArchived={(archived) => {
                    setLevels((current) =>
                      sortLevels(
                        current.map((item) =>
                          item.id === archived.id ? archived : item,
                        ),
                      ),
                    );
                  }}
                />
              )}
            </li>
          ))}
        </ul>
      )}

      {canManage && !loading && !loadError && (
        <form
          onSubmit={handleCreate}
          className="mt-6 space-y-4 rounded-xl border border-slate-200 bg-slate-50 p-5"
        >
          <h3 className="font-semibold">Create grade level</h3>

          <div>
            <label htmlFor="grade-stage" className="block text-sm">
              Education stage
            </label>
            <select
              id="grade-stage"
              value={stage}
              disabled={submitting}
              onChange={(event) =>
                setStage(event.target.value as EducationStage)
              }
              className="mt-1 w-full rounded-lg border p-2"
            >
              {stages.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label htmlFor="grade-code" className="block text-sm">
              Grade level code
            </label>
            <input
              id="grade-code"
              value={code}
              onChange={(event) => setCode(event.target.value)}
              placeholder="PRI_3"
              maxLength={32}
              required
              disabled={submitting}
              className="mt-1 w-full rounded-lg border p-2"
            />
          </div>

          <div>
            <label htmlFor="grade-name" className="block text-sm">
              Grade level name
            </label>
            <input
              id="grade-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="Primary 3"
              maxLength={100}
              required
              disabled={submitting}
              className="mt-1 w-full rounded-lg border p-2"
            />
          </div>

          <div>
            <label htmlFor="grade-order" className="block text-sm">
              Display order
            </label>
            <input
              id="grade-order"
              type="number"
              min={1}
              max={999}
              step={1}
              value={sortOrder}
              onChange={(event) => setSortOrder(event.target.value)}
              required
              disabled={submitting}
              className="mt-1 w-full rounded-lg border p-2"
            />
          </div>

          <button
            type="submit"
            disabled={submitting}
            className="min-h-11 rounded-lg bg-green-700 px-5 py-2 font-semibold text-white disabled:opacity-50"
          >
            {submitting ? "Creating grade level..." : "Create grade level"}
          </button>

          {formError && (
            <p role="alert" className="text-sm text-red-700">
              {formError}
            </p>
          )}

          {notice && (
            <p role="status" className="text-sm text-green-800">
              {notice}
            </p>
          )}
        </form>
      )}
    </section>
  );
}