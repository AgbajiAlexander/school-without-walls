import Link from "next/link";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import AcademicsClient from "./academics-client";
import type { Tenant } from "@/lib/tenant-client";

interface Envelope<T> {
  data?: T;
}

interface MeData {
  csrf_token?: string;
}

function AccessMessage({
  title,
  message,
}: {
  title: string;
  message: string;
}) {
  return (
    <main className="flex min-h-screen items-center justify-center bg-slate-50 p-4">
      <section className="w-full max-w-lg rounded-2xl border border-slate-200 bg-white p-7 text-center shadow-sm">
        <h1 className="text-xl font-semibold text-slate-900">{title}</h1>
        <p className="mt-3 text-sm text-slate-600">{message}</p>
        <Link
          href="/dashboard"
          className="mt-6 inline-flex rounded-lg bg-green-700 px-5 py-3 text-sm font-semibold text-white"
        >
          Return to dashboard
        </Link>
      </section>
    </main>
  );
}

export default async function AcademicManagementPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const cookie = (await headers()).get("cookie");

  if (!cookie) {
    redirect("/login");
  }

  const baseURL = process.env.API_INTERNAL_URL || "http://localhost:8080";
  const encodedSlug = encodeURIComponent(slug);

  let meResponse: Response;
  let schoolResponse: Response;

  try {
    [meResponse, schoolResponse] = await Promise.all([
      fetch(new URL("/api/v1/auth/me", baseURL), {
        headers: { Accept: "application/json", Cookie: cookie },
        cache: "no-store",
      }),
      fetch(new URL(`/api/v1/tenants/${encodedSlug}/settings`, baseURL), {
        headers: { Accept: "application/json", Cookie: cookie },
        cache: "no-store",
      }),
    ]);
  } catch {
    return (
      <AccessMessage
        title="Unable to reach the school"
        message="The academic service could not be reached. Please try again."
      />
    );
  }

  if (meResponse.status === 401 || schoolResponse.status === 401) {
    redirect("/login");
  }

  if (schoolResponse.status === 403 || schoolResponse.status === 404) {
    return (
      <AccessMessage
        title="School access unavailable"
        message="Your account does not have access to this school."
      />
    );
  }

  if (!meResponse.ok || !schoolResponse.ok) {
    return (
      <AccessMessage
        title="Unable to verify school"
        message="Your session or school membership could not be verified."
      />
    );
  }

  let me: Envelope<MeData>;
  let school: Envelope<Tenant>;

  try {
    [me, school] = await Promise.all([
      meResponse.json() as Promise<Envelope<MeData>>,
      schoolResponse.json() as Promise<Envelope<Tenant>>,
    ]);
  } catch {
    return (
      <AccessMessage
        title="Invalid server response"
        message="The school information could not be read."
      />
    );
  }

  if (
    !me.data?.csrf_token ||
    !school.data?.id ||
    !school.data?.current_role ||
    school.data.slug !== slug
  ) {
    return (
      <AccessMessage
        title="School verification incomplete"
        message="The server did not return a verified school context."
      />
    );
  }

  return (
    <AcademicsClient
      school={school.data}
      csrfToken={me.data.csrf_token}
    />
  );
}