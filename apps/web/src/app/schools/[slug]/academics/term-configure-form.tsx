"use client";

import { useState, type FormEvent } from "react";
import {
  configureAcademicTerms,
  type AcademicSession,
  type AcademicTerm,
  type TermInput,
} from "@/lib/academics-client";

interface Props {
  schoolSlug: string;
  session: AcademicSession;
  existingTerms: AcademicTerm[];
  onSaved: (terms: AcademicTerm[]) => void;
}

const termNames = ["First Term", "Second Term", "Third Term"];

function initialTerms(existing: AcademicTerm[]): TermInput[] {
  return termNames.map((name, index) => {
    const saved = existing.find((term) => term.sequence === index + 1);
    return {
      sequence: index + 1,
      name: saved?.name ?? name,
      starts_on: saved?.starts_on ?? "",
      ends_on: saved?.ends_on ?? "",
    };
  });
}

function validateTerms(
  session: AcademicSession,
  terms: TermInput[],
): string | null {
  for (const term of terms) {
    if (!term.name.trim() || !term.starts_on || !term.ends_on) {
      return "Complete the name and dates for all three terms.";
    }

    if (term.starts_on > term.ends_on) {
      return `${term.name}: the end date cannot be before the start date.`;
    }

    if (
      term.starts_on < session.starts_on ||
      term.ends_on > session.ends_on
    ) {
      return `${term.name}: dates must fall within the academic session.`;
    }
  }

  for (let index = 1; index < terms.length; index += 1) {
    if (terms[index].starts_on <= terms[index - 1].ends_on) {
      return "Term dates must be ordered and must not overlap.";
    }
  }

  return null;
}

export default function TermConfigureForm({
  schoolSlug,
  session,
  existingTerms,
  onSaved,
}: Props) {
  const [terms, setTerms] = useState<TermInput[]>(() =>
    initialTerms(existingTerms),
  );
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function updateTerm(
    index: number,
    field: "name" | "starts_on" | "ends_on",
    value: string,
  ) {
    setTerms((current) =>
      current.map((term, position) =>
        position === index ? { ...term, [field]: value } : term,
      ),
    );
    setError(null);
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting) return;

    setError(null);

    const validated = terms.map((term) => ({
      ...term,
      name: term.name.trim(),
    }));
    const validationError = validateTerms(session, validated);

    if (validationError) {
      setError(validationError);
      return;
    }

    setSubmitting(true);

    try {
      const saved = await configureAcademicTerms(
        schoolSlug,
        session.id,
        validated,
      );
      onSaved(saved);
    } catch (cause: unknown) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to save academic terms.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  if (session.status !== "DRAFT") {
    return null;
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="mt-5 space-y-5 rounded-xl border border-slate-200 bg-white p-4"
    >
      <div>
        <h4 className="font-semibold text-slate-900">
          Configure three academic terms
        </h4>
        <p className="mt-1 text-sm text-slate-600">
          All terms must fall between {session.starts_on} and{" "}
          {session.ends_on}, without overlapping.
        </p>
      </div>

      {terms.map((term, index) => (
        <fieldset key={term.sequence} className="space-y-3 border-t pt-4">
          <legend className="font-semibold text-slate-800">
            {termNames[index]}
          </legend>

          <div>
            <label
              htmlFor={`term-name-${index}`}
              className="block text-sm font-medium"
            >
              {termNames[index]} name
            </label>
            <input
              id={`term-name-${index}`}
              required
              maxLength={80}
              value={term.name}
              onChange={(event) =>
                updateTerm(index, "name", event.target.value)
              }
              className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2"
            />
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <label
                htmlFor={`term-start-${index}`}
                className="block text-sm font-medium"
              >
                {termNames[index]} start date
              </label>
              <input
                id={`term-start-${index}`}
                type="date"
                required
                min={session.starts_on}
                max={session.ends_on}
                value={term.starts_on}
                onChange={(event) =>
                  updateTerm(index, "starts_on", event.target.value)
                }
                className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2"
              />
            </div>

            <div>
              <label
                htmlFor={`term-end-${index}`}
                className="block text-sm font-medium"
              >
                {termNames[index]} end date
              </label>
              <input
                id={`term-end-${index}`}
                type="date"
                required
                min={session.starts_on}
                max={session.ends_on}
                value={term.ends_on}
                onChange={(event) =>
                  updateTerm(index, "ends_on", event.target.value)
                }
                className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2"
              />
            </div>
          </div>
        </fieldset>
      ))}

      {error && (
        <p role="alert" className="text-sm font-medium text-red-700">
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={submitting}
        className="min-h-11 rounded-lg bg-green-700 px-5 py-2 text-sm font-semibold text-white disabled:opacity-60"
      >
        {submitting ? "Saving academic terms..." : "Save three terms"}
      </button>
    </form>
  );
}