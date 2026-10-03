/** Where to go after sign-in: a path on this site, or home. `//x` and `/\x` would leave the site. */
export function safeNext(next?: string): string {
  return next && next.startsWith("/") && !next.startsWith("//") && !next.startsWith("/\\") ? next : "/";
}
