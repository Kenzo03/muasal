import Link from "next/link";
import { notFound } from "next/navigation";
import { getLocale, getTimeZone, getTranslations } from "next-intl/server";
import { Avatar, ClientChip, PriorityChip, StatusDot, TypeIcon, showsClients } from "@/components/Chips";
import Icon from "@/components/Icon";
import PageBar from "@/components/PageBar";
import TicketFilters from "@/components/TicketFilters";
import { dateIn, dateTime, day, dayOf } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { one, ticketQuery } from "@/lib/ticket-query";
import { button, cx, table } from "@/lib/ui";
import BulkBar, { SelectAll } from "./BulkBar";

// The ticket list (FSD §8.5): filters and pages live in the URL.
export default async function TicketsPage({
  params,
  searchParams,
}: {
  params: Promise<{ key: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const { key } = await params;
  const values = one(await searchParams);
  const project = await getProject(key);
  if (!project) notFound();
  const locale = await getLocale();
  const timeZone = await getTimeZone();
  const t = await getTranslations("tickets");
  const tp = await getTranslations("project");
  const tTypes = await getTranslations("ticketTypes");
  const tPri = await getTranslations("priorities");
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, statuses, assignees, page, labels, releases] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/statuses", path),
    api.GET("/projects/{key}/assignees", path),
    api.GET("/projects/{key}/tickets", { params: { path: { key }, query: ticketQuery(values) } }),
    api.GET("/projects/{key}/labels", path),
    api.GET("/projects/{key}/releases", path),
  ]);
  const statusOf = new Map((statuses.data?.items ?? []).map((s) => [s.id, s]));
  const canEdit = project.role !== "viewer"; // MSL-53: members change tickets together
  const withClients = showsClients(clients.data?.items ?? []);
  const items = page.data?.items ?? [];
  const next = page.data?.next_cursor;
  const { cursor: _, ...kept } = values;
  const today = dateIn(new Date(), timeZone);
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{tp("tickets")}</h1>
        <span className="mr-2 text-[13px] text-muted">{project.name}</span>
        <TicketFilters
          action={`/p/${key}/tickets`}
          values={values}
          clients={clients.data?.items ?? []}
          assignees={assignees.data?.items ?? []}
          statuses={statuses.data?.items ?? []}
          labels={(labels.data?.items ?? []).map((l) => l.label)}
          releases={releases.data?.items ?? []}
        />
        <a href={`/api/v1/projects/${key}/tickets?${new URLSearchParams({ ...exportQuery(ticketQuery(values)), format: "csv" })}`} download className={button.secondary}>
          <Icon name="download" />
          {t("exportCsv")}
        </a>
        {project.role === "admin" && (
          <Link href={`/admin/imports?project=${key}`} className={button.secondary}>
            <Icon name="upload" />
            {t("importTickets")}
          </Link>
        )}
      </PageBar>
      <main className="flex flex-col gap-3 px-4 py-4 md:px-5">
        {canEdit && items.length > 0 && <BulkBar people={assignees.data?.items ?? []} statuses={statuses.data?.items ?? []} releases={releases.data?.items ?? []} />}
        {items.length === 0 ? (
          <p className="text-muted">{t("none")}</p>
        ) : (
          <div className={table.wrap}>
            <table className={table.table}>
              <thead className={table.head}>
                <tr>
                  {canEdit && (
                    <th className={cx(table.th, "w-8 pl-4 pr-0")}>
                      <SelectAll label={t("bulk.selectAll")} />
                    </th>
                  )}
                  <th className={cx(table.th, "pl-5")}>{tp("tickets")}</th>
                  {(["status", "client", "requestedBy", "assignee", "updated", "due"] as const).filter((c) => c !== "client" || withClients).map((c) => (
                    <th key={c} className={cx(table.th, c === "updated" && "hidden 2xl:table-cell")}>{t(c)}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {items.map((it) => {
                  const status = statusOf.get(it.status_id);
                  return (
                    <tr key={it.id} className={cx(table.row, "hover:bg-paper")}>
                      {canEdit && (
                        <td className={cx(table.td, "pl-4 pr-0")}>
                          <input type="checkbox" name="key" value={it.key} form="bulk" aria-label={t("bulk.select", { key: it.key })} className="size-4 accent-accent" />
                        </td>
                      )}
                      <td className={cx(table.td, "min-w-80 py-3 pl-5")}>
                        <span className="flex items-start gap-2.5">
                          <span className="pt-0.5">
                            <TypeIcon type={it.type} label={tTypes(it.type)} />
                          </span>
                          <span className="flex min-w-0 flex-col gap-0.5">
                            <span className="flex flex-wrap items-baseline gap-x-2">
                              <Link href={`/t/${it.key}`} className="text-xs font-bold text-muted no-underline hover:text-ink">{it.key}</Link>
                              <Link href={`/t/${it.key}`} className="text-sm font-bold text-ink no-underline hover:text-ink hover:underline">{it.title}</Link>
                              {(it.priority === "high" || it.priority === "urgent") && <PriorityChip priority={it.priority} label={tPri(it.priority)} />}
                            </span>
                            <span className="text-[12.5px] text-muted">
                              {it.node_names[0] ?? "—"}
                              {it.node_names.length > 1 ? ` +${it.node_names.length - 1}` : ""}
                              {it.checklist && <span title={t("checklist")}> · ☑ {it.checklist.done}/{it.checklist.total}</span>}
                              {it.labels?.map((l) => (
                                <span key={l} className="ml-1.5 rounded-full bg-well px-1.5 text-[11px] font-semibold text-ink-soft">{l}</span>
                              ))}
                            </span>
                          </span>
                        </span>
                      </td>
                      <td className={cx(table.td, "whitespace-nowrap py-3")}>
                        {status && (
                          <span className="flex items-center gap-1.5 font-semibold text-ink-soft">
                            <StatusDot color={status.color} />
                            {status.name}
                          </span>
                        )}
                      </td>
                      {withClients && <td className={cx(table.td, "py-3")}><ClientChip client={it.client} coreLabel={t("core")} /></td>}
                      <td className={cx(table.td, "py-3 text-ink-soft")}>{it.requester_name}</td>
                      <td className={cx(table.td, "whitespace-nowrap py-3")}>
                        {it.assignee ? (
                          <span className="flex items-center gap-1.5 text-ink-soft">
                            <Avatar name={it.assignee.name} className="size-6 bg-accent-soft text-[10px] font-bold text-accent-strong" />
                            {it.assignee.name}
                          </span>
                        ) : (
                          <span className="text-muted">—</span>
                        )}
                      </td>
                      {/* MSL-23: below 1536px, Updated gives way so Due shows without scrolling. */}
                      <td className={cx(table.td, "hidden whitespace-nowrap py-3 text-muted 2xl:table-cell")} title={dateTime(it.updated_at, locale, timeZone)}>{dayOf(it.updated_at, locale, timeZone)}</td>
                      <td className={cx(table.td, "whitespace-nowrap py-3 pr-5", it.due_date && it.due_date < today ? "font-semibold text-danger" : "text-muted")}>
                        {it.due_date ? day(it.due_date, locale) : "—"}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
        {next && (
          <Link href={`?${new URLSearchParams({ ...kept, cursor: next })}`} className={cx(button.secondary, "self-end")}>{t("next")}</Link>
        )}
      </main>
    </>
  );
}

// The list's filter as query strings, for the CSV download (FSD §8.5).
function exportQuery(q: Record<string, unknown>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(q)) if (v !== undefined && v !== null && k !== "cursor" && k !== "limit") out[k] = String(v);
  return out;
}
