"use client";

import { useEffect, useState } from "react";
import { DndContext, DragOverlay, PointerSensor, useDraggable, useDroppable, useSensor, useSensors } from "@dnd-kit/core";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { Avatar, ClientChip, PriorityChip, TypeIcon } from "@/components/Chips";
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
      className={cx("flex w-[270px] shrink-0 flex-col gap-1.5 self-start rounded-md bg-well p-2", isOver && "outline-2 -outline-offset-2 outline-accent")}
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
export default function Board({ projectKey, statuses, tickets, nodes, canEdit, today, query }: Props) {
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
                <h2
                  className="flex h-8 items-center gap-2 border-t-[3px] px-1 pt-1 text-xs font-semibold uppercase tracking-[0.04em]"
                  style={{ borderTopColor: s.color }}
                >
                  <span className="truncate">{s.name}</span>
                  <span className="rounded-[3px] bg-white px-1.5 font-mono text-[11px] leading-5 text-muted">{cards.length}</span>
                  {droppable && (
                    <Link
                      href={`/p/${projectKey}/tickets/new?status_id=${s.id}`}
                      aria-label={t("addHere", { status: s.name })}
                      className="ml-auto inline-flex size-6 items-center justify-center rounded text-muted hover:bg-white hover:text-ink"
                    >
                      <Icon name="plus" />
                    </Link>
                  )}
                </h2>
                {closes(s) && (
                  <p className="flex items-center gap-2 px-1 text-xs text-muted">
                    {showAll ? t("allClosed") : t("recentClosed")}
                    <Link href={`?${new URLSearchParams(showAll ? recent : { ...query, closed: "all" })}`} className="ml-auto">
                      {showAll ? t("showRecent") : t("showAll")}
                    </Link>
                  </p>
                )}
                {cards.map((c) => (
                  <Card key={c.id} id={c.id} canEdit={canEdit} className="flex flex-col gap-1.5 rounded border border-line bg-white px-2.5 py-2">
                    <div className="flex items-center gap-1.5 text-xs">
                      <TypeIcon type={c.type} label={tTypes(c.type)} />
                      <span className="font-mono font-semibold">{c.key}</span>
                      {(c.missing_reason || c.node_names.length === 0) && (
                        <span role="img" title={t("missing")} aria-label={t("missing")} className="size-[7px] rounded-full bg-[#D97706]" />
                      )}
                      <span className="ml-auto">
                        <PriorityChip priority={c.priority} label={tPri(c.priority)} />
                      </span>
                    </div>
                    <Link href={`/t/${c.key}`} className="text-[13px] font-medium leading-snug text-ink no-underline hover:text-ink hover:underline">
                      {c.title}
                    </Link>
                    {c.node_names.length > 0 && (
                      <span className="truncate font-mono text-[11px] text-muted">
                        {c.node_names[0]}
                        {c.node_names.length > 1 ? ` +${c.node_names.length - 1}` : ""}
                      </span>
                    )}
                    <div className="flex items-center gap-1.5 text-xs text-muted">
                      <ClientChip client={c.client} coreLabel={t("noClient")} />
                      {c.due_date && (
                        <span className={c.due_date < today ? "font-medium text-danger" : ""}>{day(c.due_date, locale, c.due_date.slice(0, 4) !== today.slice(0, 4))}</span>
                      )}
                      {c.assignee && <Avatar name={c.assignee.name} className="ml-auto size-5 bg-well text-[9px] text-ink" />}
                    </div>
                    {canEdit && (
                      <select
                        aria-label={t("moveTo", { key: c.key })}
                        value={c.status_id}
                        onChange={(e) => move(c.id, Number(e.target.value))}
                        className="mt-0.5 h-7 w-full rounded border border-line-soft bg-paper px-1 text-xs text-muted"
                      >
                        {statuses.map((o) => (
                          <option key={o.id} value={o.id}>{o.name}</option>
                        ))}
                      </select>
                    )}
                  </Card>
                ))}
              </Column>
            );
          })}
        </div>
        <DragOverlay dropAnimation={null}>
          {draggedCard && (
            <div className="flex w-[254px] flex-col gap-1 rounded border border-accent bg-white px-2.5 py-2 shadow-lg">
              <span className="font-mono text-xs font-semibold">{draggedCard.key}</span>
              <span className="text-[13px] font-medium leading-snug">{draggedCard.title}</span>
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
