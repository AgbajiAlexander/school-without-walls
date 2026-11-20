"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import { canInviteStaff, establishCSRFToken, isStaffInvitationRole, sendStaffInvitation, verifyStaffInvitationAccess } from "@/lib/tenant-client";

const messages = {
  "rate-limited": "Too many staff invitations. Please try again later.",
  invalid: "Check the email address and role, then try again.",
  conflict: "This email already has a pending invitation or membership for that role.",
  unauthenticated: "Your session has expired. Sign in again to invite staff.",
  forbidden: "You do not have permission to invite staff for this school.",
  unconfirmed: "Invitation delivery could not be confirmed. Check your connection before trying again.",
};

export default function StaffInvitationForm({ schoolId, schoolSlug, onAuthorizationDenied }: {
  schoolId: string;
  schoolSlug: string;
  onAuthorizationDenied: () => void;
}) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("");
  const [sending, setSending] = useState(false);
  const [feedback, setFeedback] = useState<{ kind: "email" | "role" | "error" | "success"; message: string } | null>(null);
  const submitting = useRef(false);
  const mounted = useRef(false);
  const verification = useRef<AbortController | null>(null);
  const accessConfirmed = useRef(true);
  const [access, setAccess] = useState<"confirmed" | "checking" | "unconfirmed" | "denied">("confirmed");

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      verification.current?.abort();
    };
  }, []);
  const emailInput = useRef<HTMLInputElement>(null);
  const roleInput = useRef<HTMLSelectElement>(null);

  async function revalidateAccess() {
    if (verification.current) return;
    const controller = new AbortController();
    verification.current = controller;
    setAccess("checking");
    const isCurrent = () => mounted.current && !controller.signal.aborted;
    try {
      const { school, csrfToken } = await verifyStaffInvitationAccess(schoolSlug, controller.signal);
      if (!isCurrent()) return;
      if (school?.id === schoolId && school.slug === schoolSlug && canInviteStaff(school) &&
          typeof csrfToken === "string" && csrfToken.length > 0) {
        establishCSRFToken(csrfToken);
        accessConfirmed.current = true;
        setAccess("confirmed");
        setFeedback({ kind: "success", message: "Invitation access was refreshed. Submit again to send the invitation." });
      } else {
        setAccess("denied");
        onAuthorizationDenied();
      }
    } catch (error: unknown) {
      if (!isCurrent()) return;
      const status = error instanceof Error && "status" in error ? error.status : undefined;
      if (status === 401 || status === 403 || status === 404) {
        if (status === 401) setFeedback({ kind: "error", message: messages.unauthenticated });
        setAccess("denied");
        onAuthorizationDenied();
      } else {
        setAccess("unconfirmed");
      }
    } finally {
      controller.abort();
      verification.current = null;
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current || !accessConfirmed.current) return;

    const address = email.trim();
    if (!address || address.length > 254 || emailInput.current?.validity.typeMismatch) {
      setFeedback({ kind: "email", message: "Enter a valid email address." });
      emailInput.current?.focus();
      return;
    }
    if (!isStaffInvitationRole(role)) {
      setFeedback({ kind: "role", message: "Choose School Administrator or Principal." });
      roleInput.current?.focus();
      return;
    }

    submitting.current = true;
    setSending(true);
    setFeedback(null);
    try {
      const result = await sendStaffInvitation(schoolSlug, { email: address, role });
      if (!mounted.current) return;
      if (result === "unauthenticated" || result === "forbidden") {
        accessConfirmed.current = false;
        setFeedback({ kind: "error", message: messages[result] });
        if (result === "unauthenticated") {
          setAccess("denied");
          onAuthorizationDenied();
        } else {
          await revalidateAccess();
        }
        return;
      }
      if (result === "sent") {
        setEmail("");
        setRole("");
        setFeedback({ kind: "success", message: "Invitation sent." });
      } else {
        setFeedback({ kind: "error", message: messages[result] });
      }
    } finally {
      submitting.current = false;
      if (mounted.current) setSending(false);
    }
  }

  const fieldClass = "mt-1.5 min-h-11 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm text-slate-900 focus:border-green-700 focus:outline-none focus:ring-2 focus:ring-green-700/20";

  return (
    <section id="invite-staff" aria-labelledby="invite-staff-heading" className="mt-6 scroll-mt-4 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-8">
      <h2 id="invite-staff-heading" className="text-lg font-semibold text-slate-900">Invite Staff</h2>
      {access === "checking" && <p role="status" className="mt-3 text-sm text-slate-700">Verifying invitation access?</p>}
      {access === "unconfirmed" && (
        <button type="button" onClick={revalidateAccess} className="mt-3 min-h-11 text-sm font-semibold text-green-800 underline">
          Verify invitation access
        </button>
      )}
      {access === "confirmed" && <form noValidate aria-busy={sending} onSubmit={submit} className="mt-4 space-y-4">
        <fieldset disabled={sending} className="grid gap-4 sm:grid-cols-2">
          <div>
            <label htmlFor="staff-invitation-email" className="block text-sm font-medium text-slate-700">Staff email address</label>
            <input id="staff-invitation-email" ref={emailInput} type="email" autoComplete="email" required maxLength={254}
              value={email} onChange={(event) => setEmail(event.target.value)}
              aria-invalid={feedback?.kind === "email"}
              aria-describedby={feedback?.kind === "email" ? "staff-invitation-feedback" : undefined}
              className={fieldClass} />
          </div>
          <div>
            <label htmlFor="staff-invitation-role" className="block text-sm font-medium text-slate-700">Staff role</label>
            <select id="staff-invitation-role" ref={roleInput} required value={role}
              onChange={(event) => setRole(event.target.value)}
              aria-invalid={feedback?.kind === "role"}
              aria-describedby={feedback?.kind === "role" ? "staff-invitation-feedback" : undefined}
              className={fieldClass}>
              <option value="">Choose a role</option>
              <option value="SCHOOL_ADMIN">School Administrator</option>
              <option value="PRINCIPAL">Principal</option>
            </select>
          </div>
        </fieldset>
        <button type="submit" disabled={sending}
          className="inline-flex min-h-11 w-full items-center justify-center rounded-lg bg-green-700 px-5 py-2.5 text-sm font-semibold text-white hover:bg-green-800 disabled:cursor-not-allowed disabled:opacity-60 sm:w-auto">
          {sending ? "Sending invitation…" : "Send invitation"}
        </button>
      </form>}
      {feedback && (
        <p id="staff-invitation-feedback" role={feedback.kind === "success" ? "status" : "alert"}
          className={feedback.kind === "success" ? "mt-3 text-sm text-green-800" : "mt-3 text-sm text-red-800"}>
          {feedback.message}
        </p>
      )}
    </section>
  );
}
