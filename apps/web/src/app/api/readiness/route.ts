import { NextResponse } from "next/server";
import { checkApiReadiness } from "@/lib/api-readiness";

export const dynamic = "force-dynamic";

export async function GET() {
  const state = await checkApiReadiness(
    process.env.API_INTERNAL_URL || "http://localhost:8080",
  );
  const status = state === "ready" ? 200 : state === "timeout" ? 504 : 503;

  return NextResponse.json(
    { status: state },
    {
      status,
      headers: { "Cache-Control": "no-store, max-age=0" },
    },
  );
}
