"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { canInviteStaff, establishCSRFToken, updateTenantSettings } from "@/lib/tenant-client";
import type { Tenant } from "@/lib/tenant-client";
import StaffInvitationForm from "./staff-invitation-form";

const stages = [
  { value: "EARLY_YEARS", label: "Early years" },
  { value: "PRIMARY", label: "Primary" },
  { value: "JUNIOR_SECONDARY", label: "Junior secondary" },
  { value: "SENIOR_SECONDARY", label: "Senior secondary" },
  { value: "BRITISH_ENRICHMENT", label: "Optional British enrichment" },
];

export default function SettingsClient({ tenant, csrfToken }: { tenant: Tenant; csrfToken: string }) {
  const router = useRouter();
  const [name, setName] = useState(tenant.name);
  const [locale, setLocale] = useState(tenant.locale);
  const [timezone, setTimezone] = useState(tenant.timezone);
  const [selectedStages, setSelectedStages] = useState(tenant.education_stages);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    establishCSRFToken(csrfToken);
  }, [csrfToken]);

  function toggleStage(stage: string) {
    setSelectedStages((current) => current.includes(stage)
      ? current.filter((value) => value !== stage)
      : [...current, stage]);
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setNotice(null);
    setSubmitting(true);
    try {
      await updateTenantSettings(tenant.slug, {
        name: name.trim(),
        locale,
        timezone,
        branding: tenant.branding,
        education_stages: selectedStages,
      });
      setNotice("School settings saved.");
      router.refresh();
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : "Unable to save school settings.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="min-h-screen bg-slate-50">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex max-w-4xl items-center justify-between px-4 py-4 sm:px-6">
          <Link href="/" className="font-bold tracking-tight text-slate-900">School Without Walls</Link>
          <a href="/dashboard" className="text-sm font-medium text-green-800 hover:text-green-900">Account</a>
        </div>
      </header>
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6 sm:py-12">
        <p className="text-sm font-semibold uppercase tracking-wide text-green-800">School setup</p>
        <h1 className="mt-2 text-3xl font-bold tracking-tight text-slate-950">{tenant.name}</h1>
        <p className="mt-2 text-sm text-slate-600">Manage foundational school settings. Your verified membership is <span className="font-semibold">{tenant.current_role}</span>.</p>
        {tenant.status === "PENDING_SETUP" && (
          <div role="status" className="mt-6 rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
            Your account is active. School setup is completing.
          </div>
        )}

        <section className="mt-6 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-8">
          <form onSubmit={handleSubmit} className="space-y-6">
            <div className="grid gap-5 sm:grid-cols-2">
              <div className="sm:col-span-2">
                <label htmlFor="tenant-name" className="block text-sm font-medium text-slate-700">School name</label>
                <input id="tenant-name" required maxLength={160} value={name} onChange={(event) => setName(event.target.value)}
                  className="mt-1.5 w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm text-slate-900 focus:border-green-700 focus:outline-none focus:ring-2 focus:ring-green-700/20" />
              </div>
              <div>
                <label htmlFor="tenant-locale" className="block text-sm font-medium text-slate-700">Language and locale</label>
                <select id="tenant-locale" value={locale} onChange={(event) => setLocale(event.target.value)}
                  className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 focus:border-green-700 focus:outline-none focus:ring-2 focus:ring-green-700/20">
                  <option value="en-NG">English (Nigeria)</option>
                </select>
              </div>
              <div>
                <label htmlFor="tenant-timezone" className="block text-sm font-medium text-slate-700">Time zone</label>
                <select id="tenant-timezone" value={timezone} onChange={(event) => setTimezone(event.target.value)}
                  className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 focus:border-green-700 focus:outline-none focus:ring-2 focus:ring-green-700/20">
                  <option value="Africa/Lagos">Africa/Lagos (WAT)</option>
                </select>
              </div>
            </div>
            <fieldset>
              <legend className="text-sm font-semibold text-slate-800">Education stages</legend>
              <p className="mt-1 text-xs text-slate-500">Choose the stages served by this school. British enrichment is optional and does not replace Nigerian curriculum.</p>
              <div className="mt-3 grid gap-3 sm:grid-cols-2">
                {stages.map((stage) => (
                  <label key={stage.value} className="flex min-h-11 items-center gap-3 rounded-lg border border-slate-200 px-3 py-2.5 text-sm text-slate-700">
                    <input type="checkbox" checked={selectedStages.includes(stage.value)} onChange={() => toggleStage(stage.value)}
                      className="h-4 w-4 rounded border-slate-300 text-green-700 focus:ring-green-700" />
                    {stage.label}
                  </label>
                ))}
              </div>
            </fieldset>
            {error && <p role="alert" className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-800">{error}</p>}
            {notice && <p role="status" className="rounded-lg border border-green-200 bg-green-50 p-3 text-sm text-green-900">{notice}</p>}
            <button type="submit" disabled={submitting || selectedStages.length === 0}
              className="inline-flex min-h-11 items-center justify-center rounded-lg bg-green-700 px-5 py-2.5 text-sm font-semibold text-white transition hover:bg-green-800 disabled:cursor-not-allowed disabled:opacity-60">
              {submitting ? "Saving settings…" : "Save school settings"}
            </button>
          </form>
        </section>
        {canInviteStaff(tenant) && <StaffInvitationForm key={tenant.id} schoolSlug={tenant.slug} />}
      </div>
    </main>
  );
}
