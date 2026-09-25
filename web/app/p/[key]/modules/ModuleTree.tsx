"use client";

import { useMemo, useState } from "react";
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  pointerWithin,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type Active,
  type Announcements,
  type CollisionDetection,
  type DragEndEvent,
  type DragMoveEvent,
} from "@dnd-kit/core";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { ClientChip } from "@/components/Chips";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import { useProblemText, type Client, type Node, type Problem } from "@/lib/problem";
import { button, chip, cx, field, panel } from "@/lib/ui";
import NodeForm, { ReadOnlyNode } from "./NodeForm";

type Children = Map<number | null, Node[]>;

// ancestorsOf lists n's parents, nearest first.
function ancestorsOf(n: Node, byId: Map<number, Node>): Node[] {
  const out: Node[] = [];
  for (let p = n.parent_id === null ? undefined : byId.get(n.parent_id); p; p = p.parent_id === null ? undefined : byId.get(p.parent_id)) {
    out.push(p);
  }
  return out;
}

// subtree holds id and every node below it: the places a node cannot move to.
function subtree(id: number, children: Children): Set<number> {
  const out = new Set([id]);
  const stack = [id];
  while (stack.length > 0) {
    for (const c of children.get(stack.pop()!) ?? []) {
      out.add(c.id);
      stack.push(c.id);
    }
  }
  return out;
}

type Props = { projectKey: string; nodes: Node[]; clients: Client[]; canEdit: boolean; showArchived: boolean };

// Where a dragged node would land: before or after a row, or inside it as its last child.
type Drop = { id: number; where: "before" | "inside" | "after" };

// pointOf is where a drag points: the pointer for a mouse or touch, and the
// handle's centre for the keyboard. dnd-kit's own collision rect is the
// overlay's, which is larger than the handle and offset by the activation move.
function pointOf(e: { active: Active; activatorEvent: Event; delta: { x: number; y: number } }) {
  if ("clientX" in e.activatorEvent) {
    const p = e.activatorEvent as PointerEvent;
    return { x: p.clientX + e.delta.x, y: p.clientY + e.delta.y };
  }
  const start = e.active.rect.current.initial;
  return start && { x: start.left + start.width / 2 + e.delta.x, y: start.top + start.height / 2 + e.delta.y };
}

// dropOf reads the drop from where the drag points over the target row: its
// top third means before, its bottom third after, the middle inside.
function dropOf(e: DragMoveEvent | DragEndEvent): Drop | null {
  const over = e.over;
  const at = pointOf(e);
  if (!over || !at || over.id === e.active.id) return null;
  const y = (at.y - over.rect.top) / over.rect.height;
  return { id: Number(over.id), where: y < 1 / 3 ? "before" : y > 2 / 3 ? "after" : "inside" };
}

// underPoint finds the row under that same point.
const underPoint: CollisionDetection = (args) => {
  if (args.pointerCoordinates) return pointerWithin(args);
  const start = args.active.rect.current.initial;
  if (!start) return [];
  const at = { x: args.collisionRect.left + start.width / 2, y: args.collisionRect.top + start.height / 2 };
  return pointerWithin({ ...args, pointerCoordinates: at });
};

// A tree row: the drag handle for project admins, and a drop target for every
// live node (FSD §7.3).
function TreeRow({ n, canEdit, drop, handleLabel, className, style, children }: {
  n: Node;
  canEdit: boolean;
  drop: Drop | null;
  handleLabel: string;
  className: string;
  style: React.CSSProperties;
  children: React.ReactNode;
}) {
  const drag = useDraggable({ id: n.id, disabled: !canEdit || n.archived });
  const target = useDroppable({ id: n.id, disabled: n.archived });
  const here = drop?.id === n.id ? drop.where : null;
  return (
    <div
      ref={target.setNodeRef}
      className={cx(
        "relative",
        className,
        drag.isDragging && "opacity-40",
        here === "inside" && "outline-2 -outline-offset-2 outline-accent",
      )}
      style={style}
    >
      {here === "before" && <span aria-hidden className="absolute inset-x-0 top-0 h-0.5 bg-accent" />}
      {canEdit && !n.archived && (
        <button
          ref={drag.setNodeRef}
          type="button"
          aria-label={handleLabel}
          {...drag.attributes}
          {...drag.listeners}
          className="inline-flex size-5 shrink-0 cursor-grab touch-none items-center justify-center rounded text-muted hover:bg-well active:cursor-grabbing"
        >
          <Icon name="grip" className="size-3.5" />
        </button>
      )}
      {children}
      {here === "after" && <span aria-hidden className="absolute inset-x-0 bottom-0 h-0.5 bg-accent" />}
    </div>
  );
}

// The module tree screen (FSD §7.3): the tree on the left, the selected node on
// the right. Project admins move nodes by dragging a row's handle, with the
// pointer or the keyboard (Space, arrows, Space), or with the parent picker
// and the up and down buttons.
export default function ModuleTree({ projectKey, nodes, clients, canEdit, showArchived }: Props) {
  const t = useTranslations("modules");
  const problemText = useProblemText();
  const router = useRouter();
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [creatingUnder, setCreatingUnder] = useState<number | null | undefined>(undefined); // undefined: not creating
  const [filter, setFilter] = useState("");
  const [collapsed, setCollapsed] = useState<Set<number>>(() => new Set());
  const [error, setError] = useState("");
  const [drop, setDrop] = useState<Drop | null>(null);
  const [dragging, setDragging] = useState<number | null>(null);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }), useSensor(KeyboardSensor));

  const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);
  const children = useMemo(() => {
    const m: Children = new Map();
    for (const n of nodes) m.set(n.parent_id, [...(m.get(n.parent_id) ?? []), n]);
    return m;
  }, [nodes]);
  // R-MR-9: a module above a visible client-specific menu gets a badge.
  const hasSpecific = useMemo(() => {
    const out = new Set<number>();
    for (const n of nodes) if (n.client_specific) for (const p of ancestorsOf(n, byId)) out.add(p.id);
    return out;
  }, [nodes, byId]);
  // The filter keeps nodes whose name, alias or code matches, plus their ancestors.
  const shown = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return null;
    const keep = new Set<number>();
    for (const n of nodes) {
      if (![n.name, n.code ?? "", ...n.aliases].some((s) => s.toLowerCase().includes(q))) continue;
      keep.add(n.id);
      for (const p of ancestorsOf(n, byId)) keep.add(p.id);
    }
    return keep;
  }, [filter, nodes, byId]);

  const pathOf = (n: Node) => [...ancestorsOf(n, byId).reverse(), n].map((x) => x.name).join(" › ");
  const selected = selectedId === null ? undefined : byId.get(selectedId);

  function select(id: number | null) {
    setSelectedId(id);
    setCreatingUnder(undefined);
    setError("");
  }

  function toggle(id: number) {
    setCollapsed((s) => {
      const next = new Set(s);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function run(call: Promise<{ error?: Problem }>, then?: () => void) {
    const { error } = await call;
    if (error) return setError(problemText(error));
    setError("");
    then?.();
    router.refresh();
  }

  // A drop becomes one move: inside a node puts it last among its children;
  // before or after a node puts it at that place among the node's siblings,
  // counted without the moved node, as the server counts (§7.1).
  function onDragEnd(e: DragEndEvent) {
    const d = dropOf(e);
    setDrop(null);
    const moved = byId.get(Number(e.active.id));
    const target = d && byId.get(d.id);
    if (!d || !moved || !target || subtree(moved.id, children).has(target.id)) return;
    if (d.where === "inside") {
      if (target.id === moved.parent_id) return;
      return run(api.PATCH("/nodes/{id}", { params: { path: { id: moved.id } }, body: { move: { parent_id: target.id } } }));
    }
    const siblings = (children.get(target.parent_id) ?? []).filter((x) => !x.archived && x.id !== moved.id);
    const position = siblings.findIndex((x) => x.id === target.id) + (d.where === "after" ? 1 : 0);
    run(api.PATCH("/nodes/{id}", { params: { path: { id: moved.id } }, body: { move: { parent_id: target.parent_id, position } } }));
  }

  const announcements: Announcements = {
    onDragStart: ({ active }) => t("dnd.start", { name: byId.get(Number(active.id))?.name ?? "" }),
    onDragOver: ({ active, over }) =>
      over
        ? t("dnd.over", { name: byId.get(Number(active.id))?.name ?? "", target: byId.get(Number(over.id))?.name ?? "" })
        : t("dnd.none", { name: byId.get(Number(active.id))?.name ?? "" }),
    onDragEnd: ({ active, over }) =>
      over ? t("dnd.end", { name: byId.get(Number(active.id))?.name ?? "", target: byId.get(Number(over.id))?.name ?? "" }) : t("dnd.cancel"),
    onDragCancel: () => t("dnd.cancel"),
  };

  function badge(n: Node) {
    if (n.type === "module") {
      return hasSpecific.has(n.id) ? <span className={cx(chip, "bg-accent-soft text-accent-strong")}>{t("hasClientSpecific")}</span> : null;
    }
    if (!n.client_specific) return <span className={cx(chip, "bg-ground text-[#4A423C]")}>{t("shared")}</span>;
    return n.clients.map((c) => <ClientChip key={c.id} client={c} coreLabel="" />);
  }

  function level(parentId: number | null, depth: number): React.ReactNode {
    const list = (children.get(parentId) ?? []).filter((n) => shown === null || shown.has(n.id));
    if (list.length === 0) return null;
    return (
      <ul>
        {list.map((n) => {
          const hasKids = (children.get(n.id)?.length ?? 0) > 0;
          const open = shown !== null || !collapsed.has(n.id);
          const current = n.id === selectedId;
          return (
            <li key={n.id}>
              {/* R-MR-1: indentation stops growing after six levels. */}
              <TreeRow
                n={n}
                canEdit={canEdit}
                drop={drop}
                handleLabel={t("dragHandle", { name: n.name })}
                className={cx("flex min-h-8 items-center gap-1.5 pr-3", current && "bg-accent-soft")}
                style={{ paddingLeft: `${0.75 + Math.min(depth, 6) * 1.25}rem` }}
              >
                {hasKids ? (
                  <button
                    type="button"
                    aria-expanded={open}
                    aria-label={t(open ? "collapse" : "expand", { name: n.name })}
                    onClick={() => toggle(n.id)}
                    className="inline-flex size-5 shrink-0 cursor-pointer items-center justify-center rounded text-muted hover:bg-well"
                  >
                    <Icon name={open ? "chevron" : "chevronRight"} className="size-3.5" />
                  </button>
                ) : (
                  <span className="size-5 shrink-0" />
                )}
                <Icon name={n.type === "module" ? "folder" : "screen"} className={cx("size-4", current ? "text-accent-strong" : "text-muted")} />
                <span className="sr-only">{t(n.type)}</span>
                <button
                  type="button"
                  onClick={() => select(n.id)}
                  aria-current={current ? "true" : undefined}
                  className={cx(
                    "cursor-pointer truncate text-left hover:underline",
                    n.type === "module" && "font-semibold",
                    current && "font-semibold text-accent-strong",
                    n.archived && "text-muted",
                  )}
                >
                  {n.name}
                </button>
                {badge(n)}
                {n.archived && <span className={cx(chip, "bg-well text-muted")}>{t("archivedBadge")}</span>}
              </TreeRow>
              {open && level(n.id, depth + 1)}
            </li>
          );
        })}
      </ul>
    );
  }

  function details(n: Node) {
    const path = (
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-xs text-muted">{pathOf(n)}</p>
        <Link href={`/p/${projectKey}/modules/${n.id}`} className={cx(button.quiet, "ml-auto")}>
          {t("openPage")}
          <Icon name="arrowRight" className="size-3.5" />
        </Link>
      </div>
    );
    // R-MR-4: an archived node can only be restored.
    if (n.archived) {
      return (
        <div className="flex flex-col gap-4">
          {path}
          <ReadOnlyNode node={n} />
          {canEdit && (
            <button
              type="button"
              onClick={() => run(api.PATCH("/nodes/{id}", { params: { path: { id: n.id } }, body: { archived: false } }))}
              className={cx(button.secondary, "self-start")}
            >
              {t("restore")}
            </button>
          )}
        </div>
      );
    }
    // Positions count live siblings only, as the server does.
    const siblings = (children.get(n.parent_id) ?? []).filter((s) => !s.archived);
    const index = siblings.findIndex((s) => s.id === n.id);
    const blocked = subtree(n.id, children);
    const move = (parentId: number | null, position?: number) =>
      run(api.PATCH("/nodes/{id}", { params: { path: { id: n.id } }, body: { move: { parent_id: parentId, position } } }));
    const remove = () => {
      if (window.confirm(t("confirmDelete", { name: n.name }))) {
        run(api.DELETE("/nodes/{id}", { params: { path: { id: n.id } } }), () => setSelectedId(null));
      }
    };
    return (
      <div className="flex flex-col gap-4">
        {path}
        {!canEdit ? (
          <ReadOnlyNode node={n} />
        ) : (
          <>
            <NodeForm
              key={n.id}
              projectKey={projectKey}
              node={n}
              clients={clients}
              onSaved={() => {
                setError("");
                router.refresh();
              }}
            />
            <div className="flex flex-wrap gap-2 border-t border-line-soft pt-4">
              <button type="button" onClick={() => { setCreatingUnder(n.id); setError(""); }} className={button.secondary}>
                <Icon name="plus" />
                {t("addChild")}
              </button>
              <button type="button" disabled={index <= 0} onClick={() => move(n.parent_id, index - 1)} className={button.secondary}>
                {t("moveUp")}
              </button>
              <button type="button" disabled={index === siblings.length - 1} onClick={() => move(n.parent_id, index + 1)} className={button.secondary}>
                {t("moveDown")}
              </button>
              <button
                type="button"
                onClick={() => run(api.PATCH("/nodes/{id}", { params: { path: { id: n.id } }, body: { archived: true } }), () => setSelectedId(null))}
                className={button.secondary}
              >
                {t("archive")}
              </button>
              <button type="button" onClick={remove} className={cx(button.danger, "ml-auto")}>
                {t("delete")}
              </button>
            </div>
            <form
              key={`move-${n.id}`}
              aria-label={t("moveTo")}
              onSubmit={(e) => {
                e.preventDefault();
                const v = String(new FormData(e.currentTarget).get("parent"));
                move(v === "" ? null : Number(v));
              }}
              className="flex flex-wrap items-end gap-2"
            >
              <label className={cx(field.label, "min-w-0 flex-1")}>
                {t("moveTo")}
                <select name="parent" defaultValue={n.parent_id ?? ""} className={field.input}>
                  <option value="">{t("topLevel")}</option>
                  {nodes
                    .filter((x) => !blocked.has(x.id) && !x.archived)
                    .map((x) => (
                      <option key={x.id} value={x.id}>{pathOf(x)}</option>
                    ))}
                </select>
              </label>
              <button className={cx(button.secondary, "h-[34px]")}>{t("move")}</button>
            </form>
          </>
        )}
      </div>
    );
  }

  return (
    <div className="grid items-start gap-4 md:grid-cols-[minmax(0,560px)_minmax(0,1fr)]">
      <section aria-label={t("tree")} className={panel}>
        <div className="flex flex-wrap items-center gap-2 border-b border-line-soft p-3">
          <label className="flex h-8 min-w-40 flex-1 items-center gap-1.5 rounded border border-line bg-white px-2 text-muted focus-within:outline-2 focus-within:outline-accent">
            <Icon name="search" />
            <input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              aria-label={t("filter")}
              placeholder={t("filterPlaceholder")}
              className="min-w-0 flex-1 bg-transparent text-[13px] text-ink outline-none placeholder:text-muted"
            />
          </label>
          {canEdit && (
            <Link href={showArchived ? "?" : "?archived=1"} className={button.quiet}>
              {showArchived ? t("hideArchived") : t("showArchived")}
            </Link>
          )}
          {canEdit && (
            <button type="button" onClick={() => { select(null); setCreatingUnder(null); }} className={button.primary}>
              <Icon name="plus" />
              {t("addTop")}
            </button>
          )}
        </div>
        <div className="py-1.5 text-[13px]">
          {nodes.length === 0 ? (
            <p className="px-3 py-2 text-muted">{canEdit ? t("emptyAdmin") : t("empty")}</p>
          ) : (
            <DndContext
              sensors={sensors}
              collisionDetection={underPoint}
              accessibility={{ announcements, screenReaderInstructions: { draggable: t("dnd.instructions") } }}
              onDragStart={(e) => setDragging(Number(e.active.id))}
              onDragMove={(e) => setDrop(dropOf(e))}
              onDragEnd={(e) => {
                setDragging(null);
                onDragEnd(e);
              }}
              onDragCancel={() => {
                setDragging(null);
                setDrop(null);
              }}
            >
              {level(null, 0)}
              <DragOverlay dropAnimation={null}>
                {dragging !== null && (
                  <span className="inline-flex items-center gap-1.5 rounded border border-accent bg-white px-2 py-1 text-[13px] font-semibold shadow">
                    <Icon name={byId.get(dragging)?.type === "module" ? "folder" : "screen"} className="size-4 text-muted" />
                    {byId.get(dragging)?.name}
                  </span>
                )}
              </DragOverlay>
            </DndContext>
          )}
        </div>
      </section>
      <section aria-label={t("details")} className={cx(panel, "p-4")}>
        {error && <p role="alert" className={cx(field.error, "mb-3")}>{error}</p>}
        {creatingUnder !== undefined ? (
          <NodeForm
            key={`new-${creatingUnder}`}
            projectKey={projectKey}
            parentId={creatingUnder ?? undefined}
            parentPath={creatingUnder === null ? undefined : pathOf(byId.get(creatingUnder)!)}
            clients={clients}
            onSaved={(id) => {
              setCreatingUnder(undefined);
              setSelectedId(id);
              router.refresh();
            }}
            onCancel={() => setCreatingUnder(undefined)}
          />
        ) : selected ? (
          details(selected)
        ) : (
          <p className="text-[13px] text-muted">{t("pick")}</p>
        )}
      </section>
    </div>
  );
}
