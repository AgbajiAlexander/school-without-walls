"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import ClassArmEditControl from "./class-arm-edit-control";
import ClassArmArchiveControl from "./class-arm-archive-control";
import {
  createClassArm,
  listClassArms,
  type ClassArm,
  type SchoolClass,
} from "@/lib/structure-client";

interface Props {
  schoolSlug: string;
  schoolClass: SchoolClass;
  canManage: boolean;
}

export default function ClassArmsPanel({
  schoolSlug,
  schoolClass,
  canManage,
}: Props) {
  const [arms, setArms] = useState<ClassArm[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [retryKey, setRetryKey] = useState(0);

  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [pending, setPending] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const mountedRef = useRef(false);
  const pendingRef = useRef(false);
  const contextKey = `${schoolSlug}:${schoolClass.id}`;
  const contextRef = useRef(contextKey);

  contextRef.current = contextKey;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    setLoading(true);
    setArms([]);
    setError(null);
    setNotice(null);
    setFormError(null);

    listClassArms(schoolSlug, schoolClass.id)
      .then((values) => {
        if (cancelled) return;
        setArms(values);
        setLoading(false);
      })
      .catch((cause: unknown) => {
        if (cancelled) return;

        setError(
          cause instanceof Error
            ? cause.message
            : "Unable to load class arms.",
        );
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [schoolSlug, schoolClass.id, retryKey]);

  async function handleCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (
      !canManage ||
      schoolClass.status !== "ACTIVE" ||
      loading ||
      error ||
      pendingRef.current
    ) {
      return;
    }

    const normalizedCode = code.trim().toUpperCase();
    const normalizedName = name.trim();

    setFormError(null);
    setNotice(null);

    if (!/^[A-Z0-9][A-Z0-9_-]{0,31}$/.test(normalizedCode)) {
      setFormError("Enter a valid class arm code.");
      return;
    }

    if (
      !normalizedName ||
      new TextEncoder().encode(normalizedName).length > 100
    ) {
      setFormError("Class arm name must contain 1 to 100 UTF-8 bytes.");
      return;
    }

    const requestContext = contextKey;
    const parentClassID = schoolClass.id;

    pendingRef.current = true;
    setPending(true);

    try {
      const created = await createClassArm(schoolSlug, {
        class_id: parentClassID,
        code: normalizedCode,
        name: normalizedName,
      });

      if (!mountedRef.current ||
          contextRef.current !== requestContext) return;

      if (
        !created.id ||
        created.class_id !== parentClassID ||
        created.code !== normalizedCode ||
        created.name !== normalizedName ||
        created.status !== "ACTIVE"
      ) {
        throw new Error("The server did not confirm class arm creation.");
      }

      setArms((current) =>
        [...current, created].sort((a, b) =>
          a.code.localeCompare(b.code),
        ),
      );
      setCode("");
      setName("");
      setNotice("Class arm created successfully.");
    } catch (cause: unknown) {
      if (!mountedRef.current ||
          contextRef.current !== requestContext) return;

      setFormError(
        cause instanceof Error
          ? cause.message
          : "Unable to create class arm.",
      );
    } finally {
      pendingRef.current = false;
      if (mountedRef.current) setPending(false);
    }
  }

  return (
    <section
      aria-label={`Class arms for ${schoolClass.name}`}
      className="mt-4 rounded-xl border border-slate-200 bg-white p-4"
    >
      <h3 className="text-lg font-semibold text-slate-900">
        Class Arms — {schoolClass.name}
      </h3>

      {loading && (
        <p role="status" className="mt-3 text-sm">
          Loading class arms...
        </p>
      )}

      {!loading && error && (
        <div className="mt-3">
          <p role="alert" className="text-sm text-red-700">
            {error}
          </p>
          <button
            type="button"
            onClick={() => setRetryKey((current) => current + 1)}
            className="mt-2 rounded-lg border px-3 py-2 text-sm"
          >
            Retry class arms
          </button>
        </div>
      )}

      {!loading && !error && arms.length === 0 && (
        <p className="mt-3 text-sm text-slate-600">
          No arms configured for this class.
        </p>
      )}

      {!loading && !error && arms.length > 0 && (
        <ul className="mt-3 space-y-2">
          {arms.map((arm) => (
            <li
              key={arm.id}
              className="rounded-lg border border-slate-200 p-3"
            >
              <p className="font-medium">{arm.name}</p>
              <p className="text-sm text-slate-600">
                Code: {arm.code} · Status: {arm.status}
              </p>
              {canManage &&
                schoolClass.status === "ACTIVE" &&
                arm.status === "ACTIVE" && (
                  <ClassArmEditControl
                    key={`edit-${schoolSlug}-${schoolClass.id}-${arm.id}`}
                    schoolSlug={schoolSlug}
                    arm={arm}
                    onUpdated={(updated) => {
                      if (
                        contextRef.current !== `${schoolSlug}:${schoolClass.id}` ||
                        updated.class_id !== schoolClass.id
                      ) {
                        return;
                      }

                      setArms((current) =>
                        current
                          .map((item) =>
                            item.id === updated.id ? updated : item,
                          )
                          .sort((a, b) => a.code.localeCompare(b.code)),
                      );
                    }}
                  />
                )}
              {canManage &&
                schoolClass.status === "ACTIVE" &&
                arm.status === "ACTIVE" && (
                  <ClassArmArchiveControl
                    key={`archive-${schoolSlug}-${schoolClass.id}-${arm.id}`}
                    schoolSlug={schoolSlug}
                    arm={arm}
                    onArchived={(archived) => {
                      if (
                        contextRef.current !== `${schoolSlug}:${schoolClass.id}` ||
                        archived.class_id !== schoolClass.id
                      ) {
                        return;
                      }

                      setArms((current) =>
                        current.map((item) =>
                          item.id === archived.id ? archived : item,
                        ),
                      );
                    }}
                  />
                )}
            </li>
          ))}
        </ul>
      )}

      {canManage &&
        schoolClass.status === "ACTIVE" &&
        !loading &&
        !error && (
          <form
            onSubmit={handleCreate}
            className="mt-5 space-y-3 rounded-lg bg-slate-50 p-4"
          >
            <h4 className="font-semibold">Create class arm</h4>

            <label className="block text-sm">
              Arm code
              <input
                value={code}
                onChange={(event) => setCode(event.target.value)}
                maxLength={32}
                required
                disabled={pending}
                placeholder="A"
                className="mt-1 block w-full rounded-lg border p-2"
              />
            </label>

            <label className="block text-sm">
              Arm name
              <input
                value={name}
                onChange={(event) => setName(event.target.value)}
                maxLength={100}
                required
                disabled={pending}
                placeholder="Arm A"
                className="mt-1 block w-full rounded-lg border p-2"
              />
            </label>

            <button
              type="submit"
              disabled={pending}
              className="rounded-lg bg-green-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
            >
              {pending ? "Creating arm..." : "Create class arm"}
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