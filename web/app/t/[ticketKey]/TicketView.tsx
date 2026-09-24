"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import TicketForm from "@/components/TicketForm";
import { api } from "@/lib/api";
import { utc } from "@/lib/format";
import { useProblemText, type Client, type Node, type Ref, type Status, type Ticket } from "@/lib/problem";

type Props = { ticket: Ticket; statuses: Status[]; clients: Client[]; nodes: Node[]; assignees: Ref[]; canEdit: boolean };

const closing = (s: Status) => s.category === "done" || s.category === "cancelled";

export default function TicketView({ ticket, statuses, clients, nodes, assignees, canEdit }: Props) {
  const t = useTranslations("ticket");
  const tTypes = useTranslations("ticketTypes");
  const tPri = useTranslations("priorities");
  const problemText = useProblemText();
  const router = useRouter();
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState("");
  const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);
  const pathOf = (id: number) => {
    const names: string[] = [];
    for (let n = byId.get(id); n; n = n.parent_id === null ? undefined : byId.get(n.parent_id)) names.unshift(n.name);
    return names.join(" › ");
  };

  async function transition(statusId: number) {
    const { error } = await api.POST("/tickets/{key}/transition", {
      params: { path: { key: ticket.key } },
      body: { status_id: statusId },
    });
    if (error) return setError(problemText(error));
    setError("");
    router.refresh();
  }

  return (
    <div className="mt-2">
      <div className="flex flex-wrap items-baseline gap-3">
        <span className="font-mono text-neutral-500">{ticket.key}</span>
        <h1 className="text-2xl font-semibold">{ticket.title}</h1>
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-4 text-sm">
        <label className="flex items-center gap-2">
          {t("status")}
          <select
            value={ticket.status.id}
            disabled={!canEdit}
            onChange={(e) => transition(Number(e.target.value))}
            className="rounded border px-2 py-1"
          >
            {statuses.map((s) => (
              <option key={s.id} value={s.id} disabled={closing(s)}>{s.name}</option>
            ))}
          </select>
        </label>
        <span>{tTypes(ticket.type)}</span>
        <span className="rounded bg-neutral-100 px-2">{ticket.client?.name ?? t("core")}</span>
        <span>{tPri(ticket.priority)}</span>
        <span>{t("assignee")}: {ticket.assignee?.name ?? t("nobody")}</span>
        {ticket.due_date && <span>{t("due")}: {ticket.due_date}</span>}
        {canEdit && !editing && (
          <button type="button" onClick={() => setEditing(true)} className="underline">{t("edit")}</button>
        )}
      </div>
      {error && <p role="alert" className="mt-2 text-sm text-red-700">{error}</p>}
      {(!ticket.reason || ticket.nodes.length === 0) && (
        <p className="mt-3 rounded border border-amber-300 bg-amber-50 p-2 text-sm">{t("missingClose")}</p>
      )}
      {editing ? (
        <div className="mt-4 rounded-lg border bg-white p-4">
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
        <div className="mt-6 grid gap-6 md:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
          <div className="flex flex-col gap-4">
            <section>
              <h2 className="text-sm font-medium text-neutral-500">{t("description")}</h2>
              <p className="whitespace-pre-wrap">{ticket.description || t("none")}</p>
            </section>
            <section>
              <h2 className="text-sm font-medium text-neutral-500">{t("reason")}</h2>
              <p className="whitespace-pre-wrap">{ticket.reason || t("none")}</p>
            </section>
            <section>
              <h2 className="text-sm font-medium text-neutral-500">{t("menus")}</h2>
              <ul className="mt-1 flex flex-wrap gap-2">
                {ticket.nodes.map((n) => (
                  <li key={n.id} title={pathOf(n.id)} className="rounded bg-neutral-100 px-2 py-0.5 text-sm">
                    {n.name}
                    {n.archived ? ` (${t("archived")})` : ""}
                  </li>
                ))}
              </ul>
            </section>
          </div>
          <dl className="grid grid-cols-[auto_1fr] content-start gap-x-3 gap-y-2 text-sm">
            <dt className="text-neutral-500">{t("requestedBy")}</dt>
            <dd>{ticket.requester.name}{ticket.requester.title ? ` (${ticket.requester.title})` : ""}</dd>
            <dt className="text-neutral-500">{t("reporter")}</dt>
            <dd>{ticket.reporter.name}</dd>
            <dt className="text-neutral-500">{t("created")}</dt>
            <dd>{utc(ticket.created_at)}</dd>
            <dt className="text-neutral-500">{t("updated")}</dt>
            <dd>{utc(ticket.updated_at)}</dd>
          </dl>
        </div>
      )}
    </div>
  );
}
