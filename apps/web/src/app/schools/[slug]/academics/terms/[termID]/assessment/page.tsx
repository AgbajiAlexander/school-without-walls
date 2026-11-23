import Link from "next/link";
import { readLearningApi, readLessonSession } from "../../../../learning/lesson-api";
import SchemeEditor, { type Scheme } from "./scheme-editor";

export default async function AssessmentPage({ params }: { params: Promise<{ slug: string; termID: string }> }) {
  const { slug, termID } = await params;
  const endpoint = "/api/v1/tenants/" + encodeURIComponent(slug) + "/terms/" + encodeURIComponent(termID) + "/assessment-schemes";
  const [schemes, session] = await Promise.all([
    readLearningApi<{ term: string; archived: boolean; items: Scheme[] }>(endpoint),
    readLessonSession(),
  ]);

  return (
    <main className="mx-auto max-w-4xl space-y-4 px-4 py-8 sm:px-6">
      <Link href={"/schools/" + encodeURIComponent(slug) + "/academics"} className="text-green-800 underline">Back to Academics</Link>
      <h1 className="text-2xl font-semibold">Assessment schemes</h1>
      {schemes && session?.csrf_token ? (
        <>
          <p>{schemes.term}{schemes.archived ? " (archived)" : ""}</p>
          <SchemeEditor endpoint={endpoint} csrf={session.csrf_token} archived={schemes.archived} schemes={schemes.items} />
        </>
      ) : <p role="status">Assessment schemes are unavailable for this term or account.</p>}
    </main>
  );
}
