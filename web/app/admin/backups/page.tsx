import { redirect } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { fileSize, utc } from "@/lib/format";
import { getMe, serverApi } from "@/lib/server-api";
import { cx, panel, sectionTitle, table } from "@/lib/ui";
import RunBackup from "./RunBackup";

// Admin → Backups (FSD §15.6): the last backup's time, size and location,
// "Run backup now", and the restore steps (§19.4).
export default async function BackupsPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("backups");
  const locale = await getLocale();
  const { data } = me.is_admin ? await (await serverApi()).GET("/admin/backups") : { data: undefined };
  const last = data?.items[0];
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
        {data && <RunBackup requested={data.requested} />}
      </PageBar>
      <main className="flex max-w-4xl flex-col gap-4 p-4 md:p-5">
        {!data ? (
          <p className="text-muted">{t("adminsOnly")}</p>
        ) : (
          <>
            <section className={cx(panel, "grid gap-4 p-4 sm:grid-cols-3")}>
              <div>
                <h2 className={sectionTitle}>{t("last")}</h2>
                <p className="text-[13px] font-semibold">{last ? utc(last.created_at, locale) : t("none")}</p>
              </div>
              <div>
                <h2 className={sectionTitle}>{t("size")}</h2>
                <p className="text-[13px]">{last ? fileSize(last.size_bytes) : "—"}</p>
              </div>
              <div>
                <h2 className={sectionTitle}>{t("location")}</h2>
                <p className="font-mono text-xs">{data.location}</p>
              </div>
            </section>
            {data.items.length > 0 && (
              <div className={table.wrap}>
                <table className={table.table}>
                  <thead className={table.head}>
                    <tr>
                      <th className={table.th}>{t("file")}</th>
                      <th className={table.th}>{t("when")}</th>
                      <th className={table.th}>{t("size")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.items.map((b) => (
                      <tr key={b.file} className={table.row}>
                        <td className={cx(table.td, "font-mono text-xs")}>{b.file}</td>
                        <td className={table.td}>{utc(b.created_at, locale)}</td>
                        <td className={table.td}>{fileSize(b.size_bytes)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            <section className={cx(panel, "flex flex-col gap-2 p-4 text-[13px] leading-relaxed")}>
              <h2 className={sectionTitle}>{t("restoreTitle")}</h2>
              <p>{t("schedule")}</p>
              <ol className="ml-5 list-decimal">
                <li>{t("restore1")}</li>
                <li>
                  {t("restore2")} <code className="font-mono text-xs">./restore.sh {last?.file ?? "db-YYYYMMDD-HHMM.dump"}</code>
                </li>
                <li>{t("restore3")}</li>
              </ol>
              <p className="text-muted">{t("offsite")}</p>
            </section>
          </>
        )}
      </main>
    </>
  );
}
