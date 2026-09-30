// Project lists grow long in a busy install, so every picker puts the
// projects opened last first and filters by name or key.

/** The projects still in use: pickers, Home and short lists leave archived ones out (MSL-64). */
export function active<T extends { archived_at?: string }>(projects: T[]): T[] {
  return projects.filter((p) => !p.archived_at);
}

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

/**
 * A key for a new project's name, suggested while the key is untouched
 * (MSL-36): an acronym in the name ("Arunika DMS" → DMS), else the words'
 * initials, else the one word's first four letters; "" when none fits.
 */
export function suggestKey(name: string): string {
  const words = name.normalize("NFD").replace(/[^A-Za-z0-9]+/g, " ").trim().split(" ").filter(Boolean);
  const acronym = words.filter((w) => /^[A-Z][A-Z0-9]{2,9}$/.test(w)).pop();
  const key = (acronym ?? (words.length > 1 ? words.map((w) => w[0]).join("") : (words[0] ?? "").slice(0, 4))).toUpperCase();
  const k = key.replace(/^[0-9]+/, "").slice(0, 10);
  return k.length >= 2 ? k : "";
}
