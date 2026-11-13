'use client';

import { useState, useEffect, FormEvent, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { resetPassword } from '@/lib/auth-client';
import { passwordPolicy } from '@/lib/password-policy';

type PageState = 'idle' | 'loading' | 'success' | 'error' | 'invalid_token';

function ResetPasswordForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const token = searchParams.get('token') ?? '';

  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [pageState, setPageState] = useState<PageState>('idle');
  const [errorMessage, setErrorMessage] = useState('');

  useEffect(() => {
    if (!token) {
      setPageState('invalid_token');
      return;
    }
    const url = new URL(window.location.href);
    url.searchParams.delete('token');
    window.history.replaceState(window.history.state, '', `${url.pathname}${url.search}${url.hash}`);
  }, [token]);

  const passwordRequirements = passwordPolicy(password);
  const allRulesMet = passwordRequirements.valid;
  const passwordsMatch = password === confirm && confirm.length > 0;
  const canSubmit = allRulesMet && passwordsMatch && pageState !== 'loading';

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!canSubmit) return;

    setPageState('loading');
    setErrorMessage('');

    try {
      await resetPassword(token, password);
      setPageState('success');
      // Redirect to login after 2s
      setTimeout(() => router.push('/login?reset=success'), 2000);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '';
      if (msg.includes('expired') || msg.includes('invalid') || msg.includes('not found')) {
        setPageState('invalid_token');
      } else {
        setPageState('error');
        setErrorMessage('Something went wrong. Please request a new reset link.');
      }
    }
  }

  // ── Invalid / expired token ──────────────────────────────────────────────
  if (pageState === 'invalid_token') {
    return (
      <div className="bg-white rounded-2xl shadow-xl p-8 text-center">
        <div className="mx-auto w-16 h-16 bg-red-100 rounded-full flex items-center justify-center mb-6">
          <svg className="w-8 h-8 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
        </div>
        <h2 className="text-xl font-bold text-gray-900 mb-2">Link Invalid or Expired</h2>
        <p className="text-gray-600 text-sm mb-6">
          This password reset link is invalid or has expired. Reset links are valid for 15 minutes and can only be used once.
        </p>
        <Link
          href="/forgot-password"
          className="inline-block w-full py-3 px-4 bg-green-600 hover:bg-green-700 text-white font-semibold rounded-xl transition-colors text-center"
        >
          Request a New Link
        </Link>
        <div className="mt-3">
          <Link href="/login" className="text-sm text-gray-500 hover:text-gray-700 underline underline-offset-2">
            Back to Sign In
          </Link>
        </div>
      </div>
    );
  }

  // ── Success ───────────────────────────────────────────────────────────────
  if (pageState === 'success') {
    return (
      <div className="bg-white rounded-2xl shadow-xl p-8 text-center">
        <div className="mx-auto w-16 h-16 bg-green-100 rounded-full flex items-center justify-center mb-6">
          <svg className="w-8 h-8 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
          </svg>
        </div>
        <h2 className="text-xl font-bold text-gray-900 mb-2">Password Reset!</h2>
        <p className="text-gray-600 text-sm mb-6">
          Your password has been updated. Redirecting you to sign in…
        </p>
        <Link
          href="/login?reset=success"
          className="inline-block w-full py-3 px-4 bg-green-600 hover:bg-green-700 text-white font-semibold rounded-xl transition-colors text-center"
        >
          Go to Sign In
        </Link>
      </div>
    );
  }

  // ── Form ──────────────────────────────────────────────────────────────────
  return (
    <div className="bg-white rounded-2xl shadow-xl p-8">
      {pageState === 'error' && errorMessage && (
        <div className="mb-5 p-4 bg-red-50 border border-red-200 rounded-xl flex items-start gap-3">
          <svg className="w-5 h-5 text-red-500 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
            <path fillRule="evenodd"
              d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
              clipRule="evenodd" />
          </svg>
          <p className="text-sm text-red-700">{errorMessage}</p>
        </div>
      )}

      <form onSubmit={handleSubmit} noValidate className="space-y-5">
        {/* New Password */}
        <div>
          <label htmlFor="password" className="block text-sm font-semibold text-gray-700 mb-1.5">
            New Password
          </label>
          <div className="relative">
            <input
              id="password"
              type={showPassword ? 'text' : 'password'}
              autoComplete="new-password"
              required
              value={password}
              onChange={e => setPassword(e.target.value)}
              disabled={pageState === 'loading'}
              placeholder="Create a strong password"
              className="w-full px-4 py-3 pr-12 border border-gray-300 rounded-xl text-gray-900 placeholder-gray-400
                focus:outline-none focus:ring-2 focus:ring-green-500 focus:border-transparent
                disabled:opacity-50 disabled:bg-gray-50 transition-shadow"
            />
            <button
              type="button"
              onClick={() => setShowPassword(v => !v)}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600"
              aria-label={showPassword ? 'Hide password' : 'Show password'}
            >
              {showPassword ? (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                    d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" />
                </svg>
              ) : (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                    d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                    d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                </svg>
              )}
            </button>
          </div>

          {/* Password rules checklist */}
          {password.length > 0 && (
            <ul className="mt-3 space-y-1.5">
              {[
                { label: '12 to 128 characters', met: passwordRequirements.lengthValid },
                { label: 'Avoid common passwords', met: passwordRequirements.notCommon },
              ].map(rule => {
                return (
                  <li key={rule.label} className={`flex items-center gap-2 text-xs font-medium ${rule.met ? 'text-green-600' : 'text-gray-400'}`}>
                    {rule.met ? (
                      <svg className="w-4 h-4 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd"
                          d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                          clipRule="evenodd" />
                      </svg>
                    ) : (
                      <svg className="w-4 h-4 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                        <path fillRule="evenodd"
                          d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
                          clipRule="evenodd" />
                      </svg>
                    )}
                    {rule.label}
                  </li>
                );
              })}
              <li className="text-xs text-gray-500">Passphrases are welcome; uppercase, numbers, and symbols are not required.</li>
            </ul>
          )}
        </div>

        {/* Confirm Password */}
        <div>
          <label htmlFor="confirm" className="block text-sm font-semibold text-gray-700 mb-1.5">
            Confirm New Password
          </label>
          <input
            id="confirm"
            type={showPassword ? 'text' : 'password'}
            autoComplete="new-password"
            required
            value={confirm}
            onChange={e => setConfirm(e.target.value)}
            disabled={pageState === 'loading'}
            placeholder="Repeat your new password"
            className={`w-full px-4 py-3 border rounded-xl text-gray-900 placeholder-gray-400
              focus:outline-none focus:ring-2 focus:ring-green-500 focus:border-transparent
              disabled:opacity-50 disabled:bg-gray-50 transition-shadow
              ${confirm.length > 0 && !passwordsMatch ? 'border-red-400 bg-red-50' : 'border-gray-300'}`}
          />
          {confirm.length > 0 && !passwordsMatch && (
            <p className="mt-1.5 text-xs text-red-600 font-medium">Passwords do not match</p>
          )}
          {confirm.length > 0 && passwordsMatch && (
            <p className="mt-1.5 text-xs text-green-600 font-medium">Passwords match ✓</p>
          )}
        </div>

        {/* Submit */}
        <button
          type="submit"
          disabled={!canSubmit}
          className="w-full py-3 px-4 bg-green-600 hover:bg-green-700 disabled:bg-green-300
            text-white font-semibold rounded-xl transition-colors
            focus:outline-none focus:ring-2 focus:ring-green-500 focus:ring-offset-2
            flex items-center justify-center gap-2"
        >
          {pageState === 'loading' ? (
            <>
              <svg className="animate-spin w-5 h-5" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
              Resetting…
            </>
          ) : (
            'Set New Password'
          )}
        </button>
      </form>

      <div className="mt-6 text-center">
        <Link href="/login" className="inline-flex items-center gap-1.5 text-sm text-gray-600 hover:text-green-700 font-medium">
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
          </svg>
          Back to Sign In
        </Link>
      </div>
    </div>
  );
}

export default function ResetPasswordPage() {
  return (
    <div className="min-h-screen bg-gradient-to-br from-green-50 to-emerald-100 flex items-center justify-center p-4">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-14 h-14 bg-green-600 rounded-2xl mb-4 shadow-lg">
            <svg className="w-7 h-7 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
            </svg>
          </div>
          <h1 className="text-3xl font-bold text-gray-900">New Password</h1>
          <p className="text-gray-600 mt-2">Create a strong password for your account</p>
        </div>

        <Suspense fallback={
          <div className="bg-white rounded-2xl shadow-xl p-8 text-center text-gray-500">
            Loading…
          </div>
        }>
          <ResetPasswordForm />
        </Suspense>

        <p className="text-center text-xs text-gray-500 mt-6">
          School Without Walls &copy; {new Date().getFullYear()}
        </p>
      </div>
    </div>
  );
}
