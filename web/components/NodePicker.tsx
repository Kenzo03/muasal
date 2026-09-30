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
// Recently used menus come first, marked "Recent" (§8.1). Menus ticked before
// the filter last changed come before them and ignore the filter, so a search
// never hides a selection (MSL-33); a tick doesn't move the row under the pointer.
export default function NodePicker({ nodes, selected, onToggle, legend, recent = [] }: Props) {
  const t = useTranslations("ticketForm");
  const [filter, setFilter] = useState("");
  const [pinned, setPinned] = useState(() => new Set(selected));
  const pathOf = useMemo(() => nodePaths(nodes), [nodes]);
  const q = filter.trim().toLowerCase();
  const rank = (id: number) => (recent.includes(id) ? recent.indexOf(id) : recent.length);
  const choices = nodes
    .filter((n) => pinned.has(n.id) || !q || [pathOf(n.id), n.code ?? "", ...n.aliases].some((s) => s.toLowerCase().includes(q)))
    .map((n, i) => ({ n, i }))
    // stable: the tree order stays within each group
    .sort((a, b) => Number(pinned.has(b.n.id)) - Number(pinned.has(a.n.id)) || rank(a.n.id) - rank(b.n.id) || a.i - b.i)
    .map(({ n }) => n);
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="sr-only">{legend}</legend>
      <label className="flex h-10 items-center gap-2 rounded-[10px] border border-field bg-white px-3 text-muted focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-accent">
        <Icon name="search" />
        <input
          value={filter}
          onChange={(e) => {
            setFilter(e.target.value);
            setPinned(new Set(selected));
          }}
          aria-label={t("menusFilter")}
          placeholder={t("menusFilter")}
          className="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-muted"
        />
      </label>
      <div className="flex max-h-52 flex-col gap-0.5 overflow-y-auto rounded-xl border border-line bg-white p-1 text-[13px]">
        {choices.map((n) => (
          <label
            key={n.id}
            className={cx(
              "flex cursor-pointer items-center gap-2.5 rounded-lg px-2.5 py-2",
              selected.has(n.id) ? "bg-accent-soft font-semibold text-accent-strong" : "hover:bg-paper",
            )}
          >
            <input type="checkbox" checked={selected.has(n.id)} onChange={(e) => onToggle(n.id, e.target.checked)} className="size-4 shrink-0 accent-accent" />
            {pathOf(n.id)}
            {recent.includes(n.id) && <span className="rounded-full bg-well px-2 text-[11px] font-semibold leading-[18px] text-muted">{t("recent")}</span>}
            {n.code && <span className="ml-auto text-[11.5px] font-medium text-muted">{n.code}</span>}
          </label>
        ))}
      </div>
    </fieldset>
  );
}
