import { Fragment } from "react";
import Link from "next/link";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import AskView from "@/components/ask/AskView";
import { ClientChip } from "@/components/Chips";
import Icon from "@/components/Icon";
import type { TicketType } from "@/lib/problem";
import { getProject, serverApi } from "@/lib/server-api";
import { one } from "@/lib/ticket-query";
import { button, chip, cx } from "@/lib/ui";
import Behaviors from "./Behaviors";
import NodeDetails from "./NodeDetails";
import Timeline from "./Timeline";

const tabs = ["timeline", "behaviors", "ask", "details"] as const;
type Tab = (typeof tabs)[number];

// The node page (FSD §7.4): a menu's or module's history, the behaviors each
// client has now, and its details. The tab, the filters and the page size live
// in the URL, so every view is shareable. Archived nodes keep their page (R-MR-4).
export default async function NodePage({
  params,
  searchParams,
}: {
  params: Promise<{ key: string; nodeId: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const { key, nodeId } = await params;
  const values = one(await searchParams);
  const id = Number(nodeId);
  const project = await getProject(key);
  if (!project || !Number.isInteger(id)) notFound();
  const api = await serverApi();
  const { data: detail } = await api.GET("/nodes/{id}", { params: { path: { id } } });
  if (!detail || detail.project_key !== project.key) notFound();
  const t = await getTranslations("nodePage");
  const tm = await getTranslations("modules");
  const tab: Tab = tabs.includes(values.tab as Tab) ? (values.tab as Tab) : "timeline";
  const node = detail.node;
  const subNodes = values.sub === "0" ? false : undefined;
  const clientsOf = async () => (await api.GET("/projects/{key}/clients", { params: { path: { key } } })).data?.items ?? [];

  let content: React.ReactNode;
  if (tab === "timeline") {
    const limit = Math.min(Math.max(Number(values.limit) || 50, 1), 1000);
    const [clients, timeline] = await Promise.all([
      clientsOf(),
      api.GET("/nodes/{id}/timeline", {
        params: {
          path: { id },
          query: {
            sub_nodes: subNodes,
            client_id: values.client && values.client !== "core" ? Number(values.client) : undefined,
            core: values.client === "core" ? true : undefined,
            type: values.type as TicketType | undefined,
            from: values.from,
            to: values.to,
            limit,
          },
        },
      }),
    ]);
    content = (
      <Timeline
        items={timeline.data?.items ?? []}
        notes={timeline.data?.notes ?? []}
        sections={timeline.data?.sections ?? []}
        failed={Boolean(timeline.error)}
        more={Boolean(timeline.data?.next_cursor)}
        limit={limit}
        clients={clients}
        values={values}
      />
    );
  } else if (tab === "ask") {
    // Preset to this node and its sub-nodes (FSD §10.1).
    const label = [...detail.path.map((p) => p.name), node.name].join(" › ");
    content = (
      <AskView
        chips={[
          { kind: "project", id: project.id, label: project.key },
          { kind: "node", id: node.id, label },
        ]}
      />
    );
  } else if (tab === "behaviors") {
    const { data } = await api.GET("/nodes/{id}/behaviors", { params: { path: { id }, query: { sub_nodes: subNodes } } });
    content = <Behaviors items={data?.items ?? []} />;
  } else {
    const canEdit = project.role === "admin";
    const others = canEdit ? ((await api.GET("/projects/{key}/nodes", { params: { path: { key } } })).data?.items ?? []) : [];
    content = <NodeDetails projectKey={key} node={node} clients={canEdit ? await clientsOf() : []} canEdit={canEdit} nodes={others} />;
  }

  return (
    <>
      <div className="flex flex-col gap-3 px-4 pt-1 md:px-5">
        <nav aria-label={t("path")} className="flex flex-wrap items-center gap-1.5 text-[13px] text-muted">
          <Link href={`/p/${key}/modules`}>{t("modules")}</Link>
          {detail.path.map((p) => (
            <Fragment key={p.id}>
              <Icon name="chevronRight" className="size-3.5" />
              <Link href={`/p/${key}/modules/${p.id}`}>{p.name}</Link>
            </Fragment>
          ))}
          <Icon name="chevronRight" className="size-3.5" />
          <span className="font-semibold text-ink">{node.name}</span>
        </nav>
        <div className="flex flex-wrap items-start gap-4">
          <div className="flex min-w-0 flex-col gap-2">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="text-[32px] font-extrabold leading-tight tracking-[-0.03em]">{node.name}</h1>
              <span className="text-[13px] font-semibold text-muted">{tm(node.type)}</span>
              {node.type === "menu" &&
                (node.client_specific ? (
                  node.clients.map((c) => <ClientChip key={c.id} client={c} coreLabel="" />)
                ) : (
                  <span className={cx(chip, "bg-well text-ink-soft")}>{tm("shared")}</span>
                ))}
              {node.code && <span className="font-mono text-xs font-medium text-muted">{node.code}</span>}
              {node.archived && <span className={cx(chip, "bg-well text-muted")}>{tm("archivedBadge")}</span>}
            </div>
            {(node.description || node.aliases.length > 0) && (
              <p className="max-w-[760px] text-[14.5px] leading-relaxed text-ink-soft">
                {node.description}{" "}
                {node.aliases.length > 0 && <span className="text-muted">{t("aliases", { aliases: node.aliases.join(", ") })}</span>}
              </p>
            )}
          </div>
          {project.role !== "viewer" && !node.archived && (
            <Link href={`/p/${key}/tickets/new?node_id=${node.id}`} className={cx(button.primary, "ml-auto")}>
              <Icon name="plus" />
              {t("newTicket")}
            </Link>
          )}
        </div>
        <nav aria-label={t("tabs")} className="flex gap-1 border-b border-line">
          {tabs.map((to) => (
            <Link
              key={to}
              href={to === "timeline" ? "?" : `?tab=${to}`}
              aria-current={tab === to ? "page" : undefined}
              className={cx(
                "-mb-px border-b-[2.5px] px-3.5 py-2.5 text-sm no-underline",
                tab === to ? "border-accent font-extrabold text-ink hover:text-ink" : "border-transparent font-semibold text-ink-soft hover:text-ink",
              )}
            >
              {t(to)}
            </Link>
          ))}
        </nav>
      </div>
      <main className="flex flex-col gap-5 px-4 pb-8 pt-5 md:px-5">{content}</main>
    </>
  );
}
