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

/** Maps the filter bar's URL (client=core|id, assignee=me, ...) onto the ticket list API. */
export function ticketQuery(v: Record<string, string>) {
  return {
    q: v.q,
    client_id: v.client && v.client !== "core" ? Number(v.client) : undefined,
    core: v.client === "core" ? true : undefined,
    type: v.type as TicketType | undefined,
    mine: v.assignee === "me" ? true : undefined,
    status_id: v.status ? Number(v.status) : undefined,
    missing: v.missing as "reason" | "menus" | undefined,
    sort: v.sort as "updated" | "created" | "key" | "priority" | "due" | undefined,
    cursor: v.cursor,
  };
}
