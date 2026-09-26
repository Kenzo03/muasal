import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { getProject, serverApi } from "@/lib/server-api";
import ProjectClients from "./ProjectClients";
import ProjectForm from "./ProjectForm";
import ProjectMembers from "./ProjectMembers";
import Repos from "./Repos";
import StatusesForm from "./StatusesForm";

export default async function SettingsPage({ params }: { params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("settings");
  const tp = await getTranslations("project");
  const bar = (
    <PageBar>
      <h1 className="text-base font-semibold">{tp("settings")}</h1>
      <span className="text-[13px] text-muted">{project.name}</span>
    </PageBar>
  );
  if (project.role !== "admin") {
    return (
      <>
        {bar}
        <main className="p-4 text-muted md:p-5">{t("adminsOnly")}</main>
      </>
    );
  }
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [all, linked, members, statuses, repos] = await Promise.all([
    api.GET("/clients"),
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/members", path),
    api.GET("/projects/{key}/statuses", path),
    api.GET("/projects/{key}/repos", path),
  ]);
  const linkedClients = linked.data?.items ?? [];
  return (
    <>
      {bar}
      <main className="mx-auto flex max-w-5xl flex-col gap-4 p-4 md:p-5">
        <ProjectForm project={project} />
        <StatusesForm projectKey={key} statuses={statuses.data?.items ?? []} />
        <ProjectClients projectKey={key} all={all.data?.items ?? []} linked={linkedClients} />
        <ProjectMembers projectKey={key} members={members.data?.items ?? []} clients={linkedClients} />
        <Repos projectKey={key} repos={repos.data?.items ?? []} />
      </main>
    </>
  );
}
