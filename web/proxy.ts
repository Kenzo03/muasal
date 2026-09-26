import { NextResponse, type NextRequest } from "next/server";

// Pages anyone may open: sign-in and password setup.
const open = /^\/(login|setup)(\/|$)/;

// Every page gets a nonce-based Content-Security-Policy (FSD §18.2): Next.js
// reads the nonce from the request's policy and puts it on its own scripts, so
// only those run. Styles allow inline style attributes, which React sets.
// Pages other than sign-in and setup also need a session cookie; the Go API
// decides whether it is valid.
export function proxy(request: NextRequest) {
  if (!open.test(request.nextUrl.pathname) && !request.cookies.has("sid")) {
    return NextResponse.redirect(new URL("/login", request.url));
  }
  const nonce = btoa(crypto.randomUUID());
  const csp = [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${process.env.NODE_ENV === "development" ? " 'unsafe-eval'" : ""}`,
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data: blob:",
    "font-src 'self'",
    "connect-src 'self'",
    "worker-src 'self' blob:", // pdf.js reads uploaded PDFs in a worker (FSD §7.7)
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
  ].join("; ");
  const headers = new Headers(request.headers);
  headers.set("x-nonce", nonce);
  headers.set("Content-Security-Policy", csp);
  const response = NextResponse.next({ request: { headers } });
  response.headers.set("Content-Security-Policy", csp);
  response.headers.set("X-Content-Type-Options", "nosniff");
  response.headers.set("Referrer-Policy", "same-origin");
  return response;
}

export const config = {
  // Every page; not the API, Next.js assets or prefetches (which carry no nonce).
  matcher: [
    {
      source: "/((?!api|_next/static|_next/image|favicon.ico).*)",
      missing: [
        { type: "header", key: "next-router-prefetch" },
        { type: "header", key: "purpose", value: "prefetch" },
      ],
    },
  ],
};
