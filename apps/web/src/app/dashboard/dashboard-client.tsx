'use client';

import { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import MySchoolsPanel from './my-schools-panel';
import { getMe, logout, setCSRFToken, type CurrentUser } from '@/lib/auth-client';

type LoadState = 'loading' | 'ready' | 'error';

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString('en-NG', {
    dateStyle: 'long',
    timeStyle: 'short',
  });
}

function getInitials(name: string): string {
  return name
    .split(' ')
    .filter(Boolean)
    .slice(0, 2)
    .map(w => w[0])
    .join('')
    .toUpperCase();
}

export default function DashboardClient({
  initialUser,
  csrfToken,
}: {
  initialUser: CurrentUser;
  csrfToken: string;
}) {
  const router = useRouter();
  const [user, setUser] = useState<CurrentUser | null>(initialUser);
  const [loadState, setLoadState] = useState<LoadState>('ready');
  const [loggingOut, setLoggingOut] = useState(false);
  const [logoutError, setLogoutError] = useState<string | null>(null);

  const fetchUser = useCallback(async () => {
    setLoadState('loading');
    try {
      const me = await getMe();
      setUser(me);
      setLoadState('ready');
    } catch (error: unknown) {
      const code = error instanceof Error ? (error as Error & { code?: string }).code : undefined;
      if (code === 'UNAUTHENTICATED') {
        router.replace('/login');
      } else {
        setLoadState('error');
      }
    }
  }, [router]);

  useEffect(() => {
    setCSRFToken(csrfToken);
  }, [csrfToken]);

  async function handleLogout() {
    if (loggingOut) return;
    setLoggingOut(true);
    try {
      await logout();
      router.replace('/login');
    } catch {
      setLogoutError('Unable to safely end this session. Please try again.');
    } finally {
      setLoggingOut(false);
    }
  }

  // ── Loading ───────────────────────────────────────────────────────────────
  if (loadState === 'loading') {
    return (
      <div className="min-h-screen bg-gradient-to-br from-green-50 to-emerald-100 flex items-center justify-center">
        <div className="text-center">
          <svg className="animate-spin w-10 h-10 text-green-600 mx-auto mb-4" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
          <p className="text-gray-600 font-medium">Verifying session…</p>
        </div>
      </div>
    );
  }

  if (loadState === 'error') {
    return (
      <main className="min-h-screen flex items-center justify-center p-4">
        <div className="max-w-md rounded-xl border border-red-200 bg-white p-6 text-center shadow-sm">
          <h1 className="text-lg font-semibold text-slate-900">Unable to verify your session</h1>
          <p className="mt-2 text-sm text-slate-600">Check your connection and try again.</p>
          <button onClick={fetchUser} className="mt-4 rounded-lg bg-green-700 px-4 py-2 text-sm font-medium text-white">
            Try again
          </button>
        </div>
      </main>
    );
  }
  if (!user) return null;

  // ── Dashboard ─────────────────────────────────────────────────────────────
  return (
    <div className="min-h-screen bg-gradient-to-br from-green-50 to-emerald-100">
      {/* Nav */}
      <nav className="bg-white shadow-sm border-b border-gray-100">
        <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8 flex items-center justify-between h-16">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 bg-green-600 rounded-lg flex items-center justify-center">
              <svg className="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                  d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
              </svg>
            </div>
            <span className="font-bold text-gray-900 text-lg tracking-tight">School Without Walls</span>
          </div>

          <button
            onClick={handleLogout}
            disabled={loggingOut}
            className="inline-flex items-center gap-2 px-4 py-2 text-sm font-medium text-gray-700
              bg-gray-100 hover:bg-gray-200 disabled:opacity-50 rounded-lg transition-colors"
          >
            {loggingOut ? (
              <>
                <svg className="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                Signing out…
              </>
            ) : (
              <>
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                    d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
                </svg>
                Sign Out
              </>
            )}
          </button>
        </div>
      </nav>

      {/* Main */}
      <main className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8 py-10">
        {/* Welcome banner */}
        <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-6 mb-6 flex items-center gap-5">
          {/* Avatar */}
          <div className="w-16 h-16 bg-green-600 rounded-full flex items-center justify-center flex-shrink-0">
            <span className="text-white font-bold text-xl">
              {getInitials(user.full_name)}
            </span>
          </div>

          <div className="flex-1 min-w-0">
            <h1 className="text-2xl font-bold text-gray-900 truncate">
              Welcome back, {user.full_name.split(' ')[0]}!
            </h1>
            <p className="text-gray-500 text-sm mt-0.5">
              You&apos;re signed in to School Without Walls
            </p>
          </div>

        </div>

        {logoutError && <p role="alert" className="mb-6 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">{logoutError}</p>}

        <MySchoolsPanel />

        {/* Cards */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {/* Account details */}
          <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-6">
            <h2 className="text-base font-semibold text-gray-900 mb-4 flex items-center gap-2">
              <svg className="w-5 h-5 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                  d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
              </svg>
              Account Details
            </h2>
            <dl className="space-y-3">
              <div>
                <dt className="text-xs font-medium text-gray-400 uppercase tracking-wide">Full Name</dt>
                <dd className="text-gray-900 font-medium mt-0.5">{user.full_name}</dd>
              </div>
              <div>
                <dt className="text-xs font-medium text-gray-400 uppercase tracking-wide">Email Address</dt>
                <dd className="text-gray-900 font-medium mt-0.5">{user.email}</dd>
              </div>
              <div>
                <dt className="text-xs font-medium text-gray-400 uppercase tracking-wide">Account Status</dt>
                <dd className="mt-0.5">
                  <span className={`inline-flex items-center gap-1.5 text-sm font-medium capitalize
                    ${user.status === 'ACTIVE' ? 'text-green-700' : 'text-amber-600'}`}>
                    <span className={`w-2 h-2 rounded-full ${user.status === 'ACTIVE' ? 'bg-green-500' : 'bg-amber-400'}`} />
                    {user.status.toLowerCase().replaceAll('_', ' ')}
                  </span>
                </dd>
              </div>
            </dl>
          </div>

          {/* Session info */}
          <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-6">
            <h2 className="text-base font-semibold text-gray-900 mb-4 flex items-center gap-2">
              <svg className="w-5 h-5 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                  d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
              </svg>
              Current Session
            </h2>
            <dl className="space-y-3">
              <div>
                <dt className="text-xs font-medium text-gray-400 uppercase tracking-wide">Session Started</dt>
                <dd className="text-gray-900 font-medium mt-0.5">{formatDate(user.session_created_at)}</dd>
              </div>
              <div>
                <dt className="text-xs font-medium text-gray-400 uppercase tracking-wide">Session Expires</dt>
                <dd className="text-gray-900 font-medium mt-0.5">{formatDate(user.session_expires_at)}</dd>
              </div>
              <div>
                <dt className="text-xs font-medium text-gray-400 uppercase tracking-wide">Account Created</dt>
                <dd className="text-gray-900 font-medium mt-0.5">
                  {formatDate(user.created_at)}
                </dd>
              </div>
            </dl>

            <div className="mt-5 pt-4 border-t border-gray-100">
              <p className="text-xs text-gray-400">
                Your session is protected by server-side authentication. Use My Schools above to access the schools linked to your account.
              </p>
            </div>
          </div>
        </div>

        {/* Coming-soon notice */}
        <div className="mt-6 bg-amber-50 border border-amber-200 rounded-2xl p-5 flex items-start gap-4">
          <svg className="w-6 h-6 text-amber-500 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
          <div>
            <h3 className="text-sm font-semibold text-amber-800">Platform in development</h3>
            <p className="text-sm text-amber-700 mt-0.5">
              Academic Management and school-specific access are available for authorized members.
              Student enrolment, curriculum delivery, and additional role-based dashboards remain under development.
            </p>
          </div>
        </div>
      </main>
    </div>
  );
}
