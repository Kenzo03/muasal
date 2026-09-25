import { getTranslations } from "next-intl/server";
import { newTicketData } from "@/app/p/[key]/tickets/new/data";
import NewTicketModal from "./NewTicketModal";

// The create form as a modal over the current page (FSD §8.3): a link or the
// `c` shortcut to /p/{key}/tickets/new lands here on client navigation; a
// reload or a shared link opens the full page instead.
export default async function NewTicketModalPage({
  params,
  searchParams,
}: {
  params: Promise<{ key: string }>;
  searchParams: Promise<{ status_id?: string; node_id?: string }>;
}) {
  const { key } = await params;
  const { status_id, node_id } = await searchParams;
  const data = await newTicketData(key);
  if (!data || data.project.role === "viewer") return null;
  const t = await getTranslations("ticketForm");
  return (
    <NewTicketModal
      title={t("newIn", { project: data.project.key })}
      closeLabel={t("close")}
      projectKey={key}
      clients={data.clients}
      nodes={data.nodes}
      assignees={data.assignees}
      statusId={status_id ? Number(status_id) : undefined}
      nodeId={node_id ? Number(node_id) : undefined}
    />
  );
}
