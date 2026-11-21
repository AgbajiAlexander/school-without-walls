"use client";

import { useRouter } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";

export default function EditionForm({ endpoint, csrf, stage, role, revision, currentID, editions }: {
  endpoint: string; csrf: string; stage: string; role: string; revision: number; currentID?: string;
  editions: { id: string; title: string; edition: string; publisher: string }[];
}) {
  const router = useRouter();
  const [edition, setEdition] = useState("");
  const [busy, setBusy] = useState(false);
  const [blockedRevision, setBlockedRevision] = useState<number>();
  const [denied, setDenied] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const inFlight = useRef(false);
  const blocked = denied || blockedRevision === revision;
  const choices = editions.filter((item) => item.id !== currentID);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (inFlight.current || blocked || !choices.some((item) => item.id === edition)) return;
    inFlight.current = true;
    setBusy(true);
    setNotice("");
    setError("");
    try {
      const response = await fetch(endpoint, {
        method: "POST", credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ stage, curriculum_role: role, framework_version_id: edition, expected_revision: revision }),
      });
      if (response.status === 201) {
        setEdition("");
        setBlockedRevision(revision);
        setNotice("Curriculum edition saved.");
        router.refresh();
      } else if ([401, 403, 404].includes(response.status)) {
        setDenied(true);
        setError("Selection access is unavailable. Refresh or sign in again.");
      } else if (response.status === 409) {
        setEdition("");
        setBlockedRevision(revision);
        setError("This selection changed or the session was archived. Check the refreshed selections.");
        router.refresh();
      } else if (response.status === 422) {
        setEdition("");
        setError("This edition is no longer eligible. Check the refreshed choices.");
        router.refresh();
      } else {
        setError("The save could not be confirmed. Refresh before trying again.");
      }
    } catch {
      setError("The save could not be confirmed. Refresh before trying again.");
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }

  return (
    <>
      {choices.length ? (
        <form onSubmit={save} className="space-y-2">
          <fieldset disabled={busy || blocked} className="space-y-2">
            <label>Framework edition<select required value={edition} onChange={(event) => setEdition(event.target.value)}
              className="block min-h-11 w-full rounded border px-2">
              <option value="">Select an edition</option>
              {choices.map((item) => <option key={item.id} value={item.id}>{item.title} · {item.edition} · {item.publisher}</option>)}
            </select></label>
            <button type="submit" disabled={!choices.some((item) => item.id === edition)} className="min-h-11 rounded bg-green-800 px-3 text-white disabled:opacity-50">
              {busy ? "Saving…" : "Save curriculum edition"}
            </button>
          </fieldset>
        </form>
      ) : <p>No other eligible editions on this page.</p>}
      {notice && <p role="status" className="text-green-800">{notice}</p>}
      {error && <p role="alert" className="text-red-800">{error}</p>}
    </>
  );
}
