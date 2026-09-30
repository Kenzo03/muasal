import type { components } from "./api-types";

type Notification = components["schemas"]["Notification"];

/**
 * Notifications as the bell lists them (MSL-28): consecutive ones about one
 * ticket from one person, each within five minutes of the next, make one
 * burst, as when someone assigns a ticket and moves it. Newest first, as given.
 */
export function bursts(items: Notification[], gapMs = 5 * 60_000): Notification[][] {
  const out: Notification[][] = [];
  for (const n of items) {
    const g = out.at(-1);
    const last = g?.at(-1);
    const same = last && n.ticket_key && n.ticket_key === last.ticket_key && n.actor && n.actor.id === last.actor?.id;
    if (g && same && Date.parse(last.created_at) - Date.parse(n.created_at) <= gapMs) g.push(n);
    else out.push([n]);
  }
  return out;
}
