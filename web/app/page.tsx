import Link from "next/link";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import Icon from "@/components/Icon";
import PageBar from "@/components/PageBar";
import { getMe, getProjects } from "@/lib/server-api";
import { button, panel } from "@/lib/ui";

export default async function Home() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("home");
  const projects = await getProjects();
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("projects")}</h1>
        <p className="text-[13px] text-muted">{t("signedInAs", { name: me.name })}</p>
        {me.is_admin && (
          <Link href="/projects/new" className={`${button.primary} ml-auto`}>
            <Icon name="plus" />
            {t("newProject")}
          </Link>
        )}
      </PageBar>
      <main className="mx-auto max-w-4xl p-4 md:p-6">
        {projects.length === 0 ? (
          <p className="text-muted">{me.is_admin ? t("noProjectsAdmin") : t("noProjects")}</p>
        ) : (
          <ul className={`${panel} divide-y divide-line-soft`}>
            {projects.map((p) => (
              <li key={p.id}>
                <Link href={`/p/${p.key}/board`} className="flex items-center gap-3 px-4 py-3 text-ink no-underline hover:bg-paper hover:text-ink">
                  <span className="rounded-[3px] bg-accent-soft px-1.5 font-mono text-xs font-semibold leading-5 text-accent-strong">{p.key}</span>
                  <span className="font-medium">{p.name}</span>
                  {p.description && <span className="min-w-0 truncate text-[13px] text-muted">{p.description}</span>}
                  <Icon name="chevronRight" className="ml-auto size-4 text-muted" />
                </Link>
              </li>
            ))}
          </ul>
        )}
      </main>
    </>
  );
}
