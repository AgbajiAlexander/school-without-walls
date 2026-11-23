"use client";

import { useRouter } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";
import type { Tenant } from "@/lib/tenant-client";

export default function SchoolModeForm({ tenant, csrf }: { tenant: Tenant; csrf: string }) {
  const router = useRouter();
  const schedule = tenant.operating_schedule;
  const [mode, setMode] = useState(schedule?.mode ?? "ON_SITE");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const pending = useRef(false);
  const blocked = useRef(false);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending.current || blocked.current) return;
    setNotice("");
    setError("");
    const data = new FormData(event.currentTarget);
    let payload: object = { mode };
    if (mode !== "ON_SITE") {
      const starts = new Date(String(data.get("starts")) + "Z");
      const ends = new Date(String(data.get("ends")) + "Z");
      if (!Number.isFinite(starts.valueOf()) || !Number.isFinite(ends.valueOf()) || starts >= ends) {
        setError("Choose an end time after the start time.");
        return;
      }
      payload = { mode, starts_at: starts.toISOString(), ends_at: ends.toISOString(),
        reason: String(data.get("reason") ?? "").trim(), help: String(data.get("help") ?? "").trim() || undefined,
        schedule_url: String(data.get("schedule") ?? "").trim() || undefined };
    }
    pending.current = true;
    setBusy(true);
    try {
      const result = await fetch("/api/v1/tenants/" + encodeURIComponent(tenant.slug) + "/school/mode", {
        method: "PATCH", cache: "no-store", credentials: "same-origin", redirect: "error",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify(payload),
      });
      if (!result.ok) {
        if ([401, 403, 404].includes(result.status)) {
          blocked.current = true;
          router.refresh();
        }
        setError(result.status === 422 ? "Check the reason, date range and HTTPS schedule link."
          : "The school mode change could not be confirmed. Refresh to check your access.");
        return;
      }
      setNotice("School operating mode saved.");
      router.refresh();
    } catch {
      setError("The school mode change could not be confirmed. Refresh before retrying.");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  return (
    <section className="space-y-3 rounded-xl border p-4">
      <h2 className="text-lg font-semibold">School operating mode</h2>
      <p>Current mode: {tenant.effective_mode === "REMOTE" ? "Remote" : tenant.effective_mode === "HYBRID" ? "Hybrid" : "On site"}</p>
      <form onSubmit={(event) => void save(event)}>
        <fieldset disabled={busy || blocked.current} className="grid gap-3">
          <label>Operating mode<select value={mode} onChange={(event) => setMode(event.target.value as typeof mode)}
            className="block min-h-11 w-full rounded border px-2">
            <option value="ON_SITE">On site</option><option value="HYBRID">Hybrid</option><option value="REMOTE">Remote</option>
          </select></label>
          {mode !== "ON_SITE" && (
            <>
              <label>Starts (UTC)<input type="datetime-local" name="starts" required defaultValue={schedule?.starts_at ? new Date(schedule.starts_at).toISOString().slice(0, 16) : undefined}
                className="block min-h-11 w-full rounded border px-2" /></label>
              <label>Ends (UTC)<input type="datetime-local" name="ends" required defaultValue={schedule?.ends_at ? new Date(schedule.ends_at).toISOString().slice(0, 16) : undefined}
                className="block min-h-11 w-full rounded border px-2" /></label>
              <label>Reason<input name="reason" required maxLength={500} defaultValue={schedule?.reason} className="block min-h-11 w-full rounded border px-2" /></label>
              <label>Where to get help<input name="help" maxLength={1000} defaultValue={schedule?.help} className="block min-h-11 w-full rounded border px-2" /></label>
              <label>Schedule link<input name="schedule" type="url" maxLength={2048} defaultValue={schedule?.schedule_url} className="block min-h-11 w-full rounded border px-2" /></label>
            </>
          )}
          <button type="submit" className="min-h-11 justify-self-start rounded bg-green-800 px-3 text-white">
            {busy ? "Saving..." : "Save operating mode"}
          </button>
        </fieldset>
      </form>
      {notice && <p role="status">{notice}</p>}
      {error && <p role="alert">{error}</p>}
    </section>
  );
}
