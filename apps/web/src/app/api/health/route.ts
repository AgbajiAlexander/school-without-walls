import { NextResponse } from "next/server";

export async function GET() {
  return NextResponse.json({
    data: {
      status: "healthy",
      service: "school-without-walls-web",
      timestamp: new Date().toISOString(),
    },
    meta: {
      request_id: crypto.randomUUID(),
    },
  });
}
