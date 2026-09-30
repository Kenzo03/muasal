import type { TicketType } from "./problem";

type SearchParams = Record<string, string | string[] | undefined>;

/** The first value of each search parameter, without empty ones. */
export function one(sp: SearchParams): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(sp)) {
    const first = Array.isArray(v) ? v[0] : v;
    if (first) out[k] = first;
  }
  return out;
}

const id = (v?: string) => (v && /^\d+$/.test(v) ? Number(v) : undefined);

/**
 * Maps the filter bar's URL onto the ticket list API: client=core|id,
 * assignee=me|none|id, status=open|id, due=overdue|week, stale=days, ...
 */
export function ticketQuery(v: Record<string, string>) {
  const stale = id(v.stale);
  return {
    q: v.q,
    client_id: v.client && v.client !== "core" ? Number(v.client) : undefined,
    core: v.client === "core" ? true : undefined,
    type: v.type as TicketType | undefined,
    label: v.label || undefined, // MSL-56
    accepted: v.accepted === "yes" ? true : v.accepted === "no" ? false : undefined, // MSL-66
    release_id: id(v.release), // MSL-67
    mine: v.assignee === "me" ? true : undefined,
    unassigned: v.assignee === "none" ? true : undefined,
    assignee_id: id(v.assignee),
    status_id: id(v.status),
    open: v.status === "open" ? true : undefined,
    due: (["overdue", "week"] as const).find((d) => d === v.due),
    stale_days: stale && stale <= 365 ? stale : undefined,
    missing: v.missing as "reason" | "menus" | "weak_reason" | undefined,
    sort: v.sort as "updated" | "created" | "key" | "priority" | "due" | undefined,
    cursor: v.cursor,
  };
}
