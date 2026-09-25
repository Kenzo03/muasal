import Link from "next/link";
import { redirect } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { ClientChip, PriorityChip, StatusDot, TypeIcon } from "@/components/Chips";
import Icon from "@/components/Icon";
import PageBar from "@/components/PageBar";
import { describeChange } from "@/lib/activity";
import { day } from "@/lib/format";
import { getMe, getProjects, serverApi } from "@/lib/server-api";
import { one } from "@/lib/ticket-query";
import { button, cx, panel, table } from "@/lib/ui";

const views = ["all", "overdue", "week", "incomplete"] as const;
type View = (typeof views)[number];

// Home (FSD §6.4): what I have to do and what changed in my projects. A work
// list, not a report, so no charts. The tab lives in the URL (/?mine=overdue).
export default async function Home({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const me = await getMe();
  if (!me) redirect("/login");
  const values = one(await searchParams);
  const view: View = views.includes(values.mine as View) ? (values.mine as View) : "all";
  const t = await getTranslations("home");
  const ta = await getTranslations("activity");
  const tTypes = await getTranslations("ticketTypes");
  const tPri = await getTranslations("priorities");
  const locale = await getLocale();
  const projects = await getProjects();
  const api = await serverApi();
  const [mine, updates] = await Promise.all([
    api.GET("/me/tickets", { params: { query: { view, cursor: values.cursor } } }),
    api.GET("/me/updates"),
  ]);
  const items = mine.data?.items ?? [];
  const counts = mine.data?.counts ?? { all: 0, overdue: 0, week: 0, incomplete: 0 };
  const perProject = new Map((mine.data?.projects ?? []).map((p) => [p.key, p.open]));
  const changes = updates.data?.items ?? [];
  const now = new Date();
  const today = now.toISOString().slice(0, 10);
  // Home renders on the server, so "now" is one moment for the whole page.
  const ago = (iso: string) => {
    const minutes = Math.floor((now.getTime() - Date.parse(iso)) / 60_000);
    if (minutes < 1) return t("ago.now");
    if (minutes < 60) return t("ago.minutes", { n: minutes });
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return t("ago.hours", { n: hours });
    const days = Math.floor(hours / 24);
    if (days === 1) return t("ago.yesterday");
    return days < 7 ? t("ago.days", { n: days }) : day(iso, locale);
  };

  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
        <p className="text-[13px] text-muted">{t("signedInAs", { name: me.name })}</p>
        {me.is_admin && (
          <Link href="/projects/new" className={`${button.primary} ml-auto`}>
            <Icon name="plus" />
            {t("newProject")}
          </Link>
        )}
      </PageBar>
      {projects.length === 0 ? (
        <main className="p-4 md:p-5">
          <p className="text-muted">{me.is_admin ? t("noProjectsAdmin") : t("noProjects")}</p>
        </main>
      ) : (
        <main className="grid items-start gap-5 p-4 md:p-5 xl:grid-cols-[minmax(0,1fr)_360px]">
          <section aria-labelledby="mine-title" className={cx(panel, "min-w-0 overflow-hidden")}>
            <div className="flex flex-wrap items-baseline gap-x-2.5 px-4 pt-3">
              <h2 id="mine-title" className="text-sm font-semibold">{t("mine")}</h2>
              <span className="text-[13px] text-muted">{t("mineHint")}</span>
            </div>
            <nav aria-label={t("mineViews")} className="flex gap-1 overflow-x-auto border-b border-line px-2">
              {views.map((v) => (
                <Link
                  key={v}
                  href={v === "all" ? "/" : `/?mine=${v}`}
                  aria-current={view === v ? "page" : undefined}
                  className={cx(
                    "flex shrink-0 items-center gap-1.5 border-b-2 px-2 pb-2 pt-2.5 text-[13px] no-underline",
                    view === v ? "border-accent font-semibold text-ink hover:text-ink" : "border-transparent text-muted hover:text-ink",
                  )}
                >
                  {t(`views.${v}`)}
                  <span
                    className={cx(
                      "font-mono text-[11px]",
                      v === "overdue" && counts.overdue > 0 && "font-semibold text-danger",
                      v === "incomplete" && counts.incomplete > 0 && "font-semibold text-warn",
                    )}
                  >
                    {counts[v]}
                  </span>
                </Link>
              ))}
            </nav>
            {items.length === 0 ? (
              <p className="px-4 py-6 text-sm text-muted">
                {counts.all === 0 ? (
                  <>
                    {t("noTickets")} <Link href={`/p/${projects[0].key}/board`}>{t("openBoard")}</Link>
                  </>
                ) : (
                  t("noneInView")
                )}
              </p>
            ) : (
              <div className="overflow-x-auto">
                <table className={table.table}>
                  <thead className={table.head}>
                    <tr>
                      <th className={cx(table.th, "pl-4")}>{t("columns.ticket")}</th>
                      <th className={table.th}>{t("columns.title")}</th>
                      <th className={table.th}>{t("columns.status")}</th>
                      <th className={table.th}>{t("columns.client")}</th>
                      <th className={table.th}>{t("columns.priority")}</th>
                      <th className={cx(table.th, "pr-4 text-right")}>{t("columns.due")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((it) => {
                      const overdue = Boolean(it.due_date && it.due_date < today);
                      return (
                        <tr key={it.key} className={cx(table.row, "hover:bg-paper")}>
                          <td className={cx(table.td, "whitespace-nowrap pl-4")}>
                            <span className="flex items-center gap-1.5">
                              <TypeIcon type={it.type} label={tTypes(it.type)} />
                              <Link href={`/t/${it.key}`} className="font-mono text-xs font-semibold">{it.key}</Link>
                              {(it.missing_reason || it.missing_menus) && (
                                <span role="img" title={t("missing")} aria-label={t("missing")} className="size-[7px] rounded-full bg-[#D97706]" />
                              )}
                            </span>
                          </td>
                          <td className={cx(table.td, "min-w-64")}>
                            <Link href={`/t/${it.key}`} className="font-medium text-ink no-underline hover:text-ink hover:underline">{it.title}</Link>
                            <span className="flex flex-wrap gap-x-1.5 text-xs text-muted">
                              {it.menu ? <span>{it.menu}</span> : <span className="font-medium text-warn">{t("menuMissing")}</span>}
                              {it.menu && it.missing_reason && (
                                <>
                                  <span aria-hidden="true">·</span>
                                  <span className="font-medium text-warn">{t("reasonMissing")}</span>
                                </>
                              )}
                            </span>
                          </td>
                          <td className={cx(table.td, "whitespace-nowrap")}>
                            <span className="flex items-center gap-1.5">
                              <StatusDot color={it.status.color} />
                              {it.status.name}
                            </span>
                          </td>
                          <td className={table.td}><ClientChip client={it.client} coreLabel={t("core")} /></td>
                          <td className={table.td}><PriorityChip priority={it.priority} label={tPri(it.priority)} /></td>
                          <td className={cx(table.td, "whitespace-nowrap pr-4 text-right", overdue ? "font-medium text-danger" : "text-muted")}>
                            {it.due_date ? (
                              <span className="inline-flex items-center gap-1">
                                {overdue && <Icon name="warning" className="size-3.5" />}
                                {overdue && <span className="sr-only">{t("overdue")}</span>}
                                {day(it.due_date, locale, it.due_date.slice(0, 4) !== today.slice(0, 4))}
                              </span>
                            ) : (
                              "–"
                            )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
            {mine.data?.next_cursor && (
              <div className="border-t border-line-soft px-4 py-2.5">
                <Link href={`/?${new URLSearchParams({ ...(view === "all" ? {} : { mine: view }), cursor: mine.data.next_cursor })}`} className={button.secondary}>
                  {t("more")}
                </Link>
              </div>
            )}
          </section>
          <div className="flex flex-col gap-5">
            <section aria-labelledby="recent-title" className={panel}>
              <div className="flex items-baseline gap-2 border-b border-line px-4 pb-2.5 pt-3">
                <h2 id="recent-title" className="text-sm font-semibold">{t("recent")}</h2>
                <span className="text-[13px] text-muted">{t("recentHint")}</span>
              </div>
              {changes.length === 0 ? (
                <p className="px-4 py-4 text-sm text-muted">{t("noChanges")}</p>
              ) : (
                <ol className="divide-y divide-line-soft">
                  {changes.map((u) => (
                    <li key={u.key} className="flex flex-col gap-0.5 px-4 py-2.5">
                      <span className="flex min-w-0 items-baseline gap-2">
                        <Link href={`/t/${u.key}`} className="shrink-0 font-mono text-xs font-semibold">{u.key}</Link>
                        <Link href={`/t/${u.key}`} className="truncate text-[13px] font-medium text-ink no-underline hover:text-ink hover:underline">{u.title}</Link>
                      </span>
                      <span className="text-xs text-muted">
                        {u.change ? describeChange(ta, u.change, me.id) : t("changed")} · {ago(u.change?.at ?? u.updated_at)}
                      </span>
                    </li>
                  ))}
                </ol>
              )}
            </section>
            <section aria-labelledby="projects-title" className={panel}>
              <h2 id="projects-title" className="border-b border-line px-4 pb-2.5 pt-3 text-sm font-semibold">{t("projects")}</h2>
              <ul className="divide-y divide-line-soft">
                {projects.map((p) => (
                  <li key={p.id}>
                    <Link href={`/p/${p.key}/board`} className="flex items-center gap-2.5 px-4 py-2.5 text-ink no-underline hover:bg-paper hover:text-ink">
                      <span className="rounded-[3px] bg-accent-soft px-1.5 font-mono text-[11px] font-semibold leading-5 text-accent-strong">{p.key}</span>
                      <span className="min-w-0 truncate text-[13px] font-medium">{p.name}</span>
                      <span className="ml-auto shrink-0 text-xs text-muted">{t("yourTickets", { count: perProject.get(p.key) ?? 0 })}</span>
                      <Icon name="chevronRight" className="size-4 text-muted" />
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          </div>
        </main>
      )}
    </>
  );
}
