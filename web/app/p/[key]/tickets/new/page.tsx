import Link from "next/link";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import Icon from "@/components/Icon";
import PageBar from "@/components/PageBar";
import TicketForm from "@/components/TicketForm";
import { panel } from "@/lib/ui";
import { newTicketData } from "./data";

export default async function NewTicketPage({
  params,
  searchParams,
}: {
  params: Promise<{ key: string }>;
  searchParams: Promise<{ status_id?: string; node_id?: string }>;
}) {
  const { key } = await params;
  const { status_id, node_id } = await searchParams;
  const data = await newTicketData(key);
  if (!data) notFound();
  const { project, clients, nodes, assignees } = data;
  const t = await getTranslations("ticketForm");
  const tp = await getTranslations("project");
  return (
    <>
      <PageBar>
        <nav className="flex items-center gap-1.5 text-[13px] text-muted">
          <Link href={`/p/${key}/board`}>{key}</Link>
          <Icon name="chevronRight" className="size-3.5" />
          <Link href={`/p/${key}/tickets`}>{tp("tickets")}</Link>
          <Icon name="chevronRight" className="size-3.5" />
          <h1 className="text-[13px] font-semibold text-ink">{t("newTitle")}</h1>
        </nav>
      </PageBar>
      <main className="p-4 md:p-5">
        {project.role === "viewer" ? (
          <p className="text-muted">{t("viewersCannot")}</p>
        ) : (
          <div className={`${panel} mx-auto max-w-[820px]`}>
            <TicketForm
              projectKey={key}
              clients={clients}
              nodes={nodes}
              assignees={assignees}
              statusId={status_id ? Number(status_id) : undefined}
              nodeId={node_id ? Number(node_id) : undefined}
            />
          </div>
        )}
      </main>
    </>
  );
}
