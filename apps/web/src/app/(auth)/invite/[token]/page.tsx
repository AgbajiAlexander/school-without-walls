"use client";

import React, { use, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { verifyInvitation, acceptInvitation, InvitationDetails } from "@/lib/auth-client";
import { passwordPolicy } from "@/lib/password-policy";

interface PageProps {
  params: Promise<{ token: string }>;
}

export default function AcceptInvitationPage({ params }: PageProps) {
  const resolvedParams = use(params);
  const token = resolvedParams.token;
  const router = useRouter();

  const [invitation, setInvitation] = useState<InvitationDetails | null>(null);
  const [loadingVerify, setLoadingVerify] = useState(true);
  const [verifyError, setVerifyError] = useState<string | null>(null);

  const [fullName, setFullName] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function loadInvitation() {
      try {
        const details = await verifyInvitation(token);
        setInvitation(details);
      } catch (err: unknown) {
        if (err instanceof Error) {
          setVerifyError(err.message);
        } else {
          setVerifyError("This invitation link is invalid or has expired.");
        }
      } finally {
        setLoadingVerify(false);
      }
    }
    loadInvitation();
  }, [token]);

  const passwordRequirements = passwordPolicy(password);
  const checks = {
    length: passwordRequirements.lengthValid,
    common: passwordRequirements.notCommon,
    matches: password.length > 0 && password === confirmPassword,
  };
  const isFormValid =
    fullName.trim() !== "" &&
    checks.length &&
    checks.common &&
    checks.matches;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!isFormValid) return;

    setError(null);
    setSubmitting(true);

    try {
      await acceptInvitation(token, fullName, password);
      router.replace(invitation?.tenant_slug
        ? `/schools/${encodeURIComponent(invitation.tenant_slug)}/settings`
        : "/dashboard");
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError("Failed to activate account. Please try again.");
      }
    } finally {
      setSubmitting(false);
    }
  };

  if (loadingVerify) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center p-4">
        <div className="bg-white p-8 rounded-xl border border-slate-200 text-center max-w-sm w-full shadow-sm">
          <div className="inline-block w-8 h-8 border-4 border-green-600 border-t-transparent rounded-full animate-spin mb-4" />
          <p className="text-sm font-medium text-slate-700">Verifying invitation...</p>
        </div>
      </div>
    );
  }

  if (verifyError || !invitation) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center p-4">
        <div className="bg-white p-8 rounded-xl border border-slate-200 text-center max-w-md w-full shadow-sm">
          <div className="w-12 h-12 bg-red-100 text-red-700 rounded-full flex items-center justify-center mx-auto mb-4 font-bold text-xl">
            ✕
          </div>
          <h2 className="text-xl font-bold text-slate-900 mb-2">Invitation Expired or Invalid</h2>
          <p className="text-sm text-slate-600 mb-6">
            {verifyError || "This invitation link could not be verified. It may have expired or already been accepted."}
          </p>
          <Link
            href="/login"
            className="inline-block py-2 px-4 bg-slate-800 hover:bg-slate-900 text-white text-sm font-medium rounded-lg"
          >
            Go to Login
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-slate-50 flex flex-col justify-center py-12 sm:px-6 lg:px-8">
      <div className="sm:mx-auto sm:w-full sm:max-w-md px-4">
        <span className="inline-block px-3 py-1 bg-green-100 text-green-800 text-xs font-semibold rounded-full mb-2">
          Account Activation
        </span>
        <h1 className="text-2xl sm:text-3xl font-extrabold text-slate-900 tracking-tight">
          Complete Your Registration
        </h1>
        <p className="mt-1 text-sm text-slate-600">
          Complete account activation for <span className="font-semibold text-slate-800">{invitation.invited_email}</span>
        </p>
        {invitation.tenant_name && (
          <p className="mt-2 text-sm text-slate-600">
            You are joining <span className="font-semibold text-slate-800">{invitation.tenant_name}</span>.
          </p>
        )}
      </div>

      <div className="mt-8 sm:mx-auto sm:w-full sm:max-w-md px-4">
        <div className="bg-white py-8 px-6 shadow-sm rounded-xl border border-slate-200 sm:px-10">
          {error && (
            <div role="alert" className="mb-6 p-4 rounded-lg bg-red-50 border border-red-200 text-red-700 text-sm">
              <span className="font-bold">Error:</span> {error}
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-5">
            <div>
              <label className="block text-sm font-medium text-slate-700">Invited Email</label>
              <input
                type="email"
                disabled
                value={invitation.invited_email}
                className="mt-1 block w-full px-3 py-2 border border-slate-200 bg-slate-100 rounded-lg text-slate-600 sm:text-sm cursor-not-allowed"
              />
            </div>

            <div>
              <label htmlFor="fullName" className="block text-sm font-medium text-slate-700">
                Full Name
              </label>
              <input
                id="fullName"
                type="text"
                required
                maxLength={150}
                value={fullName}
                onChange={(e) => setFullName(e.target.value)}
                placeholder="e.g. Amaka Okonkwo"
                className="mt-1 block w-full px-3 py-2 border border-slate-300 rounded-lg shadow-sm placeholder-slate-400 focus:outline-none focus:ring-green-500 focus:border-green-500 sm:text-sm text-slate-900"
              />
            </div>

            <div>
              <div className="flex items-center justify-between">
                <label htmlFor="password" className="block text-sm font-medium text-slate-700">
                  Create or confirm password
                </label>
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="text-xs text-slate-500 hover:text-slate-800"
                >
                  {showPassword ? "Hide" : "Show"}
                </button>
              </div>
              <input
                id="password"
                type={showPassword ? "text" : "password"}
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="mt-1 block w-full px-3 py-2 border border-slate-300 rounded-lg shadow-sm placeholder-slate-400 focus:outline-none focus:ring-green-500 focus:border-green-500 sm:text-sm text-slate-900"
              />
              <p className="mt-1 text-xs text-slate-500">
                If you already have an account for this email, use its existing password. Your existing account credentials will not change.
              </p>
            </div>

            <div>
              <label htmlFor="confirmPassword" className="block text-sm font-medium text-slate-700">
                Confirm Password
              </label>
              <input
                id="confirmPassword"
                type={showPassword ? "text" : "password"}
                required
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                className="mt-1 block w-full px-3 py-2 border border-slate-300 rounded-lg shadow-sm placeholder-slate-400 focus:outline-none focus:ring-green-500 focus:border-green-500 sm:text-sm text-slate-900"
              />
            </div>

            {/* Password checklist */}
            <div className="p-3 bg-slate-50 rounded-lg border border-slate-200 text-xs text-slate-600 space-y-1">
              <div className={checks.length ? "text-green-700 font-medium" : ""}>
                {checks.length ? "✓" : "○"} 12 to 128 characters
              </div>
              <div className={checks.common ? "text-green-700 font-medium" : "text-red-600"}>
                {checks.common ? "✓" : "✗"} Avoid common passwords
              </div>
              <p>Passphrases are welcome; uppercase, numbers, and symbols are not required.</p>
              {confirmPassword.length > 0 && (
                <div className={checks.matches ? "text-green-700 font-medium" : "text-red-600"}>
                  {checks.matches ? "✓ Passwords match" : "✗ Passwords do not match"}
                </div>
              )}
            </div>

            <button
              type="submit"
              disabled={submitting || !isFormValid}
              className="w-full flex justify-center py-2.5 px-4 border border-transparent rounded-lg shadow-sm text-sm font-medium text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {submitting ? "Accepting invitation..." : "Accept Invitation & Sign In"}
            </button>
          </form>
        </div>
      </div>
    </div>
  );
}
