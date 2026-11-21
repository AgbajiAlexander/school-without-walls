import Link from "next/link";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import ReviewControls, { type ReviewItem } from "./review-controls";

export default async function LessonReviews({ params, searchParams }: {
  params: Promise<{ slug: string }>;
  searchParams: Promise<{ after?: string }>;
}) {
  const { slug } = await params;
  const { after } = await searchParams;
  const cookie = (await headers()).get("cookie");
  if (!cookie) redirect("/login");
  const school = encodeURIComponent(slug);
  const root = `/schools/${school}/lesson-reviews`;
  const base = `/api/v1/tenants/${school}`;
  async function read<T>(path: string): Promise<T | null> {
    try {
      const result = await fetch(new URL(path, process.env.API_INTERNAL_URL || "http://localhost:8080"), {
        headers: { Accept: "application/json", Cookie: cookie ?? "" }, redirect: "error", cache: "no-store",
      });
      return result.ok ? (await result.json())?.data ?? null : null;
    } catch { return null; }
  }
  const [settings, session, queue] = await Promise.all([
    read<{can_manage_academics?: boolean; can_invite_staff?: boolean; can_manage_lesson_review?: boolean; feature_flags?: Record<string, boolean>}>(`${base}/settings`),
    read<{csrf_token: string}>("/api/v1/auth/me"),
    read<{items: ReviewItem[]; next?: string}>(`${base}/lesson-reviews${typeof after === "string" ? `?${new URLSearchParams({after})}` : ""}`),
  ]);
  return (
    <main className="mx-auto max-w-4xl space-y-5 px-4 py-8 sm:px-6">
      <Link href="/dashboard" className="text-green-800 underline">Back to My Schools</Link>
      <h1 className="text-2xl font-semibold">Lesson reviews</h1>
      {settings?.can_manage_academics === true && session?.csrf_token && Array.isArray(queue?.items) ? (
        <>
          <ReviewControls base={base} csrf={session.csrf_token} items={queue.items}
            required={settings.feature_flags?.lesson_review_required === true} canConfigure={settings.can_manage_lesson_review === true} />
          {queue.next && <Link className="text-green-800 underline" href={`${root}?${new URLSearchParams({after:queue.next})}`}>Next reviews</Link>}
        </>
      ) : <p role="status">Lesson reviews are unavailable for this account or school.</p>}
    </main>
  );
}
