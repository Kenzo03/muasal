// Project lists grow long in a busy install, so every picker puts the
// projects opened last first and filters by name or key.

/** The projects opened last come first, most recent first; the rest keep their order. */
export function recentFirst<T extends { key: string }>(projects: T[], recent: string[]): T[] {
  const rank = (key: string) => (recent.includes(key) ? recent.indexOf(key) : recent.length);
  return projects
    .map((p, i) => ({ p, i }))
    .sort((a, b) => rank(a.p.key) - rank(b.p.key) || a.i - b.i)
    .map(({ p }) => p);
}

/** The projects whose name or key holds q, ignoring case; all of them for an empty q. */
export function matchProjects<T extends { key: string; name: string }>(projects: T[], q: string): T[] {
  const s = q.trim().toLowerCase();
  return s ? projects.filter((p) => p.name.toLowerCase().includes(s) || p.key.toLowerCase().includes(s)) : projects;
}
