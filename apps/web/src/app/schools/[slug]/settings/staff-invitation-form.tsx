"use client";

import { useRef, useState, type FormEvent } from "react";
import { isStaffInvitationRole, sendStaffInvitation } from "@/lib/tenant-client";

const messages = {
  invalid: "Check the email address and role, then try again.",
  conflict: "This email already has a pending invitation or membership for that role.",
  unauthenticated: "Your session has expired. Sign in again to invite staff.",
  forbidden: "You do not have permission to invite staff for this school.",
  unconfirmed: "Invitation delivery could not be confirmed. Check your connection before trying again.",
};

export default function StaffInvitationForm({ schoolSlug }: { schoolSlug: string }) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("");
  const [sending, setSending] = useState(false);
  const [feedback, setFeedback] = useState<{ kind: "email" | "role" | "error" | "success"; message: string } | null>(null);
  const submitting = useRef(false);
  const emailInput = useRef<HTMLInputElement>(null);
  const roleInput = useRef<HTMLSelectElement>(null);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current) return;

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
      if (result === "sent") {
        setEmail("");
        setRole("");
        setFeedback({ kind: "success", message: "Invitation sent." });
      } else {
        setFeedback({ kind: "error", message: messages[result] });
      }
    } finally {
      submitting.current = false;
      setSending(false);
    }
  }

  const fieldClass = "mt-1.5 min-h-11 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm text-slate-900 focus:border-green-700 focus:outline-none focus:ring-2 focus:ring-green-700/20";

  return (
    <section id="invite-staff" aria-labelledby="invite-staff-heading" className="mt-6 scroll-mt-4 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-8">
      <h2 id="invite-staff-heading" className="text-lg font-semibold text-slate-900">Invite Staff</h2>
      <form noValidate aria-busy={sending} onSubmit={submit} className="mt-4 space-y-4">
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
      </form>
      {feedback && (
        <p id="staff-invitation-feedback" role={feedback.kind === "success" ? "status" : "alert"}
          className={feedback.kind === "success" ? "mt-3 text-sm text-green-800" : "mt-3 text-sm text-red-800"}>
          {feedback.message}
        </p>
      )}
    </section>
  );
}
