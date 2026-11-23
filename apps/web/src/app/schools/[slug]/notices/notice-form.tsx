"use client";

import { useRouter } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";

const roles = [
  ["SCHOOL_OWNER", "School owners"], ["SCHOOL_ADMIN", "School administrators"], ["PRINCIPAL", "Principals"],
  ["TEACHER", "Teachers"], ["BURSAR", "Bursars"], ["STUDENT", "Students"], ["PARENT", "Parents and guardians"],
];
const stages = [
  ["EARLY_YEARS", "Early years"], ["PRIMARY", "Primary"],
  ["JUNIOR_SECONDARY", "Junior secondary"], ["SENIOR_SECONDARY", "Senior secondary"],
];

export default function NoticeForm({ endpoint, csrf, classes }: {
  endpoint: string; csrf: string; classes: { id: string; name: string }[];
}) {
  const router = useRouter();
  const pending = useRef(false);
  const [audience, setAudience] = useState("SCHOOL");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  async function publish(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending.current) return;
    const form = event.currentTarget;
    const values = new FormData(form);
    pending.current = true;
    setBusy(true);
    setNotice("");
    setError("");
    try {
      const result = await fetch(endpoint, {
        method: "POST", credentials: "same-origin", cache: "no-store", redirect: "error",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ title: values.get("title"), body: values.get("body"), audience,
          role: values.get("role") ?? undefined, stage: values.get("stage") ?? undefined, class_id: values.get("class_id") ?? undefined }),
      });
      if (result.status === 422) {
        setError("Check the notice text and choose a valid audience.");
      } else if (!result.ok) {
        setError("The notice could not be published. Refresh to check your access.");
      } else {
        form.reset();
        setAudience("SCHOOL");
        setNotice("Notice published to the selected audience.");
        router.refresh();
      }
    } catch {
      setError("Publication could not be confirmed. Refresh before trying again.");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  return (
    <section className="space-y-3 rounded-xl border p-4">
      <h2 className="text-lg font-semibold">Publish a notice</h2>
      <form onSubmit={(event) => void publish(event)}>
        <fieldset disabled={busy} className="grid gap-3">
          <label>Notice title<input name="title" required maxLength={160} className="block min-h-11 w-full rounded border px-2" /></label>
          <label>Notice text<textarea name="body" required maxLength={4000} rows={5} className="block w-full rounded border p-2" /></label>
          <label>Audience<select value={audience} onChange={(event) => setAudience(event.target.value)} className="block min-h-11 w-full rounded border px-2">
            <option value="SCHOOL">Everyone in this school</option><option value="ROLE">Role</option>
            <option value="STAGE">Stage</option><option value="CLASS" disabled={!classes.length}>Class</option>
          </select></label>
          {audience === "ROLE" && <label>Target role<select name="role" required className="block min-h-11 w-full rounded border px-2">
            {roles.map(([id, label]) => <option value={id} key={id}>{label}</option>)}
          </select></label>}
          {audience === "STAGE" && <label>Target stage<select name="stage" required className="block min-h-11 w-full rounded border px-2">
            {stages.map(([id, label]) => <option value={id} key={id}>{label}</option>)}
          </select></label>}
          {audience === "CLASS" && <label>Target class<select name="class_id" required className="block min-h-11 w-full rounded border px-2">
            {classes.map((item) => <option value={item.id} key={item.id}>{item.name}</option>)}
          </select></label>}
          <button type="submit" className="min-h-11 justify-self-start rounded bg-green-800 px-3 text-white">{busy ? "Publishing..." : "Publish notice"}</button>
        </fieldset>
      </form>
      {notice && <p role="status">{notice}</p>}
      {error && <p role="alert">{error}</p>}
    </section>
  );
}
