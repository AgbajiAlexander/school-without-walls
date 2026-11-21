import { NextRequest, NextResponse } from "next/server";
import { getDocumentSecurityHeaders } from "./lib/security-headers.mjs";

export function middleware(request: NextRequest) {
  if (process.env.NODE_ENV !== "production") {
    return NextResponse.next();
  }

  const nonceBytes = crypto.getRandomValues(new Uint8Array(16));
  const nonce = btoa(String.fromCharCode(...nonceBytes));
  const documentSecurityHeaders = getDocumentSecurityHeaders(
    nonce,
    request.nextUrl.protocol,
    process.env.NODE_ENV,
    /^\/schools\/[^/]+\/teaching\/?$/.test(request.nextUrl.pathname) ? process.env.LESSON_MEDIA_BROWSER_ORIGIN : undefined,
  );
  const contentSecurityPolicy = documentSecurityHeaders.find(
    ({ key }) => key === "Content-Security-Policy",
  )?.value;
  if (!contentSecurityPolicy) {
    throw new Error("Content Security Policy was not configured");
  }
  const requestHeaders = new Headers(request.headers);
  requestHeaders.set("Content-Security-Policy", contentSecurityPolicy);

  const response = NextResponse.next({
    request: {
      headers: requestHeaders,
    },
  });
  for (const { key, value } of documentSecurityHeaders) {
    response.headers.set(key, value);
  }

  return response;
}

export const config = {
  matcher: [
    "/((?!api(?:/|$)|_next/static|_next/image|favicon.ico|robots.txt|sitemap.xml).*)",
  ],
};
