"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Client, type Node, type Problem } from "@/lib/problem";
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

type Props = { projectKey: string; nodes: Node[]; clients: Client[]; canEdit: boolean };

// The module tree screen (FSD §7.3): the tree on the left, the selected node on the right.
// ponytail: moves use a parent picker and up/down buttons; drag-and-drop arrives with dnd-kit and the board.
export default function ModuleTree({ projectKey, nodes, clients, canEdit }: Props) {
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

  const chip = "rounded bg-neutral-100 px-2 py-0.5 text-xs";
  function badge(n: Node) {
    if (n.type === "module") return hasSpecific.has(n.id) ? <span className={chip}>{t("hasClientSpecific")}</span> : null;
    return <span className={chip}>{n.client_specific ? n.clients.map((c) => c.name).join(", ") : t("shared")}</span>;
  }

  function level(parentId: number | null, depth: number): React.ReactNode {
    const list = (children.get(parentId) ?? []).filter((n) => shown === null || shown.has(n.id));
    if (list.length === 0) return null;
    return (
      <ul>
        {list.map((n) => {
          const hasKids = (children.get(n.id)?.length ?? 0) > 0;
          const open = shown !== null || !collapsed.has(n.id);
          return (
            <li key={n.id}>
              {/* R-MR-1: indentation stops growing after six levels. */}
              <div className="flex items-center gap-2 py-1" style={{ paddingLeft: `${Math.min(depth, 6) * 1.25}rem` }}>
                {hasKids ? (
                  <button
                    type="button"
                    aria-expanded={open}
                    aria-label={t(open ? "collapse" : "expand", { name: n.name })}
                    onClick={() => toggle(n.id)}
                    className="w-5 text-neutral-500"
                  >
                    {open ? "▾" : "▸"}
                  </button>
                ) : (
                  <span className="w-5" />
                )}
                <button
                  type="button"
                  onClick={() => select(n.id)}
                  aria-current={n.id === selectedId ? "true" : undefined}
                  className={n.id === selectedId ? "font-semibold underline" : "hover:underline"}
                >
                  {n.name}
                </button>
                <span className="text-xs text-neutral-500">{t(n.type)}</span>
                {badge(n)}
              </div>
              {open && level(n.id, depth + 1)}
            </li>
          );
        })}
      </ul>
    );
  }

  const action = "rounded border px-3 py-1 disabled:opacity-40";
  function details(n: Node) {
    const siblings = children.get(n.parent_id) ?? [];
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
        <p className="text-sm text-neutral-600">{pathOf(n)}</p>
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
            <div className="flex flex-wrap gap-2 border-t pt-4 text-sm">
              <button type="button" onClick={() => { setCreatingUnder(n.id); setError(""); }} className={action}>
                {t("addChild")}
              </button>
              <button type="button" disabled={index <= 0} onClick={() => move(n.parent_id, index - 1)} className={action}>
                {t("moveUp")}
              </button>
              <button type="button" disabled={index === siblings.length - 1} onClick={() => move(n.parent_id, index + 1)} className={action}>
                {t("moveDown")}
              </button>
              <button type="button" onClick={remove} className={`${action} border-red-300 text-red-700`}>
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
              className="flex items-end gap-2 text-sm"
            >
              <label className="flex flex-col gap-1">
                {t("moveTo")}
                <select name="parent" defaultValue={n.parent_id ?? ""} className="rounded border px-2 py-1">
                  <option value="">{t("topLevel")}</option>
                  {nodes
                    .filter((x) => !blocked.has(x.id))
                    .map((x) => (
                      <option key={x.id} value={x.id}>{pathOf(x)}</option>
                    ))}
                </select>
              </label>
              <button className={action}>{t("move")}</button>
            </form>
          </>
        )}
      </div>
    );
  }

  return (
    <div className="grid gap-6 md:grid-cols-2">
      <section aria-label={t("tree")} className="rounded-lg border bg-white p-4">
        <div className="mb-3 flex items-end gap-3">
          <label className="flex flex-1 flex-col gap-1 text-sm">
            {t("filter")}
            <input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t("filterPlaceholder")} className="rounded border px-3 py-2" />
          </label>
          {canEdit && (
            <button type="button" onClick={() => { select(null); setCreatingUnder(null); }} className="rounded bg-neutral-900 px-3 py-2 text-sm text-white">
              {t("addTop")}
            </button>
          )}
        </div>
        {nodes.length === 0 ? <p className="text-sm text-neutral-600">{canEdit ? t("emptyAdmin") : t("empty")}</p> : level(null, 0)}
      </section>
      <section aria-label={t("details")} className="rounded-lg border bg-white p-4">
        {error && <p role="alert" className="mb-3 text-sm text-red-700">{error}</p>}
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
          <p className="text-sm text-neutral-600">{t("pick")}</p>
        )}
      </section>
    </div>
  );
}
