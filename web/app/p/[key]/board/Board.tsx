"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Status, type TicketSummary } from "@/lib/problem";

type Props = { projectKey: string; statuses: Status[]; tickets: TicketSummary[]; canEdit: boolean; today: string };

const closing = (s: Status) => s.category === "done" || s.category === "cancelled";

// Cards move by native drag-and-drop or by their status menu, which keyboards
// reach too. A move shows at once and rolls back when the API refuses it (FSD §8.4).
export default function Board({ projectKey, statuses, tickets, canEdit, today }: Props) {
  const t = useTranslations("board");
  const tTypes = useTranslations("ticketTypes");
  const tPri = useTranslations("priorities");
  const problemText = useProblemText();
  const router = useRouter();
  const [items, setItems] = useState(tickets);
  const [dragging, setDragging] = useState<number | null>(null);
  const [error, setError] = useState("");
  useEffect(() => setItems(tickets), [tickets]); // fresh server data wins

  async function move(ticketId: number, statusId: number) {
    const card = items.find((x) => x.id === ticketId);
    if (!card || card.status_id === statusId) return;
    const before = items;
    setItems((xs) => xs.map((x) => (x.id === ticketId ? { ...x, status_id: statusId } : x)));
    const { error } = await api.POST("/tickets/{key}/transition", {
      params: { path: { key: card.key } },
      body: { status_id: statusId },
    });
    if (error) {
      setItems(before);
      return setError(problemText(error));
    }
    setError("");
    router.refresh();
  }

  return (
    <div className="flex flex-col gap-2">
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <div className="flex gap-3 overflow-x-auto pb-2">
        {statuses.map((s) => {
          const cards = items.filter((x) => x.status_id === s.id);
          const droppable = canEdit && !closing(s);
          return (
            <section
              key={s.id}
              aria-label={s.name}
              className="flex w-72 shrink-0 flex-col gap-2 rounded-lg bg-neutral-100 p-2"
              onDragOver={droppable ? (e) => e.preventDefault() : undefined}
              onDrop={
                droppable
                  ? (e) => {
                      e.preventDefault();
                      if (dragging !== null) move(dragging, s.id);
                      setDragging(null);
                    }
                  : undefined
              }
            >
              <h2 className="flex items-center gap-2 text-sm font-medium">
                <span className="h-2 w-2 rounded-full" style={{ background: s.color }} />
                {s.name}
                <span className="text-neutral-500">{cards.length}</span>
                {droppable && (
                  <Link href={`/p/${projectKey}/tickets/new?status_id=${s.id}`} aria-label={t("addHere", { status: s.name })} className="ml-auto px-1">+</Link>
                )}
              </h2>
              {closing(s) && <p className="text-xs text-neutral-500">{t("closedLater")}</p>}
              {cards.map((c) => (
                <article
                  key={c.id}
                  draggable={canEdit}
                  onDragStart={() => setDragging(c.id)}
                  onDragEnd={() => setDragging(null)}
                  className="rounded bg-white p-2 text-sm shadow-sm"
                >
                  <div className="flex items-center gap-2 text-xs text-neutral-500">
                    <span className="font-mono">{c.key}</span>
                    <span>{tTypes(c.type)}</span>
                    {(c.missing_reason || c.node_names.length === 0) && (
                      <span title={t("missing")} aria-label={t("missing")} className="text-amber-600">●</span>
                    )}
                    <span className="ml-auto">{tPri(c.priority)}</span>
                  </div>
                  <Link href={`/t/${c.key}`} className="mt-1 block font-medium hover:underline">{c.title}</Link>
                  <div className="mt-1 flex flex-wrap gap-2 text-xs">
                    <span className="rounded bg-neutral-100 px-1.5">{c.client?.name ?? t("noClient")}</span>
                    {c.assignee && <span>{c.assignee.name}</span>}
                    {c.due_date && <span className={c.due_date < today ? "text-red-700" : ""}>{c.due_date}</span>}
                  </div>
                  {canEdit && (
                    <select
                      aria-label={t("moveTo", { key: c.key })}
                      value={c.status_id}
                      onChange={(e) => move(c.id, Number(e.target.value))}
                      className="mt-2 w-full rounded border px-1 py-0.5 text-xs"
                    >
                      {statuses.map((o) => (
                        <option key={o.id} value={o.id} disabled={closing(o)}>{o.name}</option>
                      ))}
                    </select>
                  )}
                </article>
              ))}
            </section>
          );
        })}
      </div>
    </div>
  );
}
