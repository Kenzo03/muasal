import Link from "next/link";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import Icon from "@/components/Icon";
import PageBar from "@/components/PageBar";
import TicketForm from "@/components/TicketForm";
import { getProject, serverApi } from "@/lib/server-api";
import { panel } from "@/lib/ui";

export default async function NewTicketPage({
  params,
  searchParams,
}: {
  params: Promise<{ key: string }>;
  searchParams: Promise<{ status_id?: string }>;
}) {
  const { key } = await params;
  const { status_id } = await searchParams;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("ticketForm");
  const tp = await getTranslations("project");
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, nodes, assignees] = await Promise.all([
    api.GET("/projects/{key}/clients", path),
    api.GET("/projects/{key}/nodes", path),
    api.GET("/projects/{key}/assignees", path),
  ]);
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
              clients={clients.data?.items ?? []}
              nodes={nodes.data?.items ?? []}
              assignees={assignees.data?.items ?? []}
              statusId={status_id ? Number(status_id) : undefined}
            />
          </div>
        )}
      </main>
    </>
  );
}
