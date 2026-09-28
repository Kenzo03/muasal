import Link from "next/link";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { Avatar } from "@/components/Chips";
import PageBar from "@/components/PageBar";
import { getProject, serverApi } from "@/lib/server-api";
import { cx, table } from "@/lib/ui";

// Workload: each person's open tickets, for whoever leads the project. Every
// count opens the ticket list filtered to that person and that condition.
export default async function WorkloadPage({ params }: { params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("workload");
  const tp = await getTranslations("project");
  const api = await serverApi();
  const { data } = await api.GET("/projects/{key}/workload", { params: { path: { key } } });
  const rows = data?.rows ?? [];
  const staleDays = data?.stale_days ?? 7;
  const most = Math.max(1, ...rows.map((r) => r.open));
  const list = (who: string, filter: Record<string, string>) => `/p/${key}/tickets?${new URLSearchParams({ assignee: who, ...filter })}`;

  // A count links to its filter; zero stays plain, and tone marks what needs a look.
  const count = (n: number, href: string, tone?: string) =>
    n === 0 ? (
      <span className="text-muted/60">0</span>
    ) : (
      <Link href={href} className={cx("font-bold no-underline hover:underline", tone ?? "text-ink hover:text-ink")}>{n}</Link>
    );

  return (
    <>
      <PageBar>
        <h1>{tp("workload")}</h1>
        <span className="text-[13px] text-muted">{project.name}</span>
      </PageBar>
      <main className="flex flex-col gap-3 px-4 py-4 md:px-5">
        <p className="max-w-3xl text-[13.5px] text-ink-soft">{t("intro", { days: staleDays })}</p>
        <div className={table.wrap}>
          <table className={table.table}>
            <thead className={table.head}>
              <tr>
                <th className={cx(table.th, "pl-5")}>{t("person")}</th>
                <th className={cx(table.th, "w-56")}>{t("open")}</th>
                <th className={cx(table.th, "text-right")}>{t("inProgress")}</th>
                <th className={cx(table.th, "text-right")}>{t("overdue")}</th>
                <th className={cx(table.th, "text-right")}>{t("dueWeek")}</th>
                <th className={cx(table.th, "text-right")}>{t("stale", { days: staleDays })}</th>
                <th className={cx(table.th, "pr-5 text-right")}>{t("high")}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => {
                const who = r.assignee ? String(r.assignee.id) : "none";
                return (
                  <tr key={who} className={cx(table.row, "hover:bg-paper")}>
                    <td className={cx(table.td, "py-3 pl-5")}>
                      {r.assignee ? (
                        <span className="flex items-center gap-2.5 font-semibold">
                          <Avatar name={r.assignee.name} className="size-7 bg-accent-soft text-[10.5px] font-bold text-accent-strong" />
                          {r.assignee.name}
                        </span>
                      ) : (
                        <span className="flex items-center gap-2.5 font-semibold text-ink-soft">
                          <span aria-hidden="true" className="size-7 shrink-0 rounded-full border-2 border-dashed border-field" />
                          {t("unassigned")}
                        </span>
                      )}
                    </td>
                    <td className={cx(table.td, "py-3")}>
                      <span className="flex items-center gap-3">
                        <span className="w-7 text-right tabular-nums">{count(r.open, list(who, { status: "open" }))}</span>
                        <span aria-hidden="true" className="h-1.5 flex-1 overflow-hidden rounded-full bg-well">
                          <span className="block h-full rounded-full bg-accent" style={{ width: `${(100 * r.open) / most}%` }} />
                        </span>
                      </span>
                    </td>
                    <td className={cx(table.td, "py-3 text-right tabular-nums")}>
                      {count(r.in_progress, `/p/${key}/board?${new URLSearchParams({ assignee: who })}`)}
                    </td>
                    <td className={cx(table.td, "py-3 text-right tabular-nums")}>
                      {count(r.overdue, list(who, { due: "overdue" }), "text-danger hover:text-danger")}
                    </td>
                    <td className={cx(table.td, "py-3 text-right tabular-nums")}>{count(r.due_week, list(who, { due: "week" }))}</td>
                    <td className={cx(table.td, "py-3 text-right tabular-nums")}>
                      {count(r.stale, list(who, { stale: String(staleDays) }), "text-warn hover:text-warn")}
                    </td>
                    <td className={cx(table.td, "py-3 pr-5 text-right tabular-nums")}>
                      {count(r.high, list(who, { status: "open", sort: "priority" }))}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        {rows.length === 0 && <p className="text-muted">{t("none")}</p>}
      </main>
    </>
  );
}
