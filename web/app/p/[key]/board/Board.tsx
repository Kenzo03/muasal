"use client";

import { useEffect, useState } from "react";
import { DndContext, DragOverlay, PointerSensor, useDraggable, useDroppable, useSensor, useSensors } from "@dnd-kit/core";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { Avatar, ClientChip, PriorityChip, StatusDot, TypeIcon } from "@/components/Chips";
import CloseDialog from "@/components/CloseDialog";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import { day } from "@/lib/format";
import { useProblemText, type Node, type Status, type Ticket, type TicketSummary } from "@/lib/problem";
import { cx, field } from "@/lib/ui";

type Props = {
  projectKey: string;
  statuses: Status[];
  tickets: TicketSummary[];
  nodes: Node[];
  canEdit: boolean;
  showClients: boolean;
  today: string;
  query: Record<string, string>; // the page's filters, kept by the "Show all" link
};

const closes = (s: Status) => s.category === "done" || s.category === "cancelled";

// A status column that takes dropped cards.
function Column({ status, canEdit, children }: { status: Status; canEdit: boolean; children: React.ReactNode }) {
  const { setNodeRef, isOver } = useDroppable({ id: `status-${status.id}`, data: { statusId: status.id }, disabled: !canEdit });
  return (
    <section
      ref={setNodeRef}
      aria-label={status.name}
      className={cx("flex min-w-[220px] flex-1 basis-0 flex-col gap-2 self-start rounded-2xl bg-sidebar p-2", isOver && "outline-2 -outline-offset-2 outline-accent")}
    >
      {children}
    </section>
  );
}

// A card that the pointer drags. Keyboards use its status menu instead, so the
// card itself stays a plain article with a link inside.
function Card({ id, canEdit, className, children }: { id: number; canEdit: boolean; className: string; children: React.ReactNode }) {
  const { setNodeRef, listeners, isDragging } = useDraggable({ id, disabled: !canEdit });
  return (
    <article ref={setNodeRef} {...listeners} className={cx(className, canEdit && "cursor-grab touch-none", isDragging && "opacity-40")}>
      {children}
    </article>
  );
}

// Cards move by dragging (dnd-kit) or by their status menu, which keyboards
// reach too. An open move shows at once and rolls back when the API refuses it;
// a move into Done or Cancelled opens the close dialog, and the card stays
// where it was until the close is confirmed (FSD §8.4, AC-TK-2).
export default function Board({ projectKey, statuses, tickets, nodes, canEdit, showClients, today, query }: Props) {
  const t = useTranslations("board");
  const tTypes = useTranslations("ticketTypes");
  const tPri = useTranslations("priorities");
  const locale = useLocale();
  const problemText = useProblemText();
  const router = useRouter();
  const [items, setItems] = useState(tickets);
  const [dragging, setDragging] = useState<number | null>(null);
  const [error, setError] = useState("");
  // A short move before a drag starts, so clicks on the title and the status menu still work.
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }));
  const draggedCard = dragging === null ? undefined : items.find((x) => x.id === dragging);
  const [closing, setClosing] = useState<{ ticket: Ticket; status: Status } | null>(null);
  const showAll = query.closed === "all";
  const { closed: _closed, ...recent } = query;
  useEffect(() => setItems(tickets), [tickets]); // fresh server data wins

  async function move(ticketId: number, statusId: number) {
    const card = items.find((x) => x.id === ticketId);
    const target = statuses.find((s) => s.id === statusId);
    if (!card || !target || card.status_id === statusId) return;
    if (closes(target)) {
      const { data, error } = await api.GET("/tickets/{key}", { params: { path: { key: card.key } } });
      if (error) return setError(problemText(error));
      return setClosing({ ticket: data, status: target });
    }
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
      {error && <p role="alert" className={field.error}>{error}</p>}
      <DndContext
        sensors={sensors}
        onDragStart={(e) => setDragging(Number(e.active.id))}
        onDragCancel={() => setDragging(null)}
        onDragEnd={(e) => {
          setDragging(null);
          const statusId = e.over?.data.current?.statusId as number | undefined;
          if (statusId !== undefined) move(Number(e.active.id), statusId);
        }}
      >
        <div className="flex gap-3 overflow-x-auto pb-2">
          {statuses.map((s) => {
            const cards = items.filter((x) => x.status_id === s.id);
            const droppable = canEdit;
            return (
              <Column key={s.id} status={s} canEdit={canEdit}>
                <h2 className="flex h-9 items-center gap-2 pl-2 pr-0.5 text-sm font-extrabold">
                  <StatusDot color={s.color} className="size-2.5" />
                  <span className="truncate">{s.name}</span>
                  <span className="text-[13px] font-bold text-muted">{cards.length}</span>
                  {droppable && (
                    <Link
                      href={`/p/${projectKey}/tickets/new?status_id=${s.id}`}
                      aria-label={t("addHere", { status: s.name })}
                      className="ml-auto inline-flex size-7 items-center justify-center rounded-lg text-ink-soft hover:bg-white hover:text-ink"
                    >
                      <Icon name="plus" />
                    </Link>
                  )}
                </h2>
                {closes(s) && (
                  <p className="flex flex-wrap items-center gap-x-2 gap-y-1 px-2 pb-1 text-[13px] text-ink-soft">
                    {showAll ? t("allClosed") : t("recentClosed")}
                    <Link href={`?${new URLSearchParams(showAll ? recent : { ...query, closed: "all" })}`} className="ml-auto">
                      {showAll ? t("showRecent") : t("showAll")}
                    </Link>
                  </p>
                )}
                {cards.map((c) => (
                  <Card
                    key={c.id}
                    id={c.id}
                    canEdit={canEdit}
                    className="flex flex-col gap-2 rounded-[14px] bg-white p-3.5 shadow-[0_1px_2px_rgba(43,36,32,0.06),0_0_0_1px_rgba(43,36,32,0.04)]"
                  >
                    <div className="flex items-center gap-1.5 text-xs font-bold text-muted">
                      <TypeIcon type={c.type} label={tTypes(c.type)} />
                      <span>{c.key}</span>
                      {(c.missing_reason || c.node_names.length === 0) && (
                        <span role="img" title={t("missing")} aria-label={t("missing")} className="size-[7px] rounded-full bg-[#D97706]" />
                      )}
                      {(c.priority === "high" || c.priority === "urgent") && <PriorityChip priority={c.priority} label={tPri(c.priority)} />}
                      {c.assignee && <Avatar name={c.assignee.name} className="ml-auto size-6 bg-accent-soft text-[10px] font-bold text-accent-strong" />}
                    </div>
                    <Link href={`/t/${c.key}`} className="text-sm font-bold leading-snug text-ink no-underline hover:text-ink hover:underline">
                      {c.title}
                    </Link>
                    {c.node_names.length > 0 && (
                      <span className="flex min-w-0 items-center gap-1.5 text-[12.5px] text-muted">
                        <Icon name="screen" className="size-3.5" />
                        <span className="truncate">
                          {c.node_names[0]}
                          {c.node_names.length > 1 ? ` +${c.node_names.length - 1}` : ""}
                        </span>
                      </span>
                    )}
                    <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted">
                      {showClients && <ClientChip client={c.client} coreLabel={t("noClient")} />}
                      {c.due_date && (
                        <span className={c.due_date < today ? "font-semibold text-danger" : ""}>{day(c.due_date, locale, c.due_date.slice(0, 4) !== today.slice(0, 4))}</span>
                      )}
                      {canEdit && (
                        <select
                          aria-label={t("moveTo", { key: c.key })}
                          value={c.status_id}
                          onChange={(e) => move(c.id, Number(e.target.value))}
                          className="ml-auto h-7 max-w-32 cursor-pointer rounded-lg bg-well px-1.5 text-xs font-semibold text-ink-soft hover:text-ink"
                        >
                          {statuses.map((o) => (
                            <option key={o.id} value={o.id}>{o.name}</option>
                          ))}
                        </select>
                      )}
                    </div>
                  </Card>
                ))}
              </Column>
            );
          })}
        </div>
        <DragOverlay dropAnimation={null}>
          {draggedCard && (
            <div className="flex w-[240px] rotate-1 flex-col gap-1.5 rounded-[14px] bg-white p-3.5 shadow-[0_16px_40px_rgba(43,36,32,0.18),0_2px_6px_rgba(43,36,32,0.08)]">
              <span className="text-xs font-bold text-muted">{draggedCard.key}</span>
              <span className="text-sm font-bold leading-snug">{draggedCard.title}</span>
            </div>
          )}
        </DragOverlay>
      </DndContext>
      {closing && (
        <CloseDialog
          ticket={closing.ticket}
          status={closing.status}
          nodes={nodes}
          onDone={() => {
            setClosing(null);
            router.refresh();
          }}
          onCancel={() => setClosing(null)}
        />
      )}
    </div>
  );
}
