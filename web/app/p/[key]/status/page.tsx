import Link from "next/link";
import { notFound } from "next/navigation";
import { getLocale, getTimeZone, getTranslations } from "next-intl/server";
import { showsClients } from "@/components/Chips";
import PageBar from "@/components/PageBar";
import type { components, operations } from "@/lib/api-types";
import { dateIn, day } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { cx, field, table } from "@/lib/ui";
import StatusTools from "./StatusTools";

type Query = NonNullable<operations["listTickets"]["parameters"]["query"]>;
type Ticket = components["schemas"]["TicketSummary"];

// The groups of a weekly status update (MSL-63), each a filter of the ticket
// list as Home and the list count them; a ticket may sit in more than one.
const groups = [
  ["done", { category: "done", closed_days: 7, sort: "updated" }],
  ["inProgress", { category: "in_progress", sort: "priority" }],
  ["overdue", { due: "overdue", sort: "due" }],
  ["dueSoon", { due: "week", sort: "due" }],
  ["unassigned", { open: true, unassigned: true, sort: "priority" }],
] as const satisfies readonly (readonly [string, Query])[];

// A printable weekly status page for a project, or one client with the core
// work every client shares; it copies as Markdown for the client update.
export default async function StatusPage({ params, searchParams }: {
  params: Promise<{ key: string }>;
  searchParams: Promise<{ client?: string }>;
}) {
  const { key } = await params;
  const { client } = await searchParams;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("status");
  const locale = await getLocale();
  const today = dateIn(new Date(), await getTimeZone());
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, statuses] = await Promise.all([api.GET("/projects/{key}/clients", path), api.GET("/projects/{key}/statuses", path)]);
  const clientList = clients.data?.items ?? [];
  const picked = clientList.find((c) => String(c.id) === client);
  const statusName = new Map((statuses.data?.items ?? []).map((s) => [s.id, s.name]));
  // One client's update also covers core work (no client), as its summary does.
  const list = async (q: Query) => {
    const get = (extra: Query) => api.GET("/projects/{key}/tickets", { params: { path: { key }, query: { ...q, ...extra, limit: 200 } } });
    const pages = await Promise.all(picked ? [get({ client_id: picked.id }), get({ core: true })] : [get({})]);
    return { items: pages.flatMap((p) => p.data?.items ?? []), more: pages.some((p) => p.data?.next_cursor) };
  };
  const found = await Promise.all(groups.map(([, q]) => list(q)));

  const line = (it: Ticket) =>
    [it.assignee?.name ?? t("nobody"), statusName.get(it.status_id), it.due_date && t("due", { date: day(it.due_date, locale) })].filter(Boolean).join(", ");
  const heading = t("title", { project: project.name, date: day(today, locale) }) + (picked ? ` · ${picked.name}` : "");
  const markdown = [
    `# ${heading}`,
    ...groups.flatMap(([name], i) => [
      "",
      `## ${t(name)} (${found[i].items.length})`,
      "",
      ...(found[i].items.length === 0 ? [t("none")] : found[i].items.map((it) => `- ${it.key} ${it.title} (${line(it)})`)),
    ]),
  ].join("\n");

  return (
    <>
      <PageBar>
        <h1>{t("heading")}</h1>
        <span className="text-[13px] text-muted">{project.name}</span>
        <StatusTools markdown={markdown} />
      </PageBar>
      <main className="flex flex-col gap-4 px-4 py-4 md:px-5">
        <p className="text-[13px] text-muted print:hidden">{t("intro")}</p>
        {showsClients(clientList) && (
          <form className="flex items-end gap-2 print:hidden">
            <label htmlFor="status-client" className={field.label}>
              {t("client")}
              <select id="status-client" name="client" defaultValue={picked ? String(picked.id) : ""} className={field.compact}>
                <option value="">{t("allClients")}</option>
                {clientList.map((c) => (
                  <option key={c.id} value={c.id}>{c.name}</option>
                ))}
              </select>
            </label>
            <button className="h-9 rounded-lg px-3 text-[13px] font-semibold text-accent">{t("show")}</button>
          </form>
        )}
        <h2 className="hidden text-lg font-bold print:block">{heading}</h2>
        {groups.map(([name], i) => (
          <section key={name} aria-labelledby={`status-${name}`} className="flex flex-col gap-2 break-inside-avoid">
            <h2 id={`status-${name}`} className="text-[15px] font-bold">
              {t(name)} <span className="text-muted">{found[i].items.length}</span>
            </h2>
            {found[i].items.length === 0 ? (
              <p className="text-[13px] text-muted">{t("none")}</p>
            ) : (
              <div className={table.wrap}>
                <table className={table.table}>
                  <tbody>
                    {found[i].items.map((it) => (
                      <tr key={it.id} className={table.row}>
                        <td className={cx(table.td, "w-24 whitespace-nowrap font-mono font-semibold")}><Link href={`/t/${it.key}`}>{it.key}</Link></td>
                        <td className={table.td}>{it.title}</td>
                        <td className={cx(table.td, "text-muted")}>{line(it)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {found[i].more && <p className="text-xs text-muted">{t("more")}</p>}
          </section>
        ))}
      </main>
    </>
  );
}
