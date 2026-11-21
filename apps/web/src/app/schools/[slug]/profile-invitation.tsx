"use client";

import { useId, useRef, useState, type FormEvent } from "react";

export default function ProfileInvitation({ slug, profileID, kind, name, csrf }: {
  slug: string;
  profileID: string;
  kind: "students" | "guardians";
  name: string;
  csrf: string;
}) {
  const emailID = useId();
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [status, setStatus] = useState<"idle" | "sending" | "sent" | "denied">("idle");
  const [error, setError] = useState("");
  const sending = useRef(false);

  async function send(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (sending.current || status === "sent" || status === "denied") return;
    sending.current = true;
    setStatus("sending");
    setError("");
    try {
      const response = await fetch(`/api/v1/tenants/${encodeURIComponent(slug)}/${kind}/${encodeURIComponent(profileID)}/invitations`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify(kind === "students" ? { email: email.trim() } : {}),
      });
      if (response.status === 201) {
        setStatus("sent");
        setEmail("");
      } else if ([401, 403].includes(response.status)) {
        setStatus("denied");
        setError("Invitation access is unavailable. Refresh or sign in again.");
      } else {
        setStatus("idle");
        switch (response.status) {
          case 404: setError("This profile is no longer available for invitation."); break;
          case 409: setError("An invitation or account already exists."); break;
          case 422: setError("Check that the recipient email is valid."); break;
          case 429: setError("Invitation limit reached. Try again later."); break;
          default: setError("Invitation delivery could not be confirmed.");
        }
      }
    } catch {
      setStatus("idle");
      setError("Invitation delivery could not be confirmed.");
    } finally {
      sending.current = false;
    }
  }

  if (!open) {
    return <button type="button" className="min-h-11 px-2 text-green-800 underline" onClick={() => setOpen(true)}>
      Invite <span className="sr-only">{name}</span>
    </button>;
  }

  return (
    <form aria-label={`Invite ${name}`} onSubmit={send} className="w-full space-y-2 rounded-lg border border-slate-200 p-3">
      {status === "sent" ? <p role="status">Invitation sent.</p> : (
        <fieldset disabled={status === "sending" || status === "denied"} className="space-y-2">
          {kind === "students" ? (
            <div>
              <label htmlFor={emailID}>Student account email</label>
              <input id={emailID} type="email" required maxLength={254} value={email}
                onChange={(event) => setEmail(event.target.value)}
                className="block min-h-11 w-full rounded border border-slate-300 px-3" />
            </div>
          ) : <p>Send an account invitation to the guardian’s saved contact email.</p>}
          <button type="submit" className="min-h-11 rounded bg-green-800 px-3 text-white disabled:opacity-50">
            {status === "sending" ? "Sending…" : "Send account invitation"}
          </button>
        </fieldset>
      )}
      {error && <p role="alert" className="text-red-800">{error}</p>}
      {status === "idle" && <button type="button" className="min-h-11 px-2 underline" onClick={() => {
        setEmail("");
        setError("");
        setOpen(false);
      }}>Cancel</button>}
    </form>
  );
}
