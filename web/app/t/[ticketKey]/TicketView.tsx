"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { Avatar, ClientChip, PriorityChip, StatusDot, TypeIcon, showsClients } from "@/components/Chips";
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
import Code from "./Code";
import DecisionCard from "./DecisionCard";
import Links from "./Links";

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
  const tf = useTranslations("ticketForm");
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

  const block = "flex flex-col gap-2 border-b border-line-soft px-5 py-4 last:border-0";
  const person = "size-6 bg-accent-soft text-[10px] font-bold text-accent-strong";
  return (
    <>
      <PageBar>
        <nav aria-label={t("path")} className="flex items-center gap-1.5 text-[13px] text-muted">
          <Link href={`/p/${ticket.project_key}/board`}>{ticket.project_key}</Link>
          <Icon name="chevronRight" className="size-3.5" />
          <Link href={`/p/${ticket.project_key}/tickets`}>{tp("tickets")}</Link>
          <Icon name="chevronRight" className="size-3.5" />
          <span className="font-bold text-ink">{ticket.key}</span>
        </nav>
        <Link href={askAbout(ticket)} className={cx(button.secondary, "ml-auto")}>
          <Icon name="sparkle" className="size-4 text-accent" />
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
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-2.5 text-[13px] text-ink-soft">
            <span className="font-bold text-muted">{ticket.key}</span>
            <label className="flex h-9 items-center gap-2 rounded-full border border-line bg-white pl-3.5 shadow-[0_1px_2px_rgba(43,36,32,0.04)] focus-within:border-accent">
              <StatusDot color={ticket.status.color} />
              <span className="sr-only">{t("status")}</span>
              <select
                value={ticket.status.id}
                disabled={!canEdit}
                onChange={(e) => transition(Number(e.target.value))}
                className="h-full cursor-pointer rounded-full bg-transparent text-[13.5px] font-bold text-ink outline-none disabled:cursor-default"
              >
                {statuses.map((s) => (
                  <option key={s.id} value={s.id}>{s.name}</option>
                ))}
              </select>
            </label>
            <span className="flex items-center gap-1.5 font-semibold">
              <TypeIcon type={ticket.type} label={tTypes(ticket.type)} />
              {tTypes(ticket.type)}
            </span>
            {showsClients(clients) && <ClientChip client={ticket.client} coreLabel={t("core")} />}
            {(ticket.priority === "high" || ticket.priority === "urgent") && <PriorityChip priority={ticket.priority} label={tPri(ticket.priority)} />}
          </div>
          <h1 className="max-w-4xl text-[28px] font-extrabold leading-tight tracking-[-0.02em] [text-wrap:pretty]">{ticket.title}</h1>
        </div>
        {error && <p role="alert" className={field.error}>{error}</p>}
        {!closing(ticket.status) && (!ticket.reason || ticket.nodes.length === 0) && (
          <p className="flex items-center gap-2 rounded-xl border border-warn-line bg-warn-soft px-3.5 py-2.5 text-[13px] font-medium text-warn">
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
          <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_340px]">
            <div className="flex min-w-0 flex-col gap-4">
              <section aria-labelledby="reason-title" className={cx(panel, "flex flex-col gap-2.5 px-5 py-4")}>
                <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
                  <span className="flex size-7 items-center justify-center rounded-lg bg-accent-soft text-accent">
                    <Icon name="message" />
                  </span>
                  <h2 id="reason-title" className="text-sm font-extrabold">{t("reason")}</h2>
                  <span className="text-[12.5px] text-muted">
                    {t("requestedBy")} {ticket.requester.name}{ticket.requester.title ? ` (${ticket.requester.title})` : ""}
                  </span>
                </div>
                {ticket.reason ? (
                  <p className="whitespace-pre-wrap text-[17px] font-semibold leading-relaxed tracking-[-0.01em] [text-wrap:pretty]">{ticket.reason}</p>
                ) : (
                  <p className="text-sm text-muted">{t("none")}</p>
                )}
              </section>
              {ticket.decision && <DecisionCard ticketKey={ticket.key} decision={ticket.decision} canEdit={canEditDecision} />}
              <section aria-label={t("details")} className={panel}>
                <div className={block}>
                  <h2 className={sectionTitle}>{t("menus")}</h2>
                  {ticket.nodes.length === 0 ? (
                    <p className="text-sm text-muted">{t("none")}</p>
                  ) : (
                    <ul className="flex flex-wrap gap-2">
                      {ticket.nodes.map((n) => (
                        <li key={n.id}>
                          <Link
                            href={`/p/${ticket.project_key}/modules/${n.id}`}
                            className="inline-flex h-8 items-center gap-2 rounded-[10px] bg-well px-3 text-[13.5px] font-semibold text-ink no-underline hover:bg-line-soft hover:text-ink"
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
                <div className={block}>
                  <h2 className={sectionTitle}>{t("description")}</h2>
                  {ticket.description ? <Markdown text={ticket.description} /> : <p className="text-sm text-muted">{t("none")}</p>}
                </div>
              </section>
              <Links ticketKey={ticket.key} links={ticket.links} canEdit={canEdit} />
              {ticket.code && <Code code={ticket.code} />}
              {activity}
            </div>
            <aside className="flex flex-col gap-4">
              <section aria-label={t("people")} className={cx(panel, "flex flex-col gap-3.5 px-5 py-4")}>
                <dl className="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-4 gap-y-3 text-[13.5px] leading-snug">
                  <dt className="text-muted">{t("requestedBy")}</dt>
                  <dd className="flex min-w-0 items-center gap-2">
                    <Avatar name={ticket.requester.name} className={person} />
                    <span className="min-w-0">
                      <span className="block font-semibold">{ticket.requester.name}</span>
                      {ticket.requester.title && <span className="block text-xs text-muted">{ticket.requester.title}</span>}
                    </span>
                  </dd>
                  <dt className="text-muted">{t("reporter")}</dt>
                  <dd className="flex min-w-0 items-center gap-2">
                    <Avatar name={ticket.reporter.name} className={person} />
                    <span className="font-semibold">{ticket.reporter.name}</span>
                  </dd>
                  <dt className="text-muted">{t("assignee")}</dt>
                  <dd className="flex min-w-0 items-center gap-2">
                    {ticket.assignee ? (
                      <>
                        <Avatar name={ticket.assignee.name} className={person} />
                        <span className="font-semibold">{ticket.assignee.name}</span>
                      </>
                    ) : (
                      <span className="text-muted">{t("nobody")}</span>
                    )}
                  </dd>
                  <dt className="text-muted">{tf("priority")}</dt>
                  <dd>
                    <PriorityChip priority={ticket.priority} label={tPri(ticket.priority)} />
                  </dd>
                  {ticket.due_date && (
                    <>
                      <dt className="text-muted">{t("due")}</dt>
                      <dd className="font-semibold">{day(ticket.due_date, locale)}</dd>
                    </>
                  )}
                  <dt className="text-muted">{t("created")}</dt>
                  <dd>{utc(ticket.created_at, locale)}</dd>
                  {ticket.closed_at && (
                    <>
                      <dt className="text-muted">{t("closed")}</dt>
                      <dd>{utc(ticket.closed_at, locale)}</dd>
                    </>
                  )}
                </dl>
                <p className="border-t border-line-soft pt-3 text-xs text-muted">
                  {t("updated")} {utc(ticket.updated_at, locale)}
                </p>
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
