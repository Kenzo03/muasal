import { notFound } from "next/navigation";
import { getMe, getProject, serverApi } from "@/lib/server-api";
import Activity from "./Activity";
import Attachments from "./Attachments";
import TicketView from "./TicketView";

// The ticket page (FSD §8.6): the target of every citation and link to a ticket.
export default async function TicketPage({ params }: { params: Promise<{ ticketKey: string }> }) {
  const { ticketKey } = await params;
  const api = await serverApi();
  const { data: ticket } = await api.GET("/tickets/{key}", { params: { path: { key: ticketKey } } });
  if (!ticket) notFound();
  const [me, project] = await Promise.all([getMe(), getProject(ticket.project_key)]);
  if (!me || !project) notFound();
  const path = { params: { path: { key: project.key } } };
  const [statuses, activity, clients, nodes, assignees] = await Promise.all([
    api.GET("/projects/{key}/statuses", path),
    api.GET("/tickets/{key}/activity", { params: { path: { key: ticket.key } } }),
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/nodes", path),
    api.GET("/projects/{key}/assignees", path),
  ]);
  const canEdit = project.role !== "viewer";
  return (
    <TicketView
      ticket={ticket}
      statuses={statuses.data?.items ?? []}
      clients={clients.data?.items ?? []}
      nodes={nodes.data?.items ?? []}
      assignees={assignees.data?.items ?? []}
      canEdit={canEdit}
      canEditDecision={project.role === "admin" || ticket.decision?.confirmed_by?.id === me.id}
      activity={<Activity ticketKey={ticket.key} items={activity.data?.items ?? []} meId={me.id} canComment={canEdit} />}
      attachments={
        <Attachments ticketKey={ticket.key} files={ticket.attachments} meId={me.id} isProjectAdmin={project.role === "admin"} canUpload={canEdit} />
      }
    />
  );
}
