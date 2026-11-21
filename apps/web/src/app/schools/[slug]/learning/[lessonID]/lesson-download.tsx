"use client";

import { useRef, useState } from "react";

export default function LessonDownload({ endpoint }: { endpoint: string }) {
  const pending = useRef(false);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);

  async function download() {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setFailed(false);
    try {
      const response = await fetch(endpoint, {
        cache: "no-store",
        credentials: "same-origin",
        redirect: "error",
      });
      if (!response.ok) throw new Error();
      const payload = await response.json();
      const value: unknown = payload?.data?.download?.url;
      if (typeof value !== "string") throw new Error();
      const url = new URL(value);
      const loopback = url.protocol === "http:" && (/^127(?:\.\d{1,3}){3}$/.test(url.hostname) || url.hostname === "[::1]");
      if ((!loopback && url.protocol !== "https:") || url.username || url.password || url.hostname === window.location.hostname) throw new Error();

      const link = document.createElement("a");
      link.href = url.href;
      link.rel = "noopener noreferrer";
      document.body.append(link);
      try {
        link.click();
      } finally {
        link.remove();
      }
    } catch {
      setFailed(true);
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  return (
    <section className="space-y-2 border-t pt-3">
      <button type="button" disabled={busy} onClick={() => void download()} className="min-h-11 text-green-800 underline">
        {busy ? "Preparing download..." : "Download lesson attachment"}
      </button>
      {failed && <p role="alert">The attachment is unavailable. Try again later.</p>}
    </section>
  );
}
