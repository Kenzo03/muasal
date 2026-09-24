"use client";

import { useMemo, useState } from "react";
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

// The module tree screen (FSD §7.3): the tree on the left, the selected node on the right.
// ponytail: moves use a parent picker and up/down buttons; add tree drag-and-drop if admins ask for it.
export default function ModuleTree({ projectKey, nodes, clients, canEdit, showArchived }: Props) {
  const t = useTranslations("modules");
  const problemText = useProblemText();
  const router = useRouter();
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [creatingUnder, setCreatingUnder] = useState<number | null | undefined>(undefined); // undefined: not creating
  const [filter, setFilter] = useState("");
  const [collapsed, setCollapsed] = useState<Set<number>>(() => new Set());
  const [error, setError] = useState("");

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
              <div
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
              </div>
              {open && level(n.id, depth + 1)}
            </li>
          );
        })}
      </ul>
    );
  }

  function details(n: Node) {
    const path = <p className="text-xs text-muted">{pathOf(n)}</p>;
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
          {nodes.length === 0 ? <p className="px-3 py-2 text-muted">{canEdit ? t("emptyAdmin") : t("empty")}</p> : level(null, 0)}
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
