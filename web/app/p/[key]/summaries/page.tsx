import Link from "next/link";
import { notFound } from "next/navigation";
import { getLocale, getTimeZone, getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { dateTime } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { button, cx, table } from "@/lib/ui";
import Schedules from "./Schedules";

// Change summaries (FSD §12.1): the caller's own, and every one for project admins.
export default async function SummariesPage({ params }: { params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("summaries");
  const locale = await getLocale();
  const timeZone = await getTimeZone();
  const api = await serverApi();
  const admin = project.role === "admin";
  const path = { params: { path: { key } } };
  const [{ data }, schedules, clients] = await Promise.all([
    api.GET("/summaries", { params: { query: { project: key } } }),
    admin ? api.GET("/projects/{key}/summary-schedules", path) : undefined,
    admin ? api.GET("/projects/{key}/clients", path) : undefined,
  ]);
  const items = data?.items ?? [];
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("heading")}</h1>
        <span className="text-[13px] text-muted">{project.name}</span>
        {project.role !== "viewer" && (
          <Link href={`/p/${key}/summaries/new`} className={cx(button.primary, "ml-auto")}>{t("new")}</Link>
        )}
      </PageBar>
      <main className="flex flex-col gap-3 px-4 py-4 md:px-5">
        <p className="text-[13px] text-muted">{t("intro")}</p>
        {admin && <Schedules projectKey={key} clients={clients?.data?.items ?? []} schedules={schedules?.data?.items ?? []} />}
        {items.length === 0 ? (
          <p className="text-muted">{t("none")}</p>
        ) : (
          <div className={table.wrap}>
            <table className={table.table}>
              <thead className={table.head}>
                <tr>
                  <th className={table.th}>{t("title")}</th>
                  <th className={table.th}>{t("creator")}</th>
                  <th className={table.th}>{t("created")}</th>
                </tr>
              </thead>
              <tbody>
                {items.map((s) => (
                  <tr key={s.id} className={table.row}>
                    <td className={table.td}><Link href={`/summaries/${s.id}`} className="text-ink">{s.title}</Link></td>
                    <td className={table.td}>{s.creator}</td>
                    <td className={table.td}>{dateTime(s.created_at, locale, timeZone)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </main>
    </>
  );
}
