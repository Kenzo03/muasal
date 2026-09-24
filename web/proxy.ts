import { NextResponse, type NextRequest } from "next/server";

// Only checks that a session cookie exists; the Go API decides whether it is valid.
export function proxy(request: NextRequest) {
  if (!request.cookies.has("sid")) {
    return NextResponse.redirect(new URL("/login", request.url));
  }
  return NextResponse.next();
}

export const config = {
  // Everything except sign-in, password setup, the API and Next.js assets.
  matcher: ["/((?!login|setup|api|_next|favicon.ico).*)"],
};
