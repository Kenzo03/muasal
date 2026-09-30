import type { Prefill } from "@/components/TicketForm";
import { getProject, serverApi } from "@/lib/server-api";

// What the create form needs, for the full page and the modal alike.
export async function newTicketData(key: string) {
  const project = await getProject(key);
  if (!project) return null;
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, nodes, assignees] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/nodes", path),
    api.GET("/projects/{key}/assignees", path),
  ]);
  return {
    project,
    clients: clients.data?.items ?? [],
    nodes: nodes.data?.items ?? [],
    assignees: assignees.data?.items ?? [],
  };
}

// MSL-11: a new ticket's fields from the URL, as a note's action item links
// them: ?title=…&assignee=2&due=2026-10-10&client=1&nodes=10,11&reason=…&note=DMS-DN1.
export type NewTicketParams = { title?: string; assignee?: string; due?: string; client?: string; nodes?: string; reason?: string; note?: string };
export function prefill(sp: NewTicketParams): Prefill | undefined {
  if (!sp.note && !sp.title) return undefined;
  const id = (v?: string) => (v && /^\d+$/.test(v) ? Number(v) : undefined);
  return {
    title: sp.title?.slice(0, 200),
    assigneeId: id(sp.assignee),
    due: sp.due && /^\d{4}-\d{2}-\d{2}$/.test(sp.due) ? sp.due : undefined,
    clientId: id(sp.client) ?? null,
    nodeIds: (sp.nodes ?? "").split(",").map(id).filter((n): n is number => n !== undefined),
    reason: sp.reason?.slice(0, 2000),
    noteKey: sp.note,
  };
}
