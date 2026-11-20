"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { canInviteStaff, listMySchools, type Tenant } from "@/lib/tenant-client";

type LoadState = "loading" | "ready" | "error";

export default function MySchoolsPanel() {
  const [schools, setSchools] = useState<Tenant[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [retryKey, setRetryKey] = useState(0);

  useEffect(() => {
    let cancelled = false;

    listMySchools()
      .then((result) => {
        if (cancelled) return;
        setSchools(result);
        setLoadState("ready");
      })
      .catch(() => {
        if (cancelled) return;
        setLoadState("error");
      });

    return () => {
      cancelled = true;
    };
  }, [retryKey]);

  return (
    <section
      aria-labelledby="my-schools-heading"
      className="mb-6 rounded-2xl border border-gray-100 bg-white p-6 shadow-sm"
    >
      <h2
        id="my-schools-heading"
        className="text-lg font-semibold text-gray-900"
      >
        My Schools
      </h2>

      <p className="mt-1 text-sm text-gray-600">
        Access schools linked to your account.
      </p>

      {loadState === "loading" && (
        <p role="status" className="mt-4 text-sm text-gray-600">
          Loading your schools...
        </p>
      )}

      {loadState === "error" && (
        <div role="alert" className="mt-4">
          <p className="text-sm text-red-700">
            Unable to load your schools.
          </p>
          <button
            type="button"
            onClick={() => {
              setLoadState("loading");
              setRetryKey((value) => value + 1);
            }}
            className="mt-3 rounded-lg bg-green-700 px-4 py-2 text-sm font-medium text-white"
          >
            Retry
          </button>
        </div>
      )}

      {loadState === "ready" && schools.length === 0 && (
        <p className="mt-4 text-sm text-gray-600">
          No active school memberships are currently available.
        </p>
      )}

      {loadState === "ready" && schools.length > 0 && (
        <div className="mt-5 grid gap-4 md:grid-cols-2">
          {schools.map((school) => {
            const base = `/schools/${encodeURIComponent(school.slug)}`;

            return (
              <article
                key={school.id}
                className="rounded-xl border border-gray-200 p-4"
              >
                <h3 className="font-semibold text-gray-900">
                  {school.name}
                </h3>

                <p className="mt-1 text-xs text-gray-600">
                  Role: {school.current_role?.replaceAll("_", " ") ?? "Member"}
                </p>

                <div className="mt-4 flex flex-wrap gap-3">
                  <Link
                    href={`${base}/academics`}
                    className="rounded-lg bg-green-700 px-4 py-2 text-sm font-medium text-white hover:bg-green-800"
                  >
                    Academic Management
                  </Link>

                  <Link
                    href={`${base}/settings`}
                    className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-800 hover:bg-gray-50"
                  >
                    School Settings
                  </Link>
                  {canInviteStaff(school) && (
                    <Link
                      href={`${base}/settings#invite-staff`}
                      className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-800 hover:bg-gray-50"
                    >
                      Invite Staff
                    </Link>
                  )}
                </div>
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
}
