import Link from "next/link";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import TicketFilters from "@/components/TicketFilters";
import { utc } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { one, ticketQuery } from "@/lib/ticket-query";

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
  const t = await getTranslations("tickets");
  const tTypes = await getTranslations("ticketTypes");
  const tPri = await getTranslations("priorities");
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, statuses, page] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/statuses", path),
    api.GET("/projects/{key}/tickets", { params: { path: { key }, query: ticketQuery(values) } }),
  ]);
  const statusName = new Map((statuses.data?.items ?? []).map((s) => [s.id, s.name]));
  const items = page.data?.items ?? [];
  const next = page.data?.next_cursor;
  const { cursor: _, ...kept } = values;
  const today = new Date().toISOString().slice(0, 10);
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <TicketFilters action={`/p/${key}/tickets`} values={values} clients={clients.data?.items ?? []} statuses={statuses.data?.items ?? []} />
        {project.role !== "viewer" && (
          <Link href={`/p/${key}/tickets/new`} className="rounded bg-neutral-900 px-3 py-2 text-sm text-white">{t("new")}</Link>
        )}
      </div>
      {items.length === 0 ? (
        <p className="text-neutral-600">{t("none")}</p>
      ) : (
        <table className="w-full border-collapse bg-white text-left text-sm">
          <thead>
            <tr className="border-b">
              {(["key", "title", "type", "status", "client", "assignee", "requestedBy", "menus", "priority", "updated", "due"] as const).map((c) => (
                <th key={c} className="p-2">{t(c)}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {items.map((it) => (
              <tr key={it.id} className="border-b">
                <td className="p-2 font-mono"><Link href={`/t/${it.key}`} className="underline">{it.key}</Link></td>
                <td className="p-2">{it.title}</td>
                <td className="p-2">{tTypes(it.type)}</td>
                <td className="p-2">{statusName.get(it.status_id)}</td>
                <td className="p-2">{it.client?.name ?? t("core")}</td>
                <td className="p-2">{it.assignee?.name ?? "—"}</td>
                <td className="p-2">{it.requester_name}</td>
                <td className="p-2">
                  {it.node_names[0] ?? "—"}
                  {it.node_names.length > 1 ? ` +${it.node_names.length - 1}` : ""}
                </td>
                <td className="p-2">{tPri(it.priority)}</td>
                <td className="p-2">{utc(it.updated_at)}</td>
                <td className={`p-2 ${it.due_date && it.due_date < today ? "text-red-700" : ""}`}>{it.due_date ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {next && (
        <Link href={`?${new URLSearchParams({ ...kept, cursor: next })}`} className="self-end underline">{t("next")}</Link>
      )}
    </div>
  );
}
