"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { Avatar, ClientChip, PriorityChip, StatusDot, TypeIcon } from "@/components/Chips";
import CloseDialog from "@/components/CloseDialog";
import Icon from "@/components/Icon";
import Markdown from "@/components/Markdown";
import PageBar from "@/components/PageBar";
import TicketForm from "@/components/TicketForm";
import { api } from "@/lib/api";
import { day, utc } from "@/lib/format";
import { nodePaths } from "@/lib/nodes";
import { useProblemText, type Client, type Node, type Ref, type Status, type Ticket } from "@/lib/problem";
import { button, cx, field, panel, sectionTitle } from "@/lib/ui";
import DecisionCard from "./DecisionCard";

type Props = {
  ticket: Ticket;
  statuses: Status[];
  clients: Client[];
  nodes: Node[];
  assignees: Ref[];
  canEdit: boolean;
  canEditDecision: boolean; // project admin or the record's confirmer (R-DC-5)
  activity: React.ReactNode;
  attachments: React.ReactNode;
};

const closing = (s: Status) => s.category === "done" || s.category === "cancelled";

export default function TicketView({ ticket, statuses, clients, nodes, assignees, canEdit, canEditDecision, activity, attachments }: Props) {
  const t = useTranslations("ticket");
  const tp = useTranslations("project");
  const ta = useTranslations("ask");
  const tTypes = useTranslations("ticketTypes");
  const tPri = useTranslations("priorities");
  const locale = useLocale();
  const problemText = useProblemText();
  const router = useRouter();
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState("");
  const [closingTo, setClosingTo] = useState<Status | null>(null);
  const pathOf = useMemo(() => nodePaths(nodes), [nodes]);

  // Open moves go straight through; Done and Cancelled open the close dialog (FSD §9.1).
  async function transition(statusId: number) {
    const target = statuses.find((s) => s.id === statusId);
    if (!target || target.id === ticket.status.id) return;
    if (closing(target)) return setClosingTo(target);
    const { error } = await api.POST("/tickets/{key}/transition", {
      params: { path: { key: ticket.key } },
      body: { status_id: statusId },
    });
    if (error) return setError(problemText(error));
    setError("");
    router.refresh();
  }

  const block = "flex flex-col gap-1.5 border-b border-line-soft px-4 py-3.5 last:border-0";
  return (
    <>
      <PageBar>
        <nav aria-label={t("path")} className="flex items-center gap-1.5 text-[13px] text-muted">
          <Link href={`/p/${ticket.project_key}/board`}>{ticket.project_key}</Link>
          <Icon name="chevronRight" className="size-3.5" />
          <Link href={`/p/${ticket.project_key}/tickets`}>{tp("tickets")}</Link>
          <Icon name="chevronRight" className="size-3.5" />
          <span className="font-mono font-semibold text-ink">{ticket.key}</span>
        </nav>
        <Link href={askAbout(ticket)} className={cx(button.secondary, "ml-auto")}>
          {ta("askAboutTicket")}
        </Link>
        {canEdit && !editing && (
          <button type="button" onClick={() => setEditing(true)} className={button.secondary}>
            <Icon name="edit" />
            {t("edit")}
          </button>
        )}
      </PageBar>
      <main className="flex flex-col gap-4 px-4 py-4 md:px-5">
        <div className="flex flex-col gap-2.5">
          <div className="flex flex-wrap items-center gap-2 text-[13px]">
            <span className="font-mono font-semibold text-muted">{ticket.key}</span>
            <label className="flex h-8 items-center gap-2 rounded border border-line bg-white pl-2.5 focus-within:outline-2 focus-within:outline-accent">
              <StatusDot color={ticket.status.color} />
              <span className="sr-only">{t("status")}</span>
              <select
                value={ticket.status.id}
                disabled={!canEdit}
                onChange={(e) => transition(Number(e.target.value))}
                className="h-full cursor-pointer bg-transparent pr-2 font-semibold outline-none disabled:cursor-default"
              >
                {statuses.map((s) => (
                  <option key={s.id} value={s.id}>{s.name}</option>
                ))}
              </select>
            </label>
            <span className="flex items-center gap-1.5">
              <TypeIcon type={ticket.type} label={tTypes(ticket.type)} />
              {tTypes(ticket.type)}
            </span>
            <ClientChip client={ticket.client} coreLabel={t("core")} />
            <PriorityChip priority={ticket.priority} label={tPri(ticket.priority)} />
            {ticket.assignee && (
              <span className="flex items-center gap-1.5">
                <Avatar name={ticket.assignee.name} className="size-5 bg-well text-[9px] text-ink" />
                {ticket.assignee.name}
              </span>
            )}
            {ticket.due_date && (
              <span className="text-muted">
                {t("due")} {day(ticket.due_date, locale)}
              </span>
            )}
          </div>
          <h1 className="text-2xl font-semibold leading-tight">{ticket.title}</h1>
        </div>
        {error && <p role="alert" className={field.error}>{error}</p>}
        {!closing(ticket.status) && (!ticket.reason || ticket.nodes.length === 0) && (
          <p className="flex items-center gap-2 rounded border border-warn-line bg-warn-soft px-3 py-2 text-[13px] text-warn">
            <Icon name="warning" />
            {t("missingClose")}
          </p>
        )}
        {editing ? (
          <div className={panel}>
            <TicketForm
              projectKey={ticket.project_key}
              ticket={ticket}
              clients={clients}
              nodes={nodes}
              assignees={assignees}
              onSaved={() => {
                setEditing(false);
                router.refresh();
              }}
              onCancel={() => setEditing(false)}
            />
          </div>
        ) : (
          <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_340px]">
            <div className="flex min-w-0 flex-col gap-4">
              <section aria-label={t("details")} className={panel}>
                <div className={block}>
                  <h2 className={sectionTitle}>{t("description")}</h2>
                  {ticket.description ? <Markdown text={ticket.description} /> : <p className="text-sm text-muted">{t("none")}</p>}
                </div>
                <div className={block}>
                  <h2 className={sectionTitle}>{t("reason")}</h2>
                  <p className="whitespace-pre-wrap text-sm leading-relaxed">{ticket.reason || <span className="text-muted">{t("none")}</span>}</p>
                </div>
                <div className={block}>
                  <h2 className={sectionTitle}>{t("menus")}</h2>
                  {ticket.nodes.length === 0 ? (
                    <p className="text-sm text-muted">{t("none")}</p>
                  ) : (
                    <ul className="flex flex-wrap gap-1.5">
                      {ticket.nodes.map((n) => (
                        <li key={n.id}>
                          <Link
                            href={`/p/${ticket.project_key}/modules/${n.id}`}
                            className="inline-flex h-7 items-center gap-1.5 rounded border border-line bg-ground px-2.5 text-[13px] text-ink no-underline hover:border-field hover:text-ink"
                          >
                            <Icon name="screen" className="size-3.5 text-muted" />
                            {pathOf(n.id) || n.name}
                            {n.archived ? ` (${t("archived")})` : ""}
                          </Link>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </section>
              {ticket.decision && <DecisionCard ticketKey={ticket.key} decision={ticket.decision} canEdit={canEditDecision} />}
              {activity}
            </div>
            <aside className="flex flex-col gap-4">
              <section aria-label={t("people")} className={cx(panel, "px-4 py-3.5")}>
                <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3.5 gap-y-2.5 text-[13px] leading-snug">
                  <dt className="text-muted">{t("requestedBy")}</dt>
                  <dd>{ticket.requester.name}{ticket.requester.title ? ` (${ticket.requester.title})` : ""}</dd>
                  <dt className="text-muted">{t("reporter")}</dt>
                  <dd>{ticket.reporter.name}</dd>
                  <dt className="text-muted">{t("assignee")}</dt>
                  <dd>{ticket.assignee?.name ?? t("nobody")}</dd>
                  <dt className="text-muted">{t("created")}</dt>
                  <dd>{utc(ticket.created_at, locale)}</dd>
                  <dt className="text-muted">{t("updated")}</dt>
                  <dd>{utc(ticket.updated_at, locale)}</dd>
                  {ticket.closed_at && (
                    <>
                      <dt className="text-muted">{t("closed")}</dt>
                      <dd>{utc(ticket.closed_at, locale)}</dd>
                    </>
                  )}
                </dl>
              </section>
              {attachments}
            </aside>
          </div>
        )}
        {closingTo && (
          <CloseDialog
            ticket={ticket}
            status={closingTo}
            nodes={nodes}
            onDone={() => {
              setClosingTo(null);
              router.refresh();
            }}
            onCancel={() => setClosingTo(null)}
          />
        )}
      </main>
    </>
  );
}

// "Ask about this ticket" (FSD §10.1): the Ask page preset to the ticket's
// project, menus and client.
function askAbout(ticket: Ticket) {
  const q = new URLSearchParams({ project: ticket.project_key });
  for (const n of ticket.nodes) q.append("node", String(n.id));
  if (ticket.client) q.set("client", String(ticket.client.id));
  return `/ask?${q}`;
}
