"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import SessionCreateForm from "./session-create-form";
import SessionEditForm from "./session-edit-form";
import GradeLevelsPanel from "./grade-levels-panel";
import ClassesPanel from "./classes-panel";
import TermConfigureForm from "./term-configure-form";
import SessionActivationControl from "./session-activation-control";
import SessionArchiveControl from "./session-archive-control";
import type { Tenant } from "@/lib/tenant-client";
import { establishCSRFToken } from "@/lib/tenant-client";
import {
  listAcademicSessions,
  listAcademicTerms,
  type AcademicSession,
  type AcademicTerm,
} from "@/lib/academics-client";

type LoadState = "loading" | "ready" | "error";

function formatDate(value: string): string {
  const date = new Date(`${value.slice(0, 10)}T12:00:00Z`);

  return Number.isNaN(date.getTime())
    ? value
    : date.toLocaleDateString("en-NG", {
        day: "numeric",
        month: "short",
        year: "numeric",
        timeZone: "UTC",
      });
}

export default function AcademicsClient({
  school,
  csrfToken,
}: {
  school: Tenant;
  csrfToken: string;
}) {
  const [gradeLevelsVersion, setGradeLevelsVersion] = useState(0);
  const [sessions, setSessions] = useState<AcademicSession[]>([]);
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const selectedIDRef = useRef<string | null>(null);
  const [terms, setTerms] = useState<AcademicTerm[]>([]);
  const [termsLoadedFor, setTermsLoadedFor] = useState<string | null>(null);
  const [termsNotice, setTermsNotice] = useState<string | null>(null);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [termsLoading, setTermsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [termsError, setTermsError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    establishCSRFToken(csrfToken);
  }, [csrfToken]);

  useEffect(() => {
    let cancelled = false;

    listAcademicSessions(school.slug)
      .then((values) => {
        if (cancelled) return;

        const nextID =
          values.find((item) => item.status === "ACTIVE")?.id ??
          values[0]?.id ??
          null;

        selectedIDRef.current = nextID;
        setSessions(values);
        setSelectedID(nextID);
        setLoadState("ready");
      })
      .catch((cause: unknown) => {
        if (cancelled) return;

        setError(
          cause instanceof Error
            ? cause.message
            : "Unable to load academic sessions.",
        );
        setLoadState("error");
      });

    return () => {
      cancelled = true;
    };
  }, [school.slug, reloadKey]);

  useEffect(() => {
    if (!selectedID || loadState !== "ready") {
      setTerms([]);
      setTermsLoadedFor(null);
      return;
    }

    let cancelled = false;

    setTerms([]);
    setTermsError(null);
    setTermsLoadedFor(null);
    setTermsLoading(true);

    listAcademicTerms(school.slug, selectedID)
      .then((values) => {
        if (!cancelled) {
          setTerms(values);
          setTermsLoadedFor(selectedID);
          setTermsLoading(false);
        }
      })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setTermsError(
            cause instanceof Error
              ? cause.message
              : "Unable to load academic terms.",
          );
          setTermsLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [school.slug, selectedID, loadState]);

  const selectSession = useCallback((id: string) => {
    if (selectedIDRef.current === id) return;

    selectedIDRef.current = id;
    setTerms([]);
    setTermsError(null);
    setTermsNotice(null);
    setTermsLoadedFor(null);
    setTermsLoading(true);
    setSelectedID(id);
  }, []);

  function handleSessionCreated(session: AcademicSession) {
    setSessions((current) => [...current, session]);
    setTerms([]);
    setTermsError(null);
    setTermsNotice(null);
    setTermsLoadedFor(null);
    setTermsLoading(true);
    selectedIDRef.current = session.id;
    setSelectedID(session.id);
    setLoadState("ready");
  }

  function handleSessionUpdated(updated: AcademicSession) {
    setSessions((current) =>
      current.map((session) =>
        session.id === updated.id ? updated : session,
      ),
    );
  }

  function handleSessionActivated(updated: AcademicSession) {
    setSessions((current) =>
      current.map((session) =>
        session.id === updated.id ? updated : session,
      ),
    );
    if (selectedIDRef.current === updated.id) {
      setTermsNotice("Academic session activated.");
    }
  }

  function handleSessionArchived(updated: AcademicSession) {
    setSessions((current) =>
      current.map((session) =>
        session.id === updated.id ? updated : session,
      ),
    );

    if (selectedIDRef.current === updated.id) {
      setTermsNotice("Academic session archived.");
    }
  }

  const selected = sessions.find((item) => item.id === selectedID);

  const canManage = ["SCHOOL_OWNER", "SCHOOL_ADMIN", "PRINCIPAL"].includes(
    school.current_role ?? "",
  );

  return (
    <main className="min-h-screen bg-slate-50">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-3 px-4 py-4 sm:px-6">
          <Link href="/dashboard" className="font-bold text-slate-900">
            School Without Walls
          </Link>
          <Link
            href={`/schools/${encodeURIComponent(school.slug)}/settings`}
            className="text-sm font-semibold text-green-800 hover:underline"
          >
            School settings
          </Link>
        </div>
      </header>

      <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
        <p className="text-sm font-semibold uppercase tracking-wide text-green-800">
          Academic administration
        </p>

        <h1 className="mt-2 text-3xl font-bold text-slate-950">
          Academic Management
        </h1>

        <p className="mt-2 text-slate-600">
          {school.name} - {school.current_role?.replaceAll("_", " ")}
        </p>

        <div className="mt-7 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {[
            ["Sessions and terms", "Available now"],
            ["Levels and classes", "Available now"],
            ["Subjects and offerings", "Next implementation"],
            ["Teacher assignments", "Next implementation"],
          ].map(([title, detail]) => (
            <section
              key={title}
              className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm"
            >
              <h2 className="font-semibold text-slate-900">{title}</h2>
              <p className="mt-2 text-sm text-slate-500">{detail}</p>
            </section>
          ))}
        </div>

        <section className="mt-8 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-7">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h2 className="text-xl font-semibold text-slate-900">
                Academic sessions
              </h2>
              <p className="mt-1 text-sm text-slate-600">
                Review the academic years configured for this school.
              </p>
            </div>
            {canManage && (
              <span className="rounded-lg bg-green-50 px-3 py-2 text-xs font-medium text-green-800">
                Management access
              </span>
            )}
          </div>

          {canManage && loadState === "ready" && (
            <SessionCreateForm
              schoolSlug={school.slug}
              onCreated={handleSessionCreated}
            />
          )}

          {loadState === "loading" && (
            <p role="status" className="mt-6 text-sm text-slate-600">
              Loading academic sessions...
            </p>
          )}

          {loadState === "error" && (
            <div className="mt-6">
              <p role="alert" className="text-sm text-red-700">
                {error}
              </p>
              <button
                type="button"
                onClick={() => {
                  setError(null);
                  setLoadState("loading");
                  setReloadKey((value) => value + 1);
                }}
                className="mt-3 rounded-lg bg-green-700 px-4 py-2 text-sm font-semibold text-white"
              >
                Retry
              </button>
            </div>
          )}

          {loadState === "ready" && sessions.length === 0 && (
            <div role="status" className="mt-6 rounded-xl bg-slate-50 p-5">
              <p className="font-medium text-slate-800">
                No academic sessions configured
              </p>
              <p className="mt-2 text-sm text-slate-600">
                An authorized administrator can create the first academic
                session when the management forms are enabled.
              </p>
            </div>
          )}

          {loadState === "ready" && sessions.length > 0 && (
            <div className="mt-6 grid gap-6 lg:grid-cols-2">
              <div className="space-y-3">
                {sessions.map((session) => (
                  <button
                    key={session.id}
                    type="button"
                    onClick={() => selectSession(session.id)}
                    aria-pressed={session.id === selectedID}
                    className={`w-full rounded-xl border p-4 text-left transition ${
                      session.id === selectedID
                        ? "border-green-600 bg-green-50"
                        : "border-slate-200 hover:border-green-300"
                    }`}
                  >
                    <span className="flex flex-wrap items-center justify-between gap-2">
                      <span className="font-semibold text-slate-900">
                        {session.name}
                      </span>
                      <span className="text-xs font-medium text-slate-600">
                        {session.status}
                      </span>
                    </span>
                    <span className="mt-2 block text-sm text-slate-600">
                      {formatDate(session.starts_on)} to{" "}
                      {formatDate(session.ends_on)}
                    </span>
                  </button>
                ))}
              </div>

              <div className="rounded-xl border border-slate-200 bg-slate-50 p-5">
                <h3 className="text-lg font-semibold text-slate-900">
                  {selected?.name ?? "Select a session"}
                </h3>
                {canManage && selected?.status === "DRAFT" && (
                  <SessionEditForm
                    key={`${school.slug}:${selected.id}`}
                    schoolSlug={school.slug}
                    session={selected}
                    onUpdated={handleSessionUpdated}
                  />
                )}
                <p className="mt-1 text-sm text-slate-600">
                  Academic terms
                </p>

                {termsLoading && (
                  <p role="status" className="mt-4 text-sm">
                    Loading terms...
                  </p>
                )}

                {termsError && (
                  <p role="alert" className="mt-4 text-sm text-red-700">
                    {termsError}
                  </p>
                )}

                {!termsLoading && !termsError && terms.length === 0 && (
                  <p className="mt-4 text-sm text-slate-600">
                    No terms configured for this session.
                  </p>
                )}

                {termsNotice && (
                  <p role="status" className="mt-4 text-sm text-green-800">
                    {termsNotice}
                  </p>
                )}

                {!termsLoading && !termsError && terms.length > 0 && (
                  <ol className="mt-4 space-y-3">
                    {terms.map((term) => (
                      <li
                        key={term.id}
                        className="rounded-lg border border-slate-200 bg-white p-3"
                      >
                        <p className="font-semibold text-slate-900">
                          {term.sequence}. {term.name}
                        </p>
                        <p className="mt-1 text-sm text-slate-600">
                          {formatDate(term.starts_on)} to{" "}
                          {formatDate(term.ends_on)}
                        </p>
                      </li>
                    ))}
                  </ol>
                )}
                {canManage &&
                  selected?.status === "DRAFT" &&
                  !termsLoading &&
                  !termsError &&
                  termsLoadedFor === selectedID && (
                    <TermConfigureForm
                      key={selected.id}
                      schoolSlug={school.slug}
                      session={selected}
                      existingTerms={terms}
                      onSaved={(saved) => {
                        if (selectedIDRef.current !== selected.id) return;
                        setTerms(saved);
                        setTermsNotice("Academic terms saved.");
                      }}
                    />
                  )}

                {canManage &&
                  selected?.status === "DRAFT" &&
                  !termsLoading &&
                  !termsError &&
                  termsLoadedFor === selectedID && (
                    <SessionActivationControl
                      key={`activate-${selected.id}`}
                      schoolSlug={school.slug}
                      session={selected}
                      terms={terms}
                      onActivated={handleSessionActivated}
                    />
                  )}

                {canManage &&
                  selected &&
                  selected.status !== "ARCHIVED" &&
                  !termsLoading &&
                  !termsError &&
                  termsLoadedFor === selectedID && (
                    <SessionArchiveControl
                      key={`archive-${selected.id}`}
                      schoolSlug={school.slug}
                      session={selected}
                      onArchived={handleSessionArchived}
                    />
                  )}
              </div>
            </div>
          )}
        </section>

        <GradeLevelsPanel
          onCreated={() => setGradeLevelsVersion((value) => value + 1)}
          key={`${school.id}:${school.slug}`}
          schoolSlug={school.slug}
          canManage={canManage}
        />

        <ClassesPanel
          gradeLevelsVersion={gradeLevelsVersion}
          key={`classes-${school.id}:${school.slug}`}
          schoolSlug={school.slug}
          canManage={canManage}
        />
      </div>
    </main>
  );
}