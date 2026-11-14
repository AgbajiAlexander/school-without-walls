import { headers } from "next/headers";
import { redirect } from "next/navigation";
import TenantManagementClient from "./tenant-management-client";
import type { Tenant } from "@/lib/tenant-client";

interface ApiEnvelope<T> {
  data?: T;
}

interface MeData {
  csrf_token?: string;
}

interface TenantListData {
  tenants?: Tenant[];
}

function PageMessage({ title, message }: { title: string; message: string }) {
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

export default async function PlatformTenantsPage() {
  const cookie = (await headers()).get("cookie");
  if (!cookie) redirect("/login");
  const apiBase = process.env.API_INTERNAL_URL || "http://localhost:8080";
  let meResponse: Response;
  let tenantsResponse: Response;
  try {
    [meResponse, tenantsResponse] = await Promise.all([
      fetch(new URL("/api/v1/auth/me", apiBase), {
        headers: { Accept: "application/json", Cookie: cookie },
        cache: "no-store",
      }),
      fetch(new URL("/api/v1/platform/tenants", apiBase), {
        headers: { Accept: "application/json", Cookie: cookie },
        cache: "no-store",
      }),
    ]);
  } catch {
    return <PageMessage title="Unable to reach the platform" message="Check your connection and try again." />;
  }
  if (meResponse.status === 401 || tenantsResponse.status === 401) redirect("/login");
  if (tenantsResponse.status === 403) {
    return <PageMessage title="Platform administrator access required" message="Your verified account is not authorized to provision schools." />;
  }
  if (!meResponse.ok || !tenantsResponse.ok) {
    return <PageMessage title="Unable to load schools" message="The server could not verify access or load school data. Please try again." />;
  }
  let mePayload: ApiEnvelope<MeData>;
  let tenantsPayload: ApiEnvelope<TenantListData>;
  try {
    [mePayload, tenantsPayload] = await Promise.all([
      meResponse.json() as Promise<ApiEnvelope<MeData>>,
      tenantsResponse.json() as Promise<ApiEnvelope<TenantListData>>,
    ]);
  } catch {
    return <PageMessage title="Invalid server response" message="The platform returned data that could not be read." />;
  }
  if (!mePayload.data?.csrf_token || !Array.isArray(tenantsPayload.data?.tenants)) {
    return <PageMessage title="Unable to verify platform access" message="The server response did not contain the expected session and school data." />;
  }
  return <TenantManagementClient initialTenants={tenantsPayload.data.tenants} csrfToken={mePayload.data.csrf_token} />;
}
