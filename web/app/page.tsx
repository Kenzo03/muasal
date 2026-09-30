import { cookies } from "next/headers";
import Link from "next/link";
import { redirect } from "next/navigation";
import { getLocale, getTimeZone, getTranslations } from "next-intl/server";
import { ClientChip, PriorityChip, StatusDot, TypeIcon, initials } from "@/components/Chips";
import Icon from "@/components/Icon";
import { describeChange } from "@/lib/activity";
import { dateIn, day, dayOf } from "@/lib/format";
import { getMe, getProjects, serverApi } from "@/lib/server-api";
import { one } from "@/lib/ticket-query";
import { button, cx, panel } from "@/lib/ui";

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
  const tk = await getTranslations("ask");
  const ta = await getTranslations("activity");
  const tTypes = await getTranslations("ticketTypes");
  const tPri = await getTranslations("priorities");
  const locale = await getLocale();
  const timeZone = await getTimeZone();
  const projects = await getProjects();
  const api = await serverApi();
  const [mine, updates, attention] = await Promise.all([
    api.GET("/me/tickets", { params: { query: { view, cursor: values.cursor } } }),
    api.GET("/me/updates"),
    api.GET("/me/attention"),
  ]);
  const watch = attention.data?.items ?? [];
  // MSL-18: a new server's checklist, for system admins until every step is done.
  const setup = me.is_admin ? (await api.GET("/admin/setup")).data : undefined;
  const steps = setup
    ? ([
        ["ai", setup.ai, "/admin/ai"],
        ["invited", setup.invited, "/admin/users"],
        ["project", setup.project, "/projects/new"],
        ["tree", setup.tree, setup.first_project ? `/p/${setup.first_project}/documents` : "/projects/new"],
        ["history", setup.history, "/admin/imports"],
      ] as const)
    : [];
  const checklist = steps.some(([, done]) => !done) && (
    <section aria-labelledby="setup-title" className={cx(panel, "mx-4 mb-2 flex flex-col gap-3 px-5 py-4 md:mx-5")}>
      <div className="flex flex-col gap-0.5">
        <h2 id="setup-title" className="text-base font-extrabold">{t("setup.title")}</h2>
        <span className="text-[13px] text-muted">{t("setup.hint")}</span>
      </div>
      <ol className="flex flex-col gap-1.5">
        {steps.map(([name, done, href]) => (
          <li key={name} className="flex items-center gap-2.5 text-[13.5px]">
            <span className={cx("flex size-5 shrink-0 items-center justify-center rounded-full", done ? "bg-ok-soft text-ok" : "bg-well text-muted")}>
              {done && <Icon name="check" className="size-3.5" />}
            </span>
            {done ? <span className="text-muted line-through">{t(`setup.${name}`)}</span> : <Link href={href}>{t(`setup.${name}`)}</Link>}
          </li>
        ))}
      </ol>
    </section>
  );
  const items = mine.data?.items ?? [];
  const counts = mine.data?.counts ?? { all: 0, overdue: 0, week: 0, incomplete: 0 };
  const perProject = new Map((mine.data?.projects ?? []).map((p) => [p.key, p.open]));
  // My projects: all of them when there are a few; else up to five, those with my
  // open tickets first (most first), then the ones opened last, then the rest.
  const recent = ((await cookies()).get("recent")?.value ?? "").split(",").filter(Boolean);
  const mineFirst = projects.length <= 5 ? projects : [
    ...new Map(
      [
        ...projects.filter((p) => perProject.has(p.key)).sort((a, b) => (perProject.get(b.key) ?? 0) - (perProject.get(a.key) ?? 0)),
        ...recent.flatMap((k) => projects.filter((p) => p.key === k)),
        ...projects,
      ].map((p) => [p.key, p]),
    ).values(),
  ].slice(0, 5);
  const changes = updates.data?.items ?? [];
  const now = new Date();
  const today = dateIn(now, timeZone);
  // Home renders on the server, so "now" is one moment for the whole page.
  const ago = (iso: string) => {
    const minutes = Math.floor((now.getTime() - Date.parse(iso)) / 60_000);
    if (minutes < 1) return t("ago.now");
    if (minutes < 60) return t("ago.minutes", { n: minutes });
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return t("ago.hours", { n: hours });
    const days = Math.floor(hours / 24);
    if (days === 1) return t("ago.yesterday");
    return days < 7 ? t("ago.days", { n: days }) : dayOf(iso, locale, timeZone);
  };

  // "Selamat pagi" by the clock in the user's profile timezone.
  const hour = Number(new Intl.DateTimeFormat("en-GB", { hour: "numeric", hourCycle: "h23", timeZone }).format(now));
  const part = hour < 11 ? "morning" : hour < 15 ? "midday" : hour < 18 ? "afternoon" : "evening";

  return (
    <>
      <div className="flex flex-wrap items-end gap-4 px-4 pb-2 pt-1 md:px-5">
        <div className="flex flex-col gap-1.5">
          <h1 className="text-[30px] font-extrabold leading-tight tracking-[-0.025em]">{t("greeting", { part, name: me.name.split(/\s+/)[0] })}</h1>
          <p className="text-[15px] text-ink-soft">{t("openCount", { count: counts.all })}</p>
        </div>
        {me.is_admin && (
          <Link href="/projects/new" className={`${button.secondary} ml-auto`}>
            <Icon name="plus" />
            {t("newProject")}
          </Link>
        )}
      </div>
      {checklist}
      {projects.length === 0 ? (
        <main className="p-4 md:p-5">
          <p className="text-muted">{me.is_admin ? t("noProjectsAdmin") : t("noProjects")}</p>
        </main>
      ) : (
        <main className="grid items-start gap-6 px-4 pb-8 pt-3 md:px-5 xl:grid-cols-[minmax(0,1fr)_360px]">
          <div className="flex min-w-0 flex-col gap-5">
            {/* The Home Ask box (FSD §6.4): the question opens on the Ask page. */}
            <form action="/ask" role="search" aria-label={tk("title")} className={cx(panel, "flex items-center gap-3 p-3 pl-4 focus-within:border-accent")}>
              <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-accent-soft text-accent">
                <Icon name="sparkle" className="size-5" />
              </span>
              <input
                name="q"
                required
                maxLength={1000}
                aria-label={tk("question")}
                placeholder={tk("homePlaceholder")}
                className="h-11 min-w-0 flex-1 bg-transparent text-base font-medium text-ink outline-none placeholder:text-muted"
              />
              <button type="submit" className={button.primary}>{tk("send")}</button>
            </form>
            {/* MSL-9: a lead with nothing assigned still sees what is late, unowned or incomplete. */}
            {watch.length > 0 && (
              <section aria-labelledby="attention-title" className={cx(panel, "min-w-0 px-5 py-4")}>
                <div className="flex flex-col gap-0.5">
                  <h2 id="attention-title" className="text-base font-extrabold">{t("attention")}</h2>
                  <span className="text-[13px] text-muted">{t("attentionHint")}</span>
                </div>
                <ul className="mt-3 flex flex-col gap-2.5">
                  {watch.map((p) => (
                    <li key={p.key} className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
                      <Link href={`/p/${p.key}/board`} className="min-w-0 truncate text-[13.5px] font-bold text-ink no-underline hover:underline">{p.name}</Link>
                      {(
                        [
                          ["overdue", p.overdue, "due=overdue", "bg-danger-soft text-danger"],
                          ["week", p.week, "due=week", "bg-well text-ink-soft"],
                          ["unassigned", p.unassigned, "assignee=none", "bg-well text-ink-soft"],
                          ["noReason", p.no_reason, "missing=reason", "bg-warn-soft text-warn"],
                          ["weakReason", p.weak_reason, "missing=weak_reason", "bg-warn-soft text-warn"],
                          ["noMenu", p.no_menu, "missing=menus", "bg-warn-soft text-warn"],
                          ["stale", p.stale, "stale=7", "bg-well text-ink-soft"],
                        ] as const
                      )
                        .filter(([, n]) => n > 0)
                        .map(([name, n, query, tone]) => (
                          <Link
                            key={name}
                            href={`/p/${p.key}/tickets?status=open&${query}`}
                            className={cx("rounded-full px-2.5 text-xs font-semibold leading-6 no-underline hover:underline", tone)}
                          >
                            {t(`attentionCount.${name}`, { count: n })}
                          </Link>
                        ))}
                    </li>
                  ))}
                </ul>
              </section>
            )}
            <section aria-labelledby="mine-title" className={cx(panel, "min-w-0 overflow-hidden")}>
              <div className="flex flex-wrap items-center justify-between gap-3 px-5 pt-4">
                <div className="flex flex-col gap-0.5">
                  <h2 id="mine-title" className="text-base font-extrabold">{t("mine")}</h2>
                  <span className="text-[13px] text-muted">{t("mineHint")}</span>
                </div>
                <nav aria-label={t("mineViews")} className="flex gap-0.5 overflow-x-auto rounded-[11px] bg-well p-[3px]">
                  {views.map((v) => (
                    <Link
                      key={v}
                      href={v === "all" ? "/" : `/?mine=${v}`}
                      aria-current={view === v ? "page" : undefined}
                      className={cx(
                        "flex h-[30px] shrink-0 items-center gap-1.5 rounded-lg px-3 text-[13px] no-underline",
                        view === v ? "bg-white font-bold text-ink shadow-[0_1px_2px_rgba(43,36,32,0.1)] hover:text-ink" : "font-semibold text-ink-soft hover:text-ink",
                      )}
                    >
                      {t(`views.${v}`)}
                      <span
                        className={cx(
                          "text-xs font-bold text-muted",
                          v === "overdue" && counts.overdue > 0 && "text-danger",
                          v === "incomplete" && counts.incomplete > 0 && "text-warn",
                        )}
                      >
                        {counts[v]}
                      </span>
                    </Link>
                  ))}
                </nav>
              </div>
              {items.length === 0 ? (
                <p className="px-5 py-6 text-sm text-muted">
                  {counts.all === 0 ? (
                    <>
                      {t("noTickets")} <Link href={`/p/${projects[0].key}/board`}>{t("openBoard")}</Link>
                    </>
                  ) : (
                    t("noneInView")
                  )}
                </p>
              ) : (
                <ul className="flex flex-col px-2 pb-2 pt-3">
                  {items.map((it) => {
                    const overdue = Boolean(it.due_date && it.due_date < today);
                    return (
                      <li key={it.key} className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl px-3 py-3 hover:bg-paper">
                        <span className="flex size-9 shrink-0 items-center justify-center rounded-[10px] bg-well">
                          <TypeIcon type={it.type} label={tTypes(it.type)} className="size-4" />
                        </span>
                        {/* MSL-23: the title keeps 16rem; client, status and due wrap below it on a narrow column. */}
                        <div className="flex min-w-0 flex-[1_1_16rem] flex-col gap-0.5">
                          <Link href={`/t/${it.key}`} className="truncate text-[14.5px] font-bold text-ink no-underline hover:text-ink hover:underline">{it.title}</Link>
                          <span className="flex flex-wrap items-center gap-x-1.5 text-[12.5px] text-muted">
                            <Link href={`/t/${it.key}`} className="font-bold text-muted no-underline hover:text-ink">{it.key}</Link>
                            {(it.missing_reason || it.missing_menus) && (
                              <span role="img" title={t("missing")} aria-label={t("missing")} className="size-[7px] rounded-full bg-[#D97706]" />
                            )}
                            <span aria-hidden="true">·</span>
                            {it.menu ? <span>{it.menu}</span> : <span className="font-semibold text-warn">{t("menuMissing")}</span>}
                            {it.menu && it.missing_reason && (
                              <>
                                <span aria-hidden="true">·</span>
                                <span className="font-semibold text-warn">{t("reasonMissing")}</span>
                              </>
                            )}
                          </span>
                        </div>
                        <ClientChip client={it.client} coreLabel={t("core")} />
                        <span className="flex w-28 shrink-0 items-center gap-1.5 text-[13px] font-semibold text-ink-soft">
                          <StatusDot color={it.status.color} />
                          <span className="truncate">{it.status.name}</span>
                        </span>
                        {(it.priority === "high" || it.priority === "urgent") && <PriorityChip priority={it.priority} label={tPri(it.priority)} />}
                        <span className={cx("w-20 shrink-0 text-right text-[12.5px]", overdue ? "font-semibold text-danger" : "text-muted")}>
                          {it.due_date ? (
                            <span className="inline-flex items-center gap-1">
                              {overdue && <Icon name="warning" className="size-3.5" />}
                              {overdue && <span className="sr-only">{t("overdue")}</span>}
                              {day(it.due_date, locale, it.due_date.slice(0, 4) !== today.slice(0, 4))}
                            </span>
                          ) : (
                            "–"
                          )}
                        </span>
                      </li>
                    );
                  })}
                </ul>
              )}
              {mine.data?.next_cursor && (
                <div className="border-t border-line-soft px-5 py-3">
                  <Link href={`/?${new URLSearchParams({ ...(view === "all" ? {} : { mine: view }), cursor: mine.data.next_cursor })}`} className={button.secondary}>
                    {t("more")}
                  </Link>
                </div>
              )}
            </section>
          </div>
          <div className="flex flex-col gap-5">
            <section aria-labelledby="recent-title" className={panel}>
              <div className="flex flex-col gap-0.5 px-5 pb-2 pt-4">
                <h2 id="recent-title" className="text-base font-extrabold">{t("recent")}</h2>
                <span className="text-[13px] text-muted">{t("recentHint")}</span>
              </div>
              {changes.length === 0 ? (
                <p className="px-5 pb-5 pt-2 text-sm text-muted">{t("noChanges")}</p>
              ) : (
                <ol className="flex flex-col px-2 pb-2">
                  {changes.map((u) => (
                    <li key={u.key} className="flex flex-col gap-0.5 rounded-xl px-3 py-2.5 hover:bg-paper">
                      <span className="flex min-w-0 items-baseline gap-2">
                        <Link href={`/t/${u.key}`} className="shrink-0 text-xs font-bold text-muted no-underline hover:text-ink">{u.key}</Link>
                        <Link href={`/t/${u.key}`} className="truncate text-[13.5px] font-bold text-ink no-underline hover:text-ink hover:underline">{u.title}</Link>
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
              <h2 id="projects-title" className="px-5 pb-2 pt-4 text-base font-extrabold">{t("projects")}</h2>
              <ul className="flex flex-col px-2 pb-2">
                {mineFirst.map((p) => (
                  <li key={p.id}>
                    <Link href={`/p/${p.key}/board`} className="flex items-center gap-3 rounded-xl px-3 py-2.5 text-ink no-underline hover:bg-paper hover:text-ink">
                      <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-accent-soft text-[11px] font-extrabold text-accent-strong">{initials(p.name)}</span>
                      <span className="flex min-w-0 flex-1 flex-col">
                        <span className="truncate text-[13.5px] font-bold">{p.name}</span>
                        <span className="text-xs text-muted">
                          {p.key} · {t("yourTickets", { count: perProject.get(p.key) ?? 0 })}
                        </span>
                      </span>
                      <Icon name="chevronRight" className="size-4 text-muted" />
                    </Link>
                  </li>
                ))}
              </ul>
              {mineFirst.length < projects.length && (
                <Link
                  href="/projects"
                  className="flex items-center justify-between border-t border-line-soft px-5 py-3 text-[13px] font-bold no-underline"
                >
                  {t("allProjects", { count: projects.length })}
                  <Icon name="arrowRight" className="size-4" />
                </Link>
              )}
            </section>
          </div>
        </main>
      )}
    </>
  );
}
