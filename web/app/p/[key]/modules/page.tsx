import { notFound } from "next/navigation";
import { getProject, serverApi } from "@/lib/server-api";
import ModuleTree from "./ModuleTree";

export default async function ModulesPage({
  params,
  searchParams,
}: {
  params: Promise<{ key: string }>;
  searchParams: Promise<{ archived?: string }>;
}) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project) notFound();
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const canEdit = project.role === "admin";
  // R-MR-4: archived menus stay out of the tree until an admin asks for them.
  const showArchived = canEdit && (await searchParams).archived === "1";
  const nodes =
    (await api.GET("/projects/{key}/nodes", { params: { path: { key }, query: { archived: showArchived || undefined } } })).data?.items ?? [];
  // Only project admins pick clients for menus; the link list is theirs to read.
  const clients = canEdit ? ((await api.GET("/projects/{key}/clients", path)).data?.items ?? []) : [];
  return <ModuleTree projectKey={key} nodes={nodes} clients={clients} canEdit={canEdit} showArchived={showArchived} />;
}
