import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getProject, serverApi } from "@/lib/server-api";
import ProjectClients from "./ProjectClients";
import ProjectForm from "./ProjectForm";
import ProjectMembers from "./ProjectMembers";
import StatusesForm from "./StatusesForm";

export default async function SettingsPage({ params }: { params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("settings");
  if (project.role !== "admin") return <p>{t("adminsOnly")}</p>;
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [all, linked, members, statuses] = await Promise.all([
    api.GET("/clients"),
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/members", path),
    api.GET("/projects/{key}/statuses", path),
  ]);
  const linkedClients = linked.data?.items ?? [];
  return (
    <div className="flex flex-col gap-8">
      <ProjectForm project={project} />
      <StatusesForm projectKey={key} statuses={statuses.data?.items ?? []} />
      <ProjectClients projectKey={key} all={all.data?.items ?? []} linked={linkedClients} />
      <ProjectMembers projectKey={key} members={members.data?.items ?? []} clients={linkedClients} />
    </div>
  );
}
