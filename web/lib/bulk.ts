import type { components } from "./api-types";

type Ticket = components["schemas"]["Ticket"];
type Priority = components["schemas"]["Priority"];

/** One change for many tickets (MSL-53): a field left out stays as it is; "" clears assignee or due date. */
export type BulkChange = { assignee?: string; priority?: Priority; due?: string; status?: number };

/** The PUT body that keeps a ticket as it is except for the change. */
export function updateBody(t: Ticket, c: BulkChange) {
  return {
    type: t.type,
    title: t.title,
    client_id: t.client?.id,
    requester_contact_id: t.requester.kind === "contact" ? t.requester.id : undefined,
    requester_user_id: t.requester.kind === "user" ? t.requester.id : undefined,
    node_ids: t.nodes.map((n) => n.id),
    reason: t.reason,
    description: t.description,
    assignee_id: c.assignee === undefined ? t.assignee?.id : c.assignee === "" ? undefined : Number(c.assignee),
    priority: c.priority ?? t.priority,
    due_date: c.due === undefined ? (t.due_date ?? undefined) : c.due || undefined,
    estimate_hours: t.estimate_hours ?? undefined,
    labels: t.labels ?? [],
  };
}

/** Whether the change touches a ticket's fields, not only its status. */
export const changesFields = (c: BulkChange) => c.assignee !== undefined || c.priority !== undefined || c.due !== undefined;
