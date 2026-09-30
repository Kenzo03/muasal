import Link from "next/link";
import { redirect } from "next/navigation";
import { getLocale, getTimeZone, getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { dateTime } from "@/lib/format";
import { getMe, getProjects, serverApi } from "@/lib/server-api";
import { cx, table } from "@/lib/ui";
import NewImport from "./NewImport";

// Admin → Imports (FSD §14.2): one-time ticket imports from CSV or Jira, for
// system admins and for project admins into their projects (MSL-49).
export default async function ImportsPage({ searchParams }: { searchParams: Promise<{ project?: string }> }) {
  const me = await getMe();
  if (!me) redirect("/login");
  const { project } = await searchParams;
  const t = await getTranslations("imports");
  const locale = await getLocale();
  const timeZone = await getTimeZone();
  const projects = (await getProjects()).filter((p) => me.is_admin || p.role === "admin");
  const { data } = projects.length > 0 ? await (await serverApi()).GET("/imports") : { data: undefined };
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
      </PageBar>
      <main className="flex max-w-5xl flex-col gap-4 p-4 md:p-5">
        {!data ? (
          <p className="text-muted">{t("adminsOnly")}</p>
        ) : (
          <>
            <p className="text-[13px] text-muted">{t("intro")}</p>
            <NewImport projects={projects.map((p) => ({ key: p.key, name: p.name }))} defaultKey={project} />
            {data.items.length > 0 && (
              <div className={table.wrap}>
                <table className={table.table}>
                  <thead className={table.head}>
                    <tr>
                      {(["file", "project", "status", "rows", "by", "when"] as const).map((c) => (
                        <th key={c} className={table.th}>{t(`cols.${c}`)}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {data.items.map((r) => (
                      <tr key={r.id} className={table.row}>
                        <td className={table.td}><Link href={`/admin/imports/${r.id}`}>{r.file_name}</Link></td>
                        <td className={cx(table.td, "font-mono")}>{r.project_key}</td>
                        <td className={table.td}>{t(`statuses.${r.status}`)}</td>
                        <td className={table.td}>{r.stats.rows}</td>
                        <td className={table.td}>{r.created_by}</td>
                        <td className={cx(table.td, "whitespace-nowrap")}>{dateTime(r.created_at, locale, timeZone)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </main>
    </>
  );
}
