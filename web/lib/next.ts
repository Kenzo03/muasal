/** Where to go after sign-in: a path on this site, or home. Resolved first, as browsers do (they strip tabs and newlines, and read `\` as `/`), so `//x` in any disguise is refused. */
export function safeNext(next?: string): string {
  if (!next?.startsWith("/")) return "/";
  const u = new URL(next, "http://x");
  return u.origin === "http://x" ? u.pathname + u.search + u.hash : "/";
}
