import Link from "next/link";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import TicketFilters from "@/components/TicketFilters";
import { getProject, serverApi } from "@/lib/server-api";
import { one, ticketQuery } from "@/lib/ticket-query";
import Board from "./Board";

// The board (FSD §8.4): one column per status, cards by priority, due date, then key.
export default async function BoardPage({
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
  const t = await getTranslations("board");
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, statuses, page] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/statuses", path),
    // ponytail: one page of up to 1,000 cards; per-column "Show more" comes with larger boards.
    api.GET("/projects/{key}/tickets", { params: { path: { key }, query: { ...ticketQuery(values), sort: "priority", limit: 1000 } } }),
  ]);
  const canEdit = project.role !== "viewer";
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <TicketFilters action={`/p/${key}/board`} values={values} clients={clients.data?.items ?? []} />
        {canEdit && (
          <Link href={`/p/${key}/tickets/new`} className="rounded bg-neutral-900 px-3 py-2 text-sm text-white">{t("newTicket")}</Link>
        )}
      </div>
      <Board
        projectKey={key}
        statuses={statuses.data?.items ?? []}
        tickets={page.data?.items ?? []}
        canEdit={canEdit}
        today={new Date().toISOString().slice(0, 10)}
      />
    </div>
  );
}
