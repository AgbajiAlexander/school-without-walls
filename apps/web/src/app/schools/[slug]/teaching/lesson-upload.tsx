"use client";

import { useRef, useState, type FormEvent } from "react";

const mediaTypes = new Set(["application/pdf", "image/png", "image/jpeg"]);
const maxBytes = 8 * 1024 * 1024;

export default function LessonUpload({ endpoint, csrf, origin, disabled, onBusy, onAttached }: {
  endpoint: string;
  csrf: string;
  origin: string;
  disabled: boolean;
  onBusy: (busy: boolean) => void;
  onAttached: () => void;
}) {
  const pending = useRef(false);
  const attachment = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  async function upload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (disabled || pending.current) return;
    const form = event.currentTarget;
    const file = attachment.current?.files?.[0];
    setMessage("");
    setError("");
    if (!(file instanceof File) || !mediaTypes.has(file.type) || file.size < 1 || file.size > maxBytes) {
      setError("Choose a PDF, PNG or JPEG file up to 8 MiB.");
      return;
    }
    pending.current = true;
    setBusy(true);
    onBusy(true);
    const request = {
      method: "POST",
      credentials: "same-origin",
      redirect: "error",
      cache: "no-store",
      headers: { "X-CSRF-Token": csrf },
    } as const;
    try {
      const digest = new Uint8Array(await crypto.subtle.digest("SHA-256", await file.arrayBuffer()));
      const checksum = btoa(String.fromCharCode(...digest));
      const response = await fetch(endpoint, {
        ...request,
        headers: { ...request.headers, "Content-Type": "application/json" },
        body: JSON.stringify({ media_type: file.type, size_bytes: file.size, checksum_sha256: checksum }),
      });
      if (!response.ok) throw new Error();
      const data = (await response.json())?.data;
      if (typeof data?.id !== "string" || typeof data?.upload?.url !== "string") throw new Error();
      const target = new URL(data.upload.url);
      if (target.origin !== origin || target.username || target.password) throw new Error();

      const storageHeaders = new Headers({ "Content-Type": file.type });
      if (target.searchParams.get("X-Amz-SignedHeaders")?.split(";").includes("x-amz-checksum-sha256")) {
        storageHeaders.set("x-amz-checksum-sha256", checksum);
      }
      const uploaded = await fetch(target, {
        method: "PUT",
        credentials: "omit",
        redirect: "error",
        referrerPolicy: "no-referrer",
        headers: storageHeaders,
        body: file,
      });
      if (!uploaded.ok) throw new Error();
      const completed = await fetch(endpoint + "/" + encodeURIComponent(data.id) + "/complete", request);
      if (completed.status === 422) {
        setError("The attachment did not pass file validation or scanning.");
        return;
      }
      if (!completed.ok || (await completed.json())?.data?.status !== "READY") throw new Error();
      form.reset();
      setMessage("Attachment scanned and added to this draft.");
      onAttached();
    } catch {
      setError("The upload could not be confirmed. Reload the draft before trying again.");
    } finally {
      pending.current = false;
      setBusy(false);
      onBusy(false);
    }
  }

  return (
    <form onSubmit={(event) => void upload(event)} className="space-y-2 border-t pt-3">
      <fieldset disabled={busy || disabled} className="space-y-2">
        <label className="block font-medium">
          Lesson attachment
          <input ref={attachment} type="file" name="attachment" required accept="application/pdf,image/png,image/jpeg"
            className="block min-h-11 w-full max-w-full text-sm" />
        </label>
        <p className="text-sm">PDF, PNG or JPEG, up to 8 MiB. A successful upload replaces the attachment on this draft.</p>
        <button type="submit" className="min-h-11 rounded border px-3">
          {busy ? "Uploading and scanning..." : "Upload attachment"}
        </button>
      </fieldset>
      {message && <p role="status" className="text-green-800">{message}</p>}
      {error && <p role="alert" className="text-red-800">{error}</p>}
    </form>
  );
}
