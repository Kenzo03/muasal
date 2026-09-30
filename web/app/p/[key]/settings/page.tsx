import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { getProject, serverApi } from "@/lib/server-api";
import { cx, panel } from "@/lib/ui";
import ProjectClients from "./ProjectClients";
import ProjectForm from "./ProjectForm";
import ProjectMembers from "./ProjectMembers";
import Repos from "./Repos";
import StatusesForm from "./StatusesForm";
import ArchiveButton from "../ArchiveButton";

export default async function SettingsPage({ params }: { params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("settings");
  const tp = await getTranslations("project");
  const ta = await getTranslations("archive");
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
        <main className="p-4 text-muted md:p-5">{project.archived_at ? t("archivedReadOnly") : t("adminsOnly")}</main>
      </>
    );
  }
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [all, linked, members, people, statuses, repos] = await Promise.all([
    api.GET("/clients"),
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/members", path),
    api.GET("/projects/{key}/member-candidates", path),
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
        <ProjectMembers projectKey={key} members={members.data?.items ?? []} clients={linkedClients} people={people.data?.items ?? []} />
        <Repos projectKey={key} repos={repos.data?.items ?? []} />
        {/* MSL-64: a finished project leaves pickers and Home, read-only until restored. */}
        <section aria-labelledby="archive-title" className={cx(panel, "flex flex-col items-start gap-2 px-5 py-4")}>
          <h2 id="archive-title" className="text-base font-extrabold">{ta("title")}</h2>
          <p className="text-[13px] text-muted">{ta("hint")}</p>
          <ArchiveButton projectKey={key} name={project.name} archived={false} />
        </section>
      </main>
    </>
  );
}
