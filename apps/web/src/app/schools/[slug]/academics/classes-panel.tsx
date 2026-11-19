"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import ClassEditControl from "./class-edit-control";
import ClassArchiveControl from "./class-archive-control";
import ClassArmsPanel from "./class-arms-panel";
import {
  listAcademicSessions,
  type AcademicSession,
} from "@/lib/academics-client";
import {
  createClass,
  listClasses,
  listGradeLevels,
  type GradeLevel,
  type SchoolClass,
} from "@/lib/structure-client";

interface Props {
  schoolSlug: string;
  canManage: boolean;
  gradeLevelsVersion?: number;
}

export default function ClassesPanel({
  schoolSlug,
  canManage,
  gradeLevelsVersion = 0,
}: Props) {
  const [sessions, setSessions] = useState<AcademicSession[]>([]);
  const [levels, setLevels] = useState<GradeLevel[]>([]);
  const [selectedSession, setSelectedSession] = useState("");
  const [classes, setClasses] = useState<SchoolClass[]>([]);
  const [expandedClassID, setExpandedClassID] = useState<string | null>(null);

  const [initialLoading, setInitialLoading] = useState(true);
  const [initialError, setInitialError] = useState<string | null>(null);
  const [classesLoading, setClassesLoading] = useState(false);
  const [classesError, setClassesError] = useState<string | null>(null);

  const [gradeLevelID, setGradeLevelID] = useState("");
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  const pendingRef = useRef(false);
  const selectedRef = useRef(selectedSession);
  const schoolRef = useRef(schoolSlug);
  const mountedRef = useRef(false);

  selectedRef.current = selectedSession;
  schoolRef.current = schoolSlug;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    Promise.all([
      listAcademicSessions(schoolSlug),
      listGradeLevels(schoolSlug),
    ])
      .then(([schoolSessions, schoolLevels]) => {
        if (cancelled) return;

        setSessions(schoolSessions);
        setLevels(schoolLevels);

        setSelectedSession((previous) =>
          schoolSessions.some((session) => session.id === previous)
            ? previous
            : schoolSessions.find((session) => session.status === "ACTIVE")?.id ??
              schoolSessions[0]?.id ??
              "",
        );

        setInitialError(null);
        setInitialLoading(false);
      })
      .catch((cause: unknown) => {
        if (cancelled) return;

        setInitialError(
          cause instanceof Error
            ? cause.message
            : "Unable to load class configuration.",
        );
        setInitialLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [schoolSlug, reloadKey, gradeLevelsVersion]);

  useEffect(() => {
    if (!selectedSession || initialLoading || initialError) {
      return;
    }

    let cancelled = false;

    setClasses([]);
    setClassesError(null);
    setClassesLoading(true);

    listClasses(schoolSlug, selectedSession)
      .then((values) => {
        if (cancelled) return;
        setClasses(values);
        setClassesLoading(false);
      })
      .catch((cause: unknown) => {
        if (cancelled) return;

        setClassesError(
          cause instanceof Error
            ? cause.message
            : "Unable to load classes.",
        );
        setClassesLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [schoolSlug, selectedSession, initialLoading, initialError, reloadKey]);

  async function handleCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (!canManage || pendingRef.current || !selectedSession) return;

    const normalizedCode = code.trim().toUpperCase();
    const normalizedName = name.trim();
    const activeLevel = levels.find(
      (level) => level.id === gradeLevelID && level.status === "ACTIVE",
    );

    setFormError(null);
    setNotice(null);

    if (!activeLevel) {
      setFormError("Select an active grade level.");
      return;
    }

    if (!/^[A-Z0-9][A-Z0-9_-]{0,31}$/.test(normalizedCode)) {
      setFormError("Enter a valid class code.");
      return;
    }

    if (
      !normalizedName ||
      new TextEncoder().encode(normalizedName).length > 100
    ) {
      setFormError("Class name must contain 1 to 100 UTF-8 bytes.");
      return;
    }

    const sessionID = selectedSession;
    const requestSchool = schoolSlug;

    pendingRef.current = true;
    setSubmitting(true);

    try {
      const created = await createClass(schoolSlug, {
        session_id: sessionID,
        grade_level_id: activeLevel.id,
        code: normalizedCode,
        name: normalizedName,
      });

      if (!mountedRef.current) return;

      if (
        !created.id ||
        created.session_id !== sessionID ||
        created.grade_level_id !== activeLevel.id ||
        created.code !== normalizedCode
      ) {
        throw new Error("The server did not confirm class creation.");
      }

      if (
        schoolRef.current === requestSchool &&
        selectedRef.current === sessionID
      ) {
        setClasses((current) => [...current, created]);
        setCode("");
        setName("");
        setNotice("Class created successfully.");
      }
    } catch (cause: unknown) {
      if (!mountedRef.current) return;

      if (
        schoolRef.current === requestSchool &&
        selectedRef.current === sessionID
      ) {
        setFormError(
          cause instanceof Error
            ? cause.message
            : "Unable to create class.",
        );
      }
    } finally {
      pendingRef.current = false;
      if (mountedRef.current) setSubmitting(false);
    }
  }

  const activeLevels = levels
    .filter((level) => level.status === "ACTIVE")
    .sort((a, b) => a.sort_order - b.sort_order);

  return (
    <section
      aria-label="Classes management"
      className="mt-8 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-7"
    >
      <h2 className="text-xl font-semibold text-slate-900">
        Classes
      </h2>
      <p className="mt-2 text-sm text-slate-600">
        Manage classes for each academic session using the
        grade levels configured for this school.
      </p>

      {initialLoading && (
        <p role="status" className="mt-4">
          Loading class configuration...
        </p>
      )}

      {!initialLoading && initialError && (
        <div className="mt-4">
          <p role="alert">{initialError}</p>
          <button
            type="button"
            onClick={() => {
              setInitialLoading(true);
              setReloadKey((value) => value + 1);
            }}
            className="mt-2 rounded-lg border px-4 py-2"
          >
            Retry class configuration
          </button>
        </div>
      )}

      {!initialLoading && !initialError && (
        <>
          {sessions.length === 0 ? (
            <p className="mt-4 text-sm text-slate-600">
              Create an academic session before configuring classes.
            </p>
          ) : (
            <>
              <label htmlFor="class-session" className="mt-5 block text-sm">
                Academic session
              </label>
              <select
                id="class-session"
                value={selectedSession}
                onChange={(event) => {
                  setSelectedSession(event.target.value);
                  setExpandedClassID(null);
                  setNotice(null);
                  setFormError(null);
                }}
                className="mt-1 w-full rounded-lg border p-2"
              >
                {sessions.map((session) => (
                  <option key={session.id} value={session.id}>
                    {session.name} ({session.status})
                  </option>
                ))}
              </select>

              {classesLoading && (
                <p role="status" className="mt-4">
                  Loading classes...
                </p>
              )}

              {!classesLoading && classesError && (
                <div className="mt-4">
                  <p role="alert">{classesError}</p>
                  <button
                    type="button"
                    onClick={() => setReloadKey((value) => value + 1)}
                    className="mt-2 rounded-lg border px-4 py-2"
                  >
                    Retry classes
                  </button>
                </div>
              )}

              {!classesLoading && !classesError && classes.length === 0 && (
                <p className="mt-4 text-sm text-slate-600">
                  No classes configured for this academic session.
                </p>
              )}

              {!classesLoading && !classesError && classes.length > 0 && (
                <ul className="mt-4 space-y-3">
                  {classes.map((item) => (
                    <li
                      key={item.id}
                      className="rounded-lg border border-slate-200 p-4"
                    >
                      <strong>{item.name}</strong>
                      <p className="mt-1 text-sm text-slate-600">
                        Code: {item.code} · Status: {item.status}
                      </p>
                      <p className="text-sm text-slate-600">
                        Grade level: {
                          levels.find((level) => level.id === item.grade_level_id)?.name ??
                          "Unknown grade level"
                        }
                      </p>
                      <button
                        type="button"
                        aria-expanded={expandedClassID === item.id}
                        onClick={() =>
                          setExpandedClassID((current) =>
                            current === item.id ? null : item.id,
                          )
                        }
                        className="mt-3 rounded-lg border border-slate-300 px-3 py-2 text-sm"
                      >
                        {expandedClassID === item.id
                          ? `Hide arms for ${item.name}`
                          : `Manage arms for ${item.name}`}
                      </button>
                      {expandedClassID === item.id && (
                        <ClassArmsPanel
                          key={`arms-${schoolSlug}-${selectedSession}-${item.id}`}
                          schoolSlug={schoolSlug}
                          schoolClass={item}
                          canManage={canManage}
                        />
                      )}
                      {canManage && item.status === "ACTIVE" && (
                        <ClassEditControl
                          key={`${schoolSlug}-${selectedSession}-${item.id}`}
                          schoolSlug={schoolSlug}
                          schoolClass={item}
                          onUpdated={(updated) => {
                            if (selectedRef.current !== updated.session_id) {
                              return;
                            }

                            setClasses((current) =>
                              current.map((record) =>
                                record.id === updated.id ? updated : record,
                              ),
                            );
                          }}
                        />
                      )}
                      {canManage && item.status === "ACTIVE" && (
                        <ClassArchiveControl
                          key={`archive-${schoolSlug}-${selectedSession}-${item.id}`}
                          schoolSlug={schoolSlug}
                          schoolClass={item}
                          onArchived={(archived) => {
                            if (selectedRef.current !== archived.session_id) {
                              return;
                            }

                            setClasses((current) =>
                              current.map((record) =>
                                record.id === archived.id ? archived : record,
                              ),
                            );
                          }}
                        />
                      )}
                    </li>
                  ))}
                </ul>
              )}

              {canManage && !classesLoading && !classesError && (
                <form
                  onSubmit={handleCreate}
                  className="mt-6 space-y-4 rounded-xl border bg-slate-50 p-5"
                >
                  <h3 className="font-semibold">Create class</h3>

                  <label className="block text-sm">
                    Grade level
                    <select
                      value={gradeLevelID}
                      onChange={(event) => setGradeLevelID(event.target.value)}
                      required
                      disabled={submitting || activeLevels.length === 0}
                      className="mt-1 block w-full rounded-lg border p-2"
                    >
                      <option value="">Select grade level</option>
                      {activeLevels.map((level) => (
                        <option key={level.id} value={level.id}>
                          {level.name}
                        </option>
                      ))}
                    </select>
                  </label>

                  <label className="block text-sm">
                    Class code
                    <input
                      value={code}
                      onChange={(event) => setCode(event.target.value)}
                      placeholder="PRI_3_2026"
                      maxLength={32}
                      required
                      disabled={submitting}
                      className="mt-1 block w-full rounded-lg border p-2"
                    />
                  </label>

                  <label className="block text-sm">
                    Class name
                    <input
                      value={name}
                      onChange={(event) => setName(event.target.value)}
                      placeholder="Primary 3"
                      maxLength={100}
                      required
                      disabled={submitting}
                      className="mt-1 block w-full rounded-lg border p-2"
                    />
                  </label>

                  <button
                    type="submit"
                    disabled={submitting || activeLevels.length === 0}
                    className="rounded-lg bg-green-700 px-4 py-2 font-medium text-white disabled:opacity-50"
                  >
                    {submitting ? "Creating class..." : "Create class"}
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
            </>
          )}
        </>
      )}
    </section>
  );
}