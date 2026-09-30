// MSL-11: a decision note's action items, from its "Tindak lanjut" (or
// "Action items", "Follow-up") section, one list item each:
// "- Bayu: rancang penanda, paling lambat 10 Oktober 2026" gives the owner
// Bayu, the task, and the due date 2026-10-10.

export type ActionItem = { owner?: string; task: string; due?: string };

const heading = /^#{1,6}\s*(tindak\s+lanjut|action\s+items?|follow[\s-]?ups?|to[\s-]?dos?)\b/i;
const item = /^\s*(?:[-*+]|\d+[.)])\s+(.+)$/;
const months: Record<string, number> = {
  jan: 1, januari: 1, january: 1, feb: 2, februari: 2, february: 2, mar: 3, maret: 3, march: 3, apr: 4, april: 4,
  mei: 5, may: 5, jun: 6, juni: 6, june: 6, jul: 7, juli: 7, july: 7, agu: 8, agt: 8, agustus: 8, aug: 8, august: 8,
  sep: 9, sept: 9, september: 9, okt: 10, oktober: 10, oct: 10, october: 10, nov: 11, november: 11, des: 12, desember: 12, dec: 12, december: 12,
};
const monthNames = Object.keys(months).sort((a, b) => b.length - a.length).join("|");
const dayMonth = new RegExp(`\\b(\\d{1,2})\\s+(${monthNames})\\.?(?:\\s+(\\d{4}))?\\b`, "gi");
const iso = /\b(\d{4})-(\d{2})-(\d{2})\b/g;
const slashed = /\b(\d{1,2})[/.](\d{1,2})[/.](\d{4})\b/g;
// The phrase before a due date, left out of the task.
const cue = /[,;(]?\s*(?:paling\s+lambat|selambatnya|sebelum|tenggat|deadline|due|by)\s*[:]?\s*$/i;

const pad = (n: number) => String(n).padStart(2, "0");

// The last date in text, as YYYY-MM-DD; a date without a year takes year.
export function dueIn(text: string, year: number): { due: string; at: number } | undefined {
  let found: { due: string; at: number } | undefined;
  const keep = (y: number, m: number, d: number, at: number) => {
    if (m >= 1 && m <= 12 && d >= 1 && d <= 31 && (!found || at > found.at)) found = { due: `${y}-${pad(m)}-${pad(d)}`, at };
  };
  for (const m of text.matchAll(dayMonth)) keep(m[3] ? Number(m[3]) : year, months[m[2].toLowerCase()], Number(m[1]), m.index ?? 0);
  for (const m of text.matchAll(iso)) keep(Number(m[1]), Number(m[2]), Number(m[3]), m.index ?? 0);
  for (const m of text.matchAll(slashed)) keep(Number(m[3]), Number(m[2]), Number(m[1]), m.index ?? 0);
  return found;
}

export function actionItems(body: string, year: number): ActionItem[] {
  const out: ActionItem[] = [];
  let inside = false;
  for (const line of body.split("\n")) {
    if (/^#{1,6}\s/.test(line)) {
      inside = heading.test(line);
      continue;
    }
    const m = inside ? item.exec(line) : null;
    if (!m) continue;
    let text = m[1].trim();
    let owner: string | undefined;
    const colon = text.indexOf(":");
    if (colon > 0 && colon <= 40 && text.slice(0, colon).trim().split(/\s+/).length <= 4) {
      owner = text.slice(0, colon).trim();
      text = text.slice(colon + 1).trim();
    }
    const found = dueIn(text, year);
    let task = text;
    if (found) task = (text.slice(0, found.at).replace(cue, "") + text.slice(found.at).replace(/^[^,.;)]*[.;)]?/, "")).trim();
    task = task.replace(/[\s,;.]+$/, "");
    if (task) out.push({ owner, task, due: found?.due });
  }
  return out;
}
