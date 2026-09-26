import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { fileSize } from "@/lib/format";
import { getMe, serverApi } from "@/lib/server-api";
import { cx, panel, sectionTitle } from "@/lib/ui";

// Admin → System status (FSD §18.3): database size, the job queue, the model
// server's health and the disk use of the two volumes, warning from 80%.
export default async function SystemPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("system");
  const { data } = me.is_admin ? await (await serverApi()).GET("/admin/system/status") : { data: undefined };
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
      </PageBar>
      <main className="flex max-w-4xl flex-col gap-4 p-4 md:p-5">
        {!data ? (
          <p className="text-muted">{t("adminsOnly")}</p>
        ) : (
          <>
            {data.warnings.length > 0 && (
              <ul role="alert" className="flex flex-col gap-1 rounded border border-danger-line bg-danger-soft p-3 text-[13px] text-danger">
                {data.warnings.map((w) => (
                  <li key={w}>{t(`warnings.${w}`)}</li>
                ))}
              </ul>
            )}
            <section className={cx(panel, "grid gap-4 p-4 sm:grid-cols-3")}>
              <div>
                <h2 className={sectionTitle}>{t("database")}</h2>
                <p className="text-lg font-semibold">{fileSize(data.database_bytes)}</p>
              </div>
              <div>
                <h2 className={sectionTitle}>{t("jobs")}</h2>
                <p className="text-[13px]">
                  {t("jobCounts", {
                    waiting: (data.jobs.available ?? 0) + (data.jobs.scheduled ?? 0) + (data.jobs.retryable ?? 0),
                    running: data.jobs.running ?? 0,
                    failed: data.jobs.discarded ?? 0,
                  })}
                </p>
              </div>
              <div>
                <h2 className={sectionTitle}>{t("model")}</h2>
                <p className="text-[13px]">
                  {data.model.mode === "off" ? t("aiOff") : data.model.reachable ? t("reachable") : t("unreachable")}
                </p>
                {data.model.error && <p className="text-xs text-muted">{data.model.error}</p>}
              </div>
            </section>
            <section className={cx(panel, "flex flex-col gap-3 p-4")}>
              <h2 className={sectionTitle}>{t("disks")}</h2>
              {data.disks.map((d) => {
                const pct = d.total_bytes > 0 ? Math.round((d.used_bytes / d.total_bytes) * 100) : 0;
                return (
                  <div key={d.volume} className="flex flex-col gap-1">
                    <div className="flex items-baseline gap-2 text-[13px]">
                      <span className="font-semibold">{t(`volumes.${d.volume}`)}</span>
                      <span className="font-mono text-xs text-muted">{d.path}</span>
                      <span className="ml-auto">
                        {d.missing ? t("missing") : t("used", { used: fileSize(d.used_bytes), total: fileSize(d.total_bytes), pct })}
                      </span>
                    </div>
                    {!d.missing && (
                      <div role="meter" aria-label={t(`volumes.${d.volume}`)} aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} className="h-2 overflow-hidden rounded bg-well">
                        <div className={cx("h-full", pct >= 80 ? "bg-danger" : "bg-accent")} style={{ width: `${pct}%` }} />
                      </div>
                    )}
                  </div>
                );
              })}
            </section>
          </>
        )}
      </main>
    </>
  );
}
