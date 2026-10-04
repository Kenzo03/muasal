import Link from "next/link";
import { notFound } from "next/navigation";
import { getTimeZone, getTranslations } from "next-intl/server";
import { showsClients } from "@/components/Chips";
import Icon from "@/components/Icon";
import PageBar from "@/components/PageBar";
import TicketFilters from "@/components/TicketFilters";
import { dateIn } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { one, ticketQuery } from "@/lib/ticket-query";
import { cx, panel } from "@/lib/ui";
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
  const [clients, statuses, assignees, nodes, page, setup, labels, releases] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/statuses", path),
    api.GET("/projects/{key}/assignees", path),
    api.GET("/projects/{key}/nodes", path), // the close dialog's menu picker
    // ponytail: one page of up to 1,000 cards; per-column "Show more" comes with larger boards.
    // Done and Cancelled hold the last 14 days unless "Show all" (FSD §8.4).
    api.GET("/projects/{key}/tickets", {
      params: { path: { key }, query: { ...ticketQuery(values), sort: "priority", limit: 1000, closed_days: showAll ? undefined : 14 } },
    }),
    project.role === "admin" ? api.GET("/projects/{key}/setup", path) : Promise.resolve(undefined),
    api.GET("/projects/{key}/labels", path),
    api.GET("/projects/{key}/releases", path),
  ]);
  // MSL-48: a project admin's setup guide, until the steps the team needs are done.
  const st = setup?.data;
  const steps = st
    ? ([
        ["clients", st.clients, `/p/${key}/settings#clients-title`],
        ["team", st.team, `/p/${key}/settings#members-title`],
        ["documents", st.documents, `/p/${key}/documents`],
        ["tree", st.tree, `/p/${key}/modules`],
        ["tickets", st.tickets, `/p/${key}/tickets/new`],
        ["repos", st.repos, `/p/${key}/settings#repos-title`],
      ] as const)
    : [];
  const ts = await getTranslations("project.setup");
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("board")}</h1>
        <span className="mr-2 text-[13px] text-muted">{project.name}</span>
        <TicketFilters
          action={`/p/${key}/board`}
          values={values}
          clients={clients.data?.items ?? []}
          assignees={assignees.data?.items ?? []}
          labels={(labels.data?.items ?? []).map((l) => l.label)}
          releases={releases.data?.items ?? []}
        />
      </PageBar>
      <main className="px-4 py-4 md:px-5">
        {st && !(st.clients && st.team && st.tree && st.tickets) && (
          <section aria-labelledby="project-setup" className={cx(panel, "mb-4 flex flex-col gap-3 px-5 py-4")}>
            <div className="flex flex-col gap-0.5">
              <h2 id="project-setup" className="text-base font-extrabold">{ts("title")}</h2>
              <span className="text-[13px] text-muted">{ts("hint")}</span>
            </div>
            <ol className="flex flex-col gap-1.5">
              {steps.map(([name, done, href]) => (
                <li key={name} className="flex items-center gap-2.5 text-[13.5px]">
                  <span className={cx("flex size-5 shrink-0 items-center justify-center rounded-full", done ? "bg-ok-soft text-ok" : "bg-well text-muted")}>
                    {done && <Icon name="check" className="size-3.5" />}
                  </span>
                  {done ? <span className="text-muted line-through">{ts(name)}</span> : <Link href={href}>{ts(name)}</Link>}
                </li>
              ))}
            </ol>
          </section>
        )}
        <Board
          projectKey={key}
          statuses={statuses.data?.items ?? []}
          tickets={page.data?.items ?? []}
          nodes={nodes.data?.items ?? []}
          canEdit={project.role !== "viewer"}
          showClients={showsClients(clients.data?.items ?? [])}
          today={dateIn(new Date(), await getTimeZone())}
          query={values}
        />
      </main>
    </>
  );
}
