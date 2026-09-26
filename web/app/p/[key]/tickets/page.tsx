import Link from "next/link";
import { notFound } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { Avatar, ClientChip, PriorityChip, StatusDot, TypeIcon } from "@/components/Chips";
import PageBar from "@/components/PageBar";
import TicketFilters from "@/components/TicketFilters";
import { day, utc } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { one, ticketQuery } from "@/lib/ticket-query";
import { button, cx, table } from "@/lib/ui";

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
  const t = await getTranslations("tickets");
  const tp = await getTranslations("project");
  const tTypes = await getTranslations("ticketTypes");
  const tPri = await getTranslations("priorities");
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, statuses, page] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/statuses", path),
    api.GET("/projects/{key}/tickets", { params: { path: { key }, query: ticketQuery(values) } }),
  ]);
  const statusOf = new Map((statuses.data?.items ?? []).map((s) => [s.id, s]));
  const items = page.data?.items ?? [];
  const next = page.data?.next_cursor;
  const { cursor: _, ...kept } = values;
  const today = new Date().toISOString().slice(0, 10);
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{tp("tickets")}</h1>
        <span className="mr-2 text-[13px] text-muted">{project.name}</span>
        <TicketFilters action={`/p/${key}/tickets`} values={values} clients={clients.data?.items ?? []} statuses={statuses.data?.items ?? []} />
        <a href={`/api/v1/projects/${key}/tickets?${new URLSearchParams({ ...exportQuery(ticketQuery(values)), format: "csv" })}`} download className={button.secondary}>
          {t("exportCsv")}
        </a>
      </PageBar>
      <main className="flex flex-col gap-3 px-4 py-4 md:px-5">
        {items.length === 0 ? (
          <p className="text-muted">{t("none")}</p>
        ) : (
          <div className={table.wrap}>
            <table className={table.table}>
              <thead className={table.head}>
                <tr>
                  {(["key", "title", "status", "client", "assignee", "requestedBy", "menus", "priority", "updated", "due"] as const).map((c) => (
                    <th key={c} className={table.th}>{t(c)}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {items.map((it) => {
                  const status = statusOf.get(it.status_id);
                  return (
                    <tr key={it.id} className={cx(table.row, "hover:bg-paper")}>
                      <td className={cx(table.td, "whitespace-nowrap")}>
                        <span className="flex items-center gap-1.5">
                          <TypeIcon type={it.type} label={tTypes(it.type)} />
                          <Link href={`/t/${it.key}`} className="font-mono font-semibold">{it.key}</Link>
                        </span>
                      </td>
                      <td className={cx(table.td, "min-w-64 font-medium")}>
                        <Link href={`/t/${it.key}`} className="text-ink no-underline hover:text-ink hover:underline">{it.title}</Link>
                      </td>
                      <td className={cx(table.td, "whitespace-nowrap")}>
                        {status && (
                          <span className="flex items-center gap-1.5">
                            <StatusDot color={status.color} />
                            {status.name}
                          </span>
                        )}
                      </td>
                      <td className={table.td}><ClientChip client={it.client} coreLabel={t("core")} /></td>
                      <td className={cx(table.td, "whitespace-nowrap")}>
                        {it.assignee ? (
                          <span className="flex items-center gap-1.5">
                            <Avatar name={it.assignee.name} className="size-5 bg-well text-[9px] text-ink" />
                            {it.assignee.name}
                          </span>
                        ) : (
                          <span className="text-muted">—</span>
                        )}
                      </td>
                      <td className={table.td}>{it.requester_name}</td>
                      <td className={cx(table.td, "font-mono text-xs text-muted")}>
                        {it.node_names[0] ?? "—"}
                        {it.node_names.length > 1 ? ` +${it.node_names.length - 1}` : ""}
                      </td>
                      <td className={table.td}><PriorityChip priority={it.priority} label={tPri(it.priority)} /></td>
                      <td className={cx(table.td, "whitespace-nowrap text-muted")}>{utc(it.updated_at, locale)}</td>
                      <td className={cx(table.td, "whitespace-nowrap", it.due_date && it.due_date < today ? "font-medium text-danger" : "text-muted")}>
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
