import Link from "next/link";
import { notFound } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { ClientChip } from "@/components/Chips";
import PageBar from "@/components/PageBar";
import { utc } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { cx, table } from "@/lib/ui";
import Upload from "./Upload";

// Project documents (FSD §7.7): specifications Ask cites by section, and the
// source of AI tree drafts.
export default async function DocumentsPage({ params }: { params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("documents");
  const locale = await getLocale();
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [docs, clients] = await Promise.all([api.GET("/projects/{key}/documents", path), api.GET("/projects/{key}/clients", path)]);
  const items = docs.data?.items ?? [];
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("heading")}</h1>
        <span className="text-[13px] text-muted">{project.name}</span>
      </PageBar>
      <main className="flex flex-col gap-3 px-4 py-4 md:px-5">
        <p className="text-[13px] text-muted">{t("intro")}</p>
        {project.role === "admin" && <Upload projectKey={key} clients={clients.data?.items ?? []} documents={items} />}
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
                  <th className={table.th}>{t("uploaded")}</th>
                </tr>
              </thead>
              <tbody>
                {items.map((d) => (
                  <tr key={d.key} className={table.row}>
                    <td className={cx(table.td, "font-mono font-semibold")}><Link href={`/documents/${d.key}`}>{d.key}</Link></td>
                    <td className={table.td}>
                      <Link href={`/documents/${d.key}`} className="text-ink">{d.title}</Link>
                      {d.superseded_by && <span className="ml-2 text-xs text-muted">{t("supersededBy", { key: d.superseded_by })}</span>}
                    </td>
                    <td className={table.td}><ClientChip client={d.client} coreLabel={t("allClients")} /></td>
                    <td className={table.td}>{utc(d.created_at, locale)} · {d.uploaded_by}</td>
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
