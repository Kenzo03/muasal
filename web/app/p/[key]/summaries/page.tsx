import Link from "next/link";
import { notFound } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { utc } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { button, cx, table } from "@/lib/ui";

// Change summaries (FSD §12.1): the caller's own, and every one for project admins.
export default async function SummariesPage({ params }: { params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("summaries");
  const locale = await getLocale();
  const api = await serverApi();
  const { data } = await api.GET("/summaries", { params: { query: { project: key } } });
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
                    <td className={table.td}>{utc(s.created_at, locale)}</td>
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
