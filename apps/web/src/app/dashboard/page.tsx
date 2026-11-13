import { headers } from 'next/headers';
import { redirect } from 'next/navigation';
import DashboardClient from './dashboard-client';
import type { AuthSession, AuthUser, CurrentUser } from '@/lib/auth-client';

interface MeResponse {
  data?: {
    user?: AuthUser;
    session?: AuthSession;
    csrf_token?: string;
  };
}

function isAuthUser(value: unknown): value is AuthUser {
  if (!value || typeof value !== 'object') return false;
  const user = value as Partial<AuthUser>;
  return typeof user.id === 'string'
    && typeof user.email === 'string'
    && typeof user.full_name === 'string'
    && typeof user.status === 'string'
    && typeof user.created_at === 'string';
}

function isAuthSession(value: unknown): value is AuthSession {
  if (!value || typeof value !== 'object') return false;
  const session = value as Partial<AuthSession>;
  return typeof session.id === 'string'
    && typeof session.created_at === 'string'
    && typeof session.expires_at === 'string';
}

function VerificationError() {
  return (
    <main className="flex min-h-screen items-center justify-center p-4">
      <section className="max-w-md rounded-xl border border-red-200 bg-white p-6 text-center shadow-sm">
        <h1 className="text-lg font-semibold text-slate-900">Unable to verify your session</h1>
        <p className="mt-2 text-sm text-slate-600">Check your connection and try again.</p>
        <a href="/dashboard" className="mt-4 inline-block rounded-lg bg-green-700 px-4 py-2 text-sm font-medium text-white">
          Try again
        </a>
      </section>
    </main>
  );
}

export default async function DashboardPage() {
  const cookie = (await headers()).get('cookie');
  if (!cookie) redirect('/login');

  const apiBase = process.env.API_INTERNAL_URL || 'http://localhost:8080';
  let response: Response;
  try {
    response = await fetch(new URL('/api/v1/auth/me', apiBase), {
      headers: { Accept: 'application/json', Cookie: cookie },
      cache: 'no-store',
    });
  } catch {
    return <VerificationError />;
  }

  if (response.status === 401 || response.status === 403) redirect('/login');
  if (!response.ok) return <VerificationError />;

  let payload: MeResponse;
  try {
    payload = await response.json() as MeResponse;
  } catch {
    return <VerificationError />;
  }

  const data = payload.data;
  if (!data || !isAuthUser(data.user) || !isAuthSession(data.session) || typeof data.csrf_token !== 'string') {
    return <VerificationError />;
  }

  const user: CurrentUser = {
    ...data.user,
    session_created_at: data.session.created_at,
    session_expires_at: data.session.expires_at,
  };

  return <DashboardClient initialUser={user} csrfToken={data.csrf_token} />;
}
