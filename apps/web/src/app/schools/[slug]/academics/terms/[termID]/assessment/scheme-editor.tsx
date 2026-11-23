"use client";

import { useRouter } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";

export type Component = { name: string; max_score: string; weight: string };
type GradeBand = { label: string; min_bp: number; max_bp: number };
type BandDraft = { key: number; label: string; minimum: string; maximum: string };
export type Scheme = { id: string; stage: string; version: number; status: "DRAFT" | "PUBLISHED"; revision: number; components: Component[]; grade_bands?: GradeBand[] };
const stages = { PRIMARY: "Primary", JUNIOR_SECONDARY: "Junior secondary", SENIOR_SECONDARY: "Senior secondary" };
const empty = { name: "", max_score: "", weight: "" };

export default function SchemeEditor({ endpoint, csrf, archived, schemes }: {
  endpoint: string; csrf: string; archived: boolean; schemes: Scheme[];
}) {
  const router = useRouter();
  const pending = useRef(false);
  const nextKey = useRef(1);
  const [stage, setStage] = useState("PRIMARY");
  const [editing, setEditing] = useState<Scheme>();
  const [parts, setParts] = useState([{ ...empty, key: 0 }]);
  const [bands, setBands] = useState<BandDraft[]>([]);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [confirmation, setConfirmation] = useState<{ id: string; revision: number }>();
  const total = parts.reduce((sum, part) => sum + Math.round((Number(part.weight) || 0) * 100), 0);

  function change(key: number, field: keyof Component, value: string) {
    setParts((current) => current.map((part) => part.key === key ? { ...part, [field]: value } : part));
  }

  function updateBand(key: number, field: "label" | "minimum" | "maximum", value: string) {
    setBands((current) => current.map((band) => band.key === key ? { ...band, [field]: value } : band));
  }

  function reset() {
    setBands([]);
    setEditing(undefined);
    setParts([{ ...empty, key: nextKey.current++ }]);
  }

  async function write(path: string, method: "POST" | "PUT", body: object, success: string) {
    if (pending.current || archived) return;
    pending.current = true;
    setBusy(true);
    setMessage("");
    setError("");
    try {
      const response = await fetch(endpoint + path, {
        method, cache: "no-store", redirect: "error", credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify(body),
      });
      if (response.ok) {
        reset();
        setConfirmation(undefined);
        setMessage(success);
        router.refresh();
      } else if (response.status === 422) {
        setError("Check the components and grade bands. Bands cannot overlap, and publication requires weights totaling exactly 100%.");
      } else if (response.status === 409) {
        setError("The scheme changed or a draft already exists. Reload before trying again.");
      } else {
        setError("The scheme change could not be confirmed. Refresh to check your access.");
      }
    } catch {
      setError("The change could not be confirmed. Refresh before retrying.");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const components = parts.map(({ name, max_score, weight }) => ({ name, max_score, weight }));
    const grade_bands = bands.map((band) => ({ label: band.label, min_bp: Math.round(Number(band.minimum) * 100), max_bp: Math.round(Number(band.maximum) * 100) }));
    void write(editing ? "/" + encodeURIComponent(editing.id) : "", editing ? "PUT" : "POST",
      editing ? { revision: editing.revision, components, grade_bands } : { stage, components, grade_bands }, "Assessment draft saved.");
  }

  return (
    <div className="space-y-4">
      {!archived && <form onSubmit={save} className="space-y-3 rounded-xl border p-4">
        <h2 className="text-lg font-semibold">{editing ? "Edit assessment draft" : "New assessment draft"}</h2>
        <fieldset disabled={busy} className="space-y-3">
          {!editing && <label>Stage<select value={stage} onChange={(event) => setStage(event.target.value)} className="block min-h-11 w-full rounded border px-2">
            {Object.entries(stages).map(([key, label]) => <option key={key} value={key}>{label}</option>)}
          </select></label>}
          {parts.map((part, index) => (
            <fieldset key={part.key} className="grid gap-2 rounded border p-3 sm:grid-cols-3">
              <legend>Component {index + 1}</legend>
              <label>Component name<input required maxLength={100} value={part.name} onChange={(event) => change(part.key, "name", event.target.value)} className="block min-h-11 w-full rounded border px-2" /></label>
              <label>Maximum score<input required type="number" min="0.01" max="9999999.99" step="0.01" value={part.max_score} onChange={(event) => change(part.key, "max_score", event.target.value)} className="block min-h-11 w-full rounded border px-2" /></label>
              <label>Weight (%)<input required type="number" min="0.01" max="100" step="0.01" value={part.weight} onChange={(event) => change(part.key, "weight", event.target.value)} className="block min-h-11 w-full rounded border px-2" /></label>
              {parts.length > 1 && <button type="button" className="min-h-11 justify-self-start underline" onClick={() => setParts(parts.filter((other) => other.key !== part.key))}>Remove component</button>}
            </fieldset>
          ))}
          <fieldset className="space-y-2 rounded border p-3">
            <legend className="font-medium">Grade bands</legend>
            <p className="text-sm">Optional. Minimum and maximum percentages are inclusive and bands cannot overlap.</p>
            {bands.map((band, index) => (
              <fieldset key={band.key} className="grid gap-2 sm:grid-cols-3">
                <legend>Grade band {index + 1}</legend>
                <label>Grade label<input required maxLength={40} value={band.label} onChange={(event) => updateBand(band.key, "label", event.target.value)}
                  className="block min-h-11 w-full rounded border px-2" /></label>
                <label>Minimum (%)<input required type="number" min={0} max={100} step="0.01" value={band.minimum}
                  onChange={(event) => updateBand(band.key, "minimum", event.target.value)} className="block min-h-11 w-full rounded border px-2" /></label>
                <label>Maximum (%)<input required type="number" min={0} max={100} step="0.01" value={band.maximum}
                  onChange={(event) => updateBand(band.key, "maximum", event.target.value)} className="block min-h-11 w-full rounded border px-2" /></label>
                <button type="button" className="min-h-11 justify-self-start underline" onClick={() => setBands(bands.filter((item) => item.key !== band.key))}>Remove grade band</button>
              </fieldset>
            ))}
            <button type="button" disabled={bands.length >= 30} className="min-h-11 underline"
              onClick={() => setBands([...bands, { key: nextKey.current++, label: "", minimum: "", maximum: "" }])}>Add grade band</button>
          </fieldset>
          <p>Draft weight total: {(total / 100).toFixed(2)}%</p>
          <div className="flex flex-wrap gap-3">
            <button type="button" disabled={parts.length >= 30} className="min-h-11 underline"
              onClick={() => setParts([...parts, { ...empty, key: nextKey.current++ }])}>Add component</button>
            <button type="submit" className="min-h-11 rounded bg-green-800 px-3 text-white">Save assessment draft</button>
            {editing && <button type="button" onClick={reset} className="min-h-11 underline">Cancel edit</button>}
          </div>
        </fieldset>
      </form>}
      {message && <p role="status">{message}</p>}
      {error && <p role="alert">{error}</p>}
      <h2 className="text-lg font-semibold">Saved schemes</h2>
      {!schemes.length && <p>No assessment schemes have been saved.</p>}
      {schemes.map((item) => {
        const label = stages[item.stage as keyof typeof stages] ?? item.stage;
        const path = "/" + encodeURIComponent(item.id);
        const savedTotal = item.components.reduce((sum, part) => sum + Math.round(Number(part.weight) * 100), 0);
        return (
          <article key={item.id} className="space-y-2 rounded-xl border p-4">
            <h3 className="font-semibold">{label} version {item.version}</h3>
            <p>{item.status === "PUBLISHED" ? "Published" : "Draft"} · {(savedTotal / 100).toFixed(2)}%</p>
            <ul>{item.components.map((part) => <li key={part.name} className="break-words">{part.name} · {part.weight}% · Maximum score {part.max_score}</li>)}</ul>
            {!!item.grade_bands?.length && <ul aria-label="Saved grade bands">
              {item.grade_bands.map((band) => <li key={band.label}>{band.label}: {(band.min_bp / 100).toFixed(2)}% to {(band.max_bp / 100).toFixed(2)}%</li>)}
            </ul>}
            {!archived && item.status === "DRAFT" && <div className="flex flex-wrap items-center gap-3">
              <button type="button" disabled={busy} className="min-h-11 underline" onClick={() => {
                setEditing(item);
                setBands((item.grade_bands ?? []).map((band) => ({
                  key: nextKey.current++, label: band.label,
                  minimum: (band.min_bp / 100).toFixed(2), maximum: (band.max_bp / 100).toFixed(2),
                })));
                setParts(item.components.map((part) => ({ ...part, key: nextKey.current++ })));
              }}>Edit assessment draft</button>
              {confirmation?.id === item.id && confirmation.revision === item.revision ? <button type="button" disabled={busy || savedTotal !== 10000 || editing?.id === item.id}
                className="min-h-11 rounded bg-green-800 px-3 text-white"
                onClick={() => void write(path + "/publish", "POST", { revision: item.revision }, "Assessment scheme published.")}>Confirm publication</button>
                : <button type="button" disabled={busy || savedTotal !== 10000 || editing?.id === item.id}
                  className="min-h-11 underline" onClick={() => setConfirmation({ id: item.id, revision: item.revision })}>Publish scheme</button>}
              {confirmation?.id === item.id && confirmation.revision === item.revision && <button type="button" className="min-h-11 underline" onClick={() => setConfirmation(undefined)}>Cancel</button>}
            </div>}
          </article>
        );
      })}
    </div>
  );
}
