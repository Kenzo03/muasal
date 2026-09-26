import Link from "next/link";
import { notFound } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { ClientChip } from "@/components/Chips";
import PageBar from "@/components/PageBar";
import { day } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { button, cx, table } from "@/lib/ui";

// Decision notes (FSD §9.4): decisions made outside tickets, newest first.
export default async function NotesPage({ params, searchParams }: {
  params: Promise<{ key: string }>;
  searchParams: Promise<{ archived?: string }>;
}) {
  const { key } = await params;
  const { archived } = await searchParams;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("notes");
  const locale = await getLocale();
  const api = await serverApi();
  const { data } = await api.GET("/projects/{key}/notes", { params: { path: { key }, query: { archived: archived === "1" } } });
  const items = data?.items ?? [];
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("heading")}</h1>
        <span className="text-[13px] text-muted">{project.name}</span>
        <Link href={archived === "1" ? "?" : "?archived=1"} className={cx(button.quiet, "ml-auto")}>
          {archived === "1" ? t("hideArchived") : t("showArchived")}
        </Link>
        {project.role !== "viewer" && (
          <Link href={`/p/${key}/notes/new`} className={button.primary}>{t("new")}</Link>
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
                  <th className={table.th}>{t("key")}</th>
                  <th className={table.th}>{t("title")}</th>
                  <th className={table.th}>{t("client")}</th>
                  <th className={table.th}>{t("decidedOn")}</th>
                </tr>
              </thead>
              <tbody>
                {items.map((n) => (
                  <tr key={n.key} className={table.row}>
                    <td className={cx(table.td, "font-mono font-semibold")}><Link href={`/notes/${n.key}`}>{n.key}</Link></td>
                    <td className={table.td}>
                      <Link href={`/notes/${n.key}`} className="text-ink">{n.title}</Link>
                      {n.archived && <span className="ml-2 text-xs text-muted">{t("archivedLabel")}</span>}
                    </td>
                    <td className={table.td}><ClientChip client={n.client} coreLabel={t("allClients")} /></td>
                    <td className={table.td}>{day(n.decided_on, locale)}</td>
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
