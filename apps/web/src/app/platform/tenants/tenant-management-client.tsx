"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { establishCSRFToken, loadCapturedMessages, provisionTenant } from "@/lib/tenant-client";
import type { CapturedMessage, Tenant } from "@/lib/tenant-client";

function TenantStatus({ status }: { status: Tenant["status"] }) {
  const style = status === "ACTIVE"
    ? "bg-green-100 text-green-800"
    : status === "PENDING_SETUP"
      ? "bg-amber-100 text-amber-800"
      : "bg-slate-100 text-slate-700";
  return <span className={`rounded-full px-2.5 py-1 text-xs font-semibold ${style}`}>{status.replaceAll("_", " ")}</span>;
}

export default function TenantManagementClient({ initialTenants, csrfToken }: { initialTenants: Tenant[]; csrfToken: string }) {
  const router = useRouter();
  const [tenants, setTenants] = useState(initialTenants);
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [ownerEmail, setOwnerEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [messages, setMessages] = useState<CapturedMessage[] | null>(null);
  const [mailError, setMailError] = useState<string | null>(null);
  const [loadingMail, setLoadingMail] = useState(false);

  useEffect(() => {
    establishCSRFToken(csrfToken);
  }, [csrfToken]);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setNotice(null);
    setSubmitting(true);
    try {
      const tenant = await provisionTenant({
        slug: slug.trim().toLowerCase(),
        name: name.trim(),
        owner_email: ownerEmail.trim(),
        locale: "en-NG",
        timezone: "Africa/Lagos",
      });
      setTenants((current) => [tenant, ...current]);
      setNotice(`School created. An owner invitation was sent to ${ownerEmail.trim()}.`);
      setSlug("");
      setName("");
      setOwnerEmail("");
      router.refresh();
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : "Unable to create the school.");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleLoadMessages() {
    setLoadingMail(true);
    setMailError(null);
    try {
      setMessages(await loadCapturedMessages());
    } catch (requestError: unknown) {
      const status = (requestError as Error & { status?: number }).status;
      setMailError(status === 404
        ? "Captured email is available only in local development and test environments."
        : requestError instanceof Error ? requestError.message : "Unable to load captured email.");
      setMessages(null);
    } finally {
      setLoadingMail(false);
    }
  }

  return (
    <main className="min-h-screen bg-slate-50">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-4 py-4 sm:px-6">
          <Link href="/" className="font-bold tracking-tight text-slate-900">School Without Walls</Link>
          <a href="/dashboard" className="text-sm font-medium text-green-800 hover:text-green-900">Account</a>
        </div>
      </header>
      <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6 sm:py-12">
        <div className="mb-8">
          <p className="text-sm font-semibold uppercase tracking-wide text-green-800">Platform administration</p>
          <h1 className="mt-2 text-3xl font-bold tracking-tight text-slate-950">Schools</h1>
          <p className="mt-2 max-w-2xl text-sm text-slate-600">Provision a school and invite its first owner. School memberships and access are verified by the API.</p>
        </div>

        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(20rem,0.8fr)]">
          <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-7">
            <h2 className="text-lg font-semibold text-slate-900">Create a school</h2>
            <form onSubmit={handleSubmit} className="mt-5 space-y-4">
              <div>
                <label htmlFor="school-name" className="block text-sm font-medium text-slate-700">School name</label>
                <input id="school-name" required maxLength={160} value={name} onChange={(event) => setName(event.target.value)}
                  className="mt-1.5 w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm text-slate-900 shadow-sm focus:border-green-700 focus:outline-none focus:ring-2 focus:ring-green-700/20" />
              </div>
              <div>
                <label htmlFor="school-slug" className="block text-sm font-medium text-slate-700">School web address</label>
                <div className="mt-1.5 flex overflow-hidden rounded-lg border border-slate-300 shadow-sm focus-within:ring-2 focus-within:ring-green-700/20">
                  <input id="school-slug" required pattern="[a-z0-9]+(-[a-z0-9]+)*" maxLength={80}
                    value={slug} onChange={(event) => setSlug(event.target.value.toLowerCase())}
                    aria-describedby="slug-help"
                    className="min-w-0 flex-1 px-3 py-2.5 text-sm text-slate-900 focus:outline-none" />
                  <span className="flex items-center bg-slate-50 px-3 text-xs text-slate-500">school address</span>
                </div>
                <p id="slug-help" className="mt-1 text-xs text-slate-500">Lowercase letters, numbers and single hyphens.</p>
              </div>
              <div>
                <label htmlFor="owner-email" className="block text-sm font-medium text-slate-700">First school owner email</label>
                <input id="owner-email" type="email" autoComplete="email" required maxLength={254} value={ownerEmail}
                  onChange={(event) => setOwnerEmail(event.target.value)}
                  className="mt-1.5 w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm text-slate-900 shadow-sm focus:border-green-700 focus:outline-none focus:ring-2 focus:ring-green-700/20" />
                <p className="mt-1 text-xs text-slate-500">The owner chooses their own password through a one-time invitation.</p>
              </div>
              {error && <p role="alert" className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-800">{error}</p>}
              {notice && <p role="status" className="rounded-lg border border-green-200 bg-green-50 p-3 text-sm text-green-900">{notice}</p>}
              <button type="submit" disabled={submitting}
                className="inline-flex min-h-11 w-full items-center justify-center rounded-lg bg-green-700 px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-green-800 disabled:cursor-not-allowed disabled:opacity-60 sm:w-auto">
                {submitting ? "Creating school…" : "Create school and invite owner"}
              </button>
            </form>
          </section>

          <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-7">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h2 className="text-lg font-semibold text-slate-900">Schools on the platform</h2>
                <p className="mt-1 text-sm text-slate-500">{tenants.length} {tenants.length === 1 ? "school" : "schools"}</p>
              </div>
              <button type="button" onClick={handleLoadMessages} disabled={loadingMail}
                className="rounded-lg border border-slate-300 px-3 py-2 text-xs font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-60">
                {loadingMail ? "Loading…" : "Local email capture"}
              </button>
            </div>
            {tenants.length === 0 ? (
              <div className="mt-6 rounded-xl border border-dashed border-slate-300 p-6 text-center">
                <p className="font-medium text-slate-800">No schools yet</p>
                <p className="mt-1 text-sm text-slate-500">Create the first school to begin onboarding.</p>
              </div>
            ) : (
              <ul className="mt-5 divide-y divide-slate-100">
                {tenants.map((tenant) => (
                  <li key={tenant.id} className="flex flex-wrap items-center justify-between gap-3 py-4 first:pt-0">
                    <div className="min-w-0">
                      <p className="truncate font-medium text-slate-900">{tenant.name}</p>
                      <p className="mt-0.5 text-xs text-slate-500">{tenant.slug}</p>
                    </div>
                    <TenantStatus status={tenant.status} />
                  </li>
                ))}
              </ul>
            )}
            {mailError && <p role="status" className="mt-4 rounded-lg bg-amber-50 p-3 text-sm text-amber-900">{mailError}</p>}
            {messages && (
              <div className="mt-5 border-t border-slate-200 pt-4">
                <h3 className="text-sm font-semibold text-slate-800">Captured local messages</h3>
                {messages.length === 0 ? <p className="mt-2 text-sm text-slate-500">No messages captured this run.</p> : (
                  <ul className="mt-3 space-y-3">
                    {messages.map((message, index) => (
                      <li key={`${message.created_at}-${index}`} className="rounded-lg bg-slate-50 p-3">
                        <p className="text-xs font-semibold text-slate-700">{message.subject} — {message.to}</p>
                        <pre className="mt-2 whitespace-pre-wrap break-words font-sans text-xs text-slate-600">{message.body}</pre>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
          </section>
        </div>
      </div>
    </main>
  );
}
