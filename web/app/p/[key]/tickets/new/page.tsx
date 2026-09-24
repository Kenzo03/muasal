import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import TicketForm from "@/components/TicketForm";
import { getProject, serverApi } from "@/lib/server-api";

export default async function NewTicketPage({
  params,
  searchParams,
}: {
  params: Promise<{ key: string }>;
  searchParams: Promise<{ status_id?: string }>;
}) {
  const { key } = await params;
  const { status_id } = await searchParams;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("ticketForm");
  if (project.role === "viewer") return <p>{t("viewersCannot")}</p>;
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, nodes, assignees] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/nodes", path),
    api.GET("/projects/{key}/assignees", path),
  ]);
  return (
    <div className="max-w-3xl rounded-lg border bg-white p-6">
      <h2 className="mb-4 text-xl font-semibold">{t("newTitle")}</h2>
      <TicketForm
        projectKey={key}
        clients={clients.data?.items ?? []}
        nodes={nodes.data?.items ?? []}
        assignees={assignees.data?.items ?? []}
        statusId={status_id ? Number(status_id) : undefined}
      />
    </div>
  );
}
