// Deterministic on server and browser, so hydration matches. Timestamps show in
// the timezone of the user's profile (i18n/request.ts). Intl only turns them
// into that zone's numbers, which every runtime agrees on; the month names are
// ours, as Intl's differ between runtimes.
const months: Record<string, string[]> = {
  id: ["Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"],
  en: ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"],
};

// ponytail: one formatter per timezone for the life of the process; users pick from a few hundred zones at most.
const zones = new Map<string, Intl.DateTimeFormat>();

/** The date (2026-09-27) and time (14:02) a moment shows on a clock in timeZone. */
// Without a zone (none configured), UTC: the same on server and browser.
function inZone(at: string | Date, timeZone = "UTC") {
  let f = zones.get(timeZone);
  if (!f) {
    f = new Intl.DateTimeFormat("en-US", {
      timeZone, year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23",
    });
    zones.set(timeZone, f);
  }
  const p = Object.fromEntries(f.formatToParts(new Date(at)).map((x) => [x.type, x.value]));
  return { date: `${p.year}-${p.month}-${p.day}`, time: `${p.hour}:${p.minute}` };
}

/** A calendar date such as a due date (2026-09-27) as "27 Sep 2026"; without the year when withYear is false. */
export function day(date: string, locale: string, withYear = true) {
  const [y, m, d] = date.slice(0, 10).split("-");
  const text = `${Number(d)} ${(months[locale] ?? months.id)[Number(m) - 1]}`;
  return withYear ? `${text} ${y}` : text;
}

/** The day a timestamp falls on in timeZone, written as day() writes it. */
export function dayOf(iso: string, locale: string, timeZone: string | undefined, withYear = true) {
  return day(inZone(iso, timeZone).date, locale, withYear);
}

/** A timestamp as "27 Sep 2026, 21:02" in timeZone. */
export function dateTime(iso: string, locale: string, timeZone: string | undefined) {
  const { date, time } = inZone(iso, timeZone);
  return `${day(date, locale)}, ${time}`;
}

/** The date (2026-09-27) a moment falls on in timeZone; dateIn(new Date(), timeZone) is today. */
export function dateIn(at: string | Date, timeZone: string | undefined) {
  return inZone(at, timeZone).date;
}

/** An Ask item's day: a note's decision day as it is (it arrives as midnight UTC), a ticket's or document's timestamp in timeZone. */
export function itemDay(item: { kind: string; date: string }, locale: string, timeZone: string | undefined) {
  return item.kind === "note" ? day(item.date, locale) : dayOf(item.date, locale, timeZone);
}

export function fileSize(bytes: number) {
  const units = ["MB", "GB", "TB"];
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  let n = bytes / 1024 / 1024;
  let u = 0;
  while (n >= 1024 && u < units.length - 1) {
    n /= 1024;
    u++;
  }
  return `${n.toFixed(1)} ${units[u]}`;
}
