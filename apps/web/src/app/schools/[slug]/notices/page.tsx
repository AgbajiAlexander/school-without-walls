import Link from "next/link";
import { readLearningApi } from "../learning/lesson-api";
import NoticeForm from "./notice-form";

type Notice = { id: string; title: string; body: string; published_at: string };

export default async function SchoolNoticesPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const base = "/api/v1/tenants/" + encodeURIComponent(slug);
  const endpoint = base + "/notices";
  const [settings, feed] = await Promise.all([
    readLearningApi<{ can_manage_academics?: boolean }>(base + "/settings"),
    readLearningApi<{ items: Notice[] }>(endpoint),
  ]);
  const [session, classes] = settings?.can_manage_academics ? await Promise.all([
    readLearningApi<{ csrf_token: string }>("/api/v1/auth/me"),
    readLearningApi<{ items: { id: string; name: string }[] }>(endpoint + "/classes"),
  ]) : [null, null];

  return (
    <main className="mx-auto max-w-3xl space-y-5 px-4 py-8 sm:px-6">
      <Link href="/dashboard" className="text-green-800 underline">Back to My Schools</Link>
      <h1 className="text-2xl font-semibold">School notices</h1>
      {session?.csrf_token && <NoticeForm endpoint={endpoint} csrf={session.csrf_token} classes={classes?.items ?? []} />}
      {!settings || !feed ? <p role="status">School notices are unavailable.</p>
        : !feed.items.length ? <p role="status">No notices for your audience.</p>
        : <ul className="space-y-3">{feed.items.map((notice) => (
          <li key={notice.id} className="space-y-2 break-words rounded-xl border p-4">
            <h2 className="text-lg font-semibold">{notice.title}</h2>
            <time dateTime={notice.published_at} className="text-sm text-slate-700">{notice.published_at.slice(0, 10)}</time>
            <p className="whitespace-pre-wrap">{notice.body}</p>
          </li>
        ))}</ul>}
    </main>
  );
}
