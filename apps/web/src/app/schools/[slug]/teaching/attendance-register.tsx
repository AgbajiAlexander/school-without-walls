"use client";

import { useRouter } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";

export type RosterItem = { id: string; display_name: string; status?: string; mode?: string; updated_at?: string };

export default function AttendanceRegister({ endpoint, date, csrf, items }: {
  endpoint: string; date: string; csrf: string; items: RosterItem[];
}) {
  const router = useRouter();
  const pending = useRef(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending.current) return;
    setError("");
    setNotice("");
    const data = new FormData(event.currentTarget);
    const marks = items.flatMap((item) => {
      const status = String(data.get(item.id + "-status") ?? "");
      const mode = String(data.get(item.id + "-mode") ?? "");
      const reason = String(data.get(item.id + "-reason") ?? "").trim();
      if ((!status && !mode) || (status === item.status && mode === item.mode)) return [];
      return [{ student_id: item.id, status, mode, reason, expected_updated_at: item.updated_at }];
    });
    if (!marks.length) {
      setNotice("No attendance changes to save.");
      return;
    }
    if (marks.some((mark) => !mark.status || !mark.mode || (mark.expected_updated_at && !mark.reason))) {
      setError("Choose a status and mode for each mark, and enter a reason for every correction.");
      return;
    }
    pending.current = true;
    setBusy(true);
    try {
      const result = await fetch(endpoint, {
        method: "PUT", cache: "no-store", credentials: "same-origin", redirect: "error",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ date, marks }),
      });
      if (result.status === 409) {
        setError("Attendance changed. Reload the roster before saving.");
      } else if (!result.ok) {
        setError("Attendance could not be saved. Check the roster and your access.");
      } else {
        setNotice("Attendance saved.");
        router.refresh();
      }
    } catch {
      setError("Attendance could not be confirmed. Reload the roster to check the saved marks.");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  return (
    <form onSubmit={(event) => void save(event)} className="space-y-3">
      <fieldset disabled={busy} className="space-y-3">
        <legend className="sr-only">Attendance marks</legend>
        <ul className="space-y-3">{items.map((item) => (
          <li key={item.id + (item.updated_at ?? "")} className="space-y-2 break-words rounded border p-3">
            <h3 className="font-semibold">{item.display_name}</h3>
            <p>{item.status ? item.status.toLowerCase() : "Not recorded"}{item.mode === "REMOTE" ? " (Remote)" : item.mode === "ON_SITE" ? " (On site)" : ""}</p>
            <div className="grid gap-2 sm:grid-cols-2">
              <label>Status<select name={item.id + "-status"} defaultValue={item.status ?? ""} className="block min-h-11 w-full rounded border px-2">
                <option value="" disabled>Choose status</option><option value="PRESENT">Present</option>
                <option value="ABSENT">Absent</option><option value="LATE">Late</option><option value="EXCUSED">Excused</option>
              </select></label>
              <label>Mode<select name={item.id + "-mode"} defaultValue={item.mode ?? ""} className="block min-h-11 w-full rounded border px-2">
                <option value="" disabled>Choose mode</option><option value="ON_SITE">On site</option><option value="REMOTE">Remote</option>
              </select></label>
            </div>
            {item.updated_at && <label>Correction reason<input name={item.id + "-reason"} maxLength={500}
              className="block min-h-11 w-full rounded border px-2" /></label>}
          </li>
        ))}</ul>
        <button type="submit" className="min-h-11 rounded bg-green-800 px-3 text-white">{busy ? "Saving..." : "Save attendance"}</button>
      </fieldset>
      {notice && <p role="status">{notice}</p>}
      {error && <p role="alert">{error}</p>}
    </form>
  );
}
