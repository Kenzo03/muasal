"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import Icon from "./Icon";
import { nodePaths } from "@/lib/nodes";
import type { Node } from "@/lib/problem";
import { cx } from "@/lib/ui";

type Props = {
  nodes: Node[];
  selected: Set<number>;
  onToggle: (id: number, on: boolean) => void;
  legend: string; // read by screen readers; the visible label sits beside the picker
  recent?: number[]; // the user's recently used nodes, most recent first
};

// The menu picker of the ticket form and the close dialog (FSD §8.3): a filter
// by path, alias or code, then a checkbox per menu or module, named by its path.
// Recently used menus come first, marked "Recent" (§8.1).
export default function NodePicker({ nodes, selected, onToggle, legend, recent = [] }: Props) {
  const t = useTranslations("ticketForm");
  const [filter, setFilter] = useState("");
  const pathOf = useMemo(() => nodePaths(nodes), [nodes]);
  const q = filter.trim().toLowerCase();
  const rank = (id: number) => (recent.includes(id) ? recent.indexOf(id) : recent.length);
  const choices = nodes
    .filter((n) => !q || [pathOf(n.id), n.code ?? "", ...n.aliases].some((s) => s.toLowerCase().includes(q)))
    .map((n, i) => ({ n, i }))
    .sort((a, b) => rank(a.n.id) - rank(b.n.id) || a.i - b.i) // stable: the tree order stays within each group
    .map(({ n }) => n);
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="sr-only">{legend}</legend>
      <label className="flex h-[34px] items-center gap-2 rounded border border-field bg-white px-2.5 text-muted focus-within:outline-2 focus-within:outline-accent">
        <Icon name="search" />
        <input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          aria-label={t("menusFilter")}
          placeholder={t("menusFilter")}
          className="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-muted"
        />
      </label>
      <div className="flex max-h-48 flex-col overflow-y-auto rounded border border-line-soft text-[13px]">
        {choices.map((n) => (
          <label key={n.id} className={cx("flex items-center gap-2 border-b border-line-soft px-2.5 py-1.5 last:border-0", selected.has(n.id) && "bg-accent-soft")}>
            <input type="checkbox" checked={selected.has(n.id)} onChange={(e) => onToggle(n.id, e.target.checked)} className="size-4 accent-accent" />
            {pathOf(n.id)}
            {recent.includes(n.id) && <span className="rounded-[3px] bg-paper px-1.5 text-[11px] font-semibold text-muted">{t("recent")}</span>}
            {n.code && <span className="ml-auto font-mono text-[11px] text-muted">{n.code}</span>}
          </label>
        ))}
      </div>
    </fieldset>
  );
}
