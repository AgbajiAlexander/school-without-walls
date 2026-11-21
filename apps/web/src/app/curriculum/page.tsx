import Link from "next/link";
import { headers } from "next/headers";
import { redirect } from "next/navigation";

const stages = {
  EARLY_YEARS: "Early Years",
  PRIMARY: "Primary",
  JUNIOR_SECONDARY: "Junior Secondary",
  SENIOR_SECONDARY: "Senior Secondary",
};

const kinds: Record<string, string> = {
  NIGERIAN_PRIMARY: "Nigerian curriculum",
  ENRICHMENT: "Optional enrichment",
  DEVELOPMENTAL: "Developmental framework",
};

type Framework = {
  id: string;
  jurisdiction: string;
  publisher: string;
  title: string;
  edition: string;
  framework_kind: string;
  effective_from: string;
  effective_to: string | null;
  source_url: string;
  license_note: string;
};

type Objective = {
  id: string;
  stage: keyof typeof stages;
  grade_band: string;
  subject: string;
  strand: string;
  code: string;
  objective_text: string;
};

type Page<T> = { items: T[]; next?: string };
const root = "/curriculum";

function url(query: Record<string, string>): string {
  return `${root}?${new URLSearchParams(query)}`;
}

function safeSource(value: string) {
  try {
    const source = new URL(value);
    return source.protocol === "https:" && !source.username && !source.password ? source.href : undefined;
  } catch {
    return undefined;
  }
}

function Frameworks({ page }: { page: Page<Framework> }) {
  if (page.items.length === 0) return <p role="status">No published framework editions on this page.</p>;
  return (
    <ul className="grid gap-4 sm:grid-cols-2">
      {page.items.map((edition) => {
        const source = safeSource(edition.source_url);
        return (
          <li key={edition.id} className="space-y-2 rounded-xl border border-slate-200 bg-white p-4">
            <p className="text-sm font-medium text-green-800">{kinds[edition.framework_kind] ?? "Curriculum framework"}</p>
            <h2 className="text-lg font-semibold">{edition.title}</h2>
            <p>{edition.publisher} · {edition.jurisdiction} · {edition.edition}</p>
            <p>Effective from {edition.effective_from}{edition.effective_to ? ` to ${edition.effective_to}` : ""}</p>
            <p className="break-words text-sm">{edition.license_note}</p>
            <div className="flex flex-wrap gap-4">
              <Link className="py-2 text-green-800 underline" href={url({ framework_version_id: edition.id })}>
                Browse objectives <span className="sr-only">for {edition.title}, {edition.edition}</span>
              </Link>
              {source && <a className="py-2 text-green-800 underline" href={source}>Publisher source</a>}
            </div>
          </li>
        );
      })}
    </ul>
  );
}

function Objectives({ version, stage, page }: { version: string; stage: string; page: Page<Objective> }) {
  return (
    <>
      <form action={root} className="flex flex-wrap items-end gap-3">
        <input type="hidden" name="framework_version_id" value={version} />
        <label className="block">Education stage
          <select className="block min-h-11 rounded border border-slate-300 px-2" name="stage" defaultValue={stage}>
            <option value="">All stages</option>
            {Object.entries(stages).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </label>
        <button className="min-h-11 rounded bg-green-800 px-3 text-white" type="submit">Filter objectives</button>
      </form>
      {page.items.length === 0 ? <p role="status">No published objectives match this selection.</p> : (
        <ul className="space-y-3">
          {page.items.map((objective) => (
            <li key={objective.id} className="space-y-1 rounded-xl border border-slate-200 bg-white p-4">
              <h2 className="font-semibold">{objective.code}</h2>
              <p className="text-sm text-slate-700">{stages[objective.stage]} · {objective.grade_band} · {objective.subject} · {objective.strand}</p>
              <p className="break-words">{objective.objective_text}</p>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

export default async function CurriculumPage({ searchParams }: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const cookie = (await headers()).get("cookie");
  if (!cookie) redirect("/login");
  const query = await searchParams;
  const version = typeof query.framework_version_id === "string" ? query.framework_version_id : "";
  const stage = typeof query.stage === "string" ? query.stage : "";
  const after = typeof query.after === "string" ? query.after : "";
  const filters: Record<string, string> = version ? { framework_version_id: version, stage } : {};
  const upstream = new URLSearchParams(after ? { ...filters, after } : filters);
  const resource = version ? "objectives" : "frameworks";
  const apiBase = process.env.API_INTERNAL_URL || "http://localhost:8080";
  const response = await fetch(new URL(`/api/v1/curriculum/${resource}?${upstream}`, apiBase), {
    headers: { Accept: "application/json", Cookie: cookie },
    cache: "no-store",
    redirect: "error",
  }).catch(() => null);
  if (response?.status === 401) redirect("/login");
  const envelope = response?.ok ? await response.json().catch(() => null) : null;
  const page = Array.isArray(envelope?.data?.items) ? envelope.data as Page<Framework> | Page<Objective> : null;
  const next = typeof page?.next === "string" ? page.next : undefined;

  return (
    <main className="mx-auto w-full max-w-5xl space-y-5 px-4 py-8 sm:px-6">
      <Link href="/dashboard" className="text-green-800 underline">Back to My Schools</Link>
      <h1 className="text-2xl font-semibold">{version ? "Curriculum objectives" : "Curriculum frameworks"}</h1>
      {version && <Link href={root} className="inline-block py-2 text-green-800 underline">Back to frameworks</Link>}
      {page ? (
        version ? <Objectives version={version} stage={stage} page={page as Page<Objective>} />
          : <Frameworks page={page as Page<Framework>} />
      ) : (
        <p role="status">{response?.status === 403
          ? "An active school membership is required to browse curriculum."
          : response?.status === 422
            ? "This curriculum selection is invalid."
            : "Unable to load curriculum. Refresh the page to try again."}</p>
      )}
      {page && (after || next) && (
        <nav aria-label="Curriculum pages" className="flex gap-4">
          {after && <Link className="py-2 text-green-800 underline" href={url(filters)}>First page</Link>}
          {next && <Link className="py-2 text-green-800 underline" href={url({ ...filters, after: next })}>Next page</Link>}
        </nav>
      )}
    </main>
  );
}
