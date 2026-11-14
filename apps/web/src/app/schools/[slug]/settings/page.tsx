import { headers } from "next/headers";
import { redirect } from "next/navigation";
import SettingsClient from "./settings-client";
import type { Tenant } from "@/lib/tenant-client";

interface ApiEnvelope<T> {
  data?: T;
}

interface MeData {
  csrf_token?: string;
}

function SettingsMessage({ title, message }: { title: string; message: string }) {
  return (
    <main className="flex min-h-screen items-center justify-center p-4">
      <section className="max-w-lg rounded-2xl border border-slate-200 bg-white p-7 text-center shadow-sm">
        <h1 className="text-xl font-semibold text-slate-900">{title}</h1>
        <p className="mt-2 text-sm text-slate-600">{message}</p>
        <a href="/dashboard" className="mt-5 inline-flex rounded-lg bg-green-700 px-4 py-2 text-sm font-semibold text-white hover:bg-green-800">Return to account</a>
      </section>
    </main>
  );
}

export default async function SchoolSettingsPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const cookie = (await headers()).get("cookie");
  if (!cookie) redirect("/login");
  const apiBase = process.env.API_INTERNAL_URL || "http://localhost:8080";
  const encodedSlug = encodeURIComponent(slug);
  let meResponse: Response;
  let settingsResponse: Response;
  try {
    [meResponse, settingsResponse] = await Promise.all([
      fetch(new URL("/api/v1/auth/me", apiBase), {
        headers: { Accept: "application/json", Cookie: cookie },
        cache: "no-store",
      }),
      fetch(new URL(`/api/v1/tenants/${encodedSlug}/settings`, apiBase), {
        headers: { Accept: "application/json", Cookie: cookie },
        cache: "no-store",
      }),
    ]);
  } catch {
    return <SettingsMessage title="Unable to reach the school" message="Check your connection and try again." />;
  }
  if (meResponse.status === 401 || settingsResponse.status === 401) redirect("/login");
  if (settingsResponse.status === 404 || settingsResponse.status === 403) {
    return <SettingsMessage title="School access unavailable" message="This school is unavailable or your account does not have an active membership." />;
  }
  if (!meResponse.ok || !settingsResponse.ok) {
    return <SettingsMessage title="Unable to load school settings" message="The server could not verify your session or load these settings." />;
  }
  let mePayload: ApiEnvelope<MeData>;
  let settingsPayload: ApiEnvelope<Tenant>;
  try {
    [mePayload, settingsPayload] = await Promise.all([
      meResponse.json() as Promise<ApiEnvelope<MeData>>,
      settingsResponse.json() as Promise<ApiEnvelope<Tenant>>,
    ]);
  } catch {
    return <SettingsMessage title="Invalid server response" message="The school settings could not be read." />;
  }
  if (!mePayload.data?.csrf_token || !settingsPayload.data?.id || !settingsPayload.data.current_role) {
    return <SettingsMessage title="Unable to verify school membership" message="The server response did not contain the expected session and membership data." />;
  }
  return <SettingsClient tenant={settingsPayload.data} csrfToken={mePayload.data.csrf_token} />;
}
