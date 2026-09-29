import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { showsClients } from "@/components/Chips";
import PageBar from "@/components/PageBar";
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
  const t = await getTranslations("project");
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const showAll = values.closed === "all";
  const [clients, statuses, nodes, page] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/statuses", path),
    api.GET("/projects/{key}/nodes", path), // the close dialog's menu picker
    // ponytail: one page of up to 1,000 cards; per-column "Show more" comes with larger boards.
    // Done and Cancelled hold the last 14 days unless "Show all" (FSD §8.4).
    api.GET("/projects/{key}/tickets", {
      params: { path: { key }, query: { ...ticketQuery(values), sort: "priority", limit: 1000, closed_days: showAll ? undefined : 14 } },
    }),
  ]);
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("board")}</h1>
        <span className="mr-2 text-[13px] text-muted">{project.name}</span>
        <TicketFilters action={`/p/${key}/board`} values={values} clients={clients.data?.items ?? []} />
      </PageBar>
      <main className="px-4 py-4 md:px-5">
        <Board
          projectKey={key}
          statuses={statuses.data?.items ?? []}
          tickets={page.data?.items ?? []}
          nodes={nodes.data?.items ?? []}
          canEdit={project.role !== "viewer"}
          showClients={showsClients(clients.data?.items ?? [])}
          today={new Date().toISOString().slice(0, 10)}
          query={values}
        />
      </main>
    </>
  );
}
