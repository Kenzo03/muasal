// Deterministic on server and browser, so hydration matches: no Intl, whose
// month names differ between runtimes. Times stay in UTC until profile
// timezones arrive in a later iteration.
const months: Record<string, string[]> = {
  id: ["Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"],
  en: ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"],
};

/** A date (2026-09-27, or the date part of a timestamp) as "27 Sep 2026"; without the year when withYear is false. */
export function day(iso: string, locale: string, withYear = true) {
  const [y, m, d] = iso.slice(0, 10).split("-");
  const text = `${Number(d)} ${(months[locale] ?? months.id)[Number(m) - 1]}`;
  return withYear ? `${text} ${y}` : text;
}

/** A timestamp as "27 Sep 2026, 14:02 UTC". */
export function utc(iso: string, locale: string) {
  return `${day(iso, locale)}, ${iso.slice(11, 16)} UTC`;
}

export function fileSize(bytes: number) {
  return bytes < 1024 * 1024 ? `${Math.max(1, Math.round(bytes / 1024))} KB` : `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}
