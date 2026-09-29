import Link from "next/link";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import Icon from "@/components/Icon";
import PageBar from "@/components/PageBar";
import { getMe, getProjects, serverApi } from "@/lib/server-api";
import { button, cx } from "@/lib/ui";
import ProjectList from "./ProjectList";

// Every project the user may open, by name, with their role and open tickets;
// the sidebar, the switcher and Home link here once their short lists end.
export default async function ProjectsPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("projectList");
  const th = await getTranslations("home");
  const api = await serverApi();
  const [projects, mine] = await Promise.all([getProjects(), api.GET("/me/tickets", { params: { query: { limit: 1 } } })]);
  const open = Object.fromEntries((mine.data?.projects ?? []).map((p) => [p.key, p.open]));
  const byName = [...projects].sort((a, b) => a.name.localeCompare(b.name));
  return (
    <>
      <PageBar>
        <h1>{t("heading")}</h1>
        <span className="text-[13px] text-muted">{t("count", { count: projects.length })}</span>
        {me.is_admin && (
          <Link href="/projects/new" className={cx(button.primary, "ml-auto")}>
            <Icon name="plus" />
            {th("newProject")}
          </Link>
        )}
      </PageBar>
      <main className="px-4 py-4 md:px-5">
        {projects.length === 0 ? (
          <p className="text-muted">{me.is_admin ? th("noProjectsAdmin") : th("noProjects")}</p>
        ) : (
          <ProjectList projects={byName.map((p) => ({ key: p.key, name: p.name, description: p.description, role: p.role }))} open={open} />
        )}
      </main>
    </>
  );
}
