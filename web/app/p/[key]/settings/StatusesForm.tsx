"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import { useProblemText, type Status } from "@/lib/problem";
import { button, cx, field, panel, table } from "@/lib/ui";

type Row = Pick<Status, "name" | "category" | "color" | "is_default"> & { id?: number };

const toRow = ({ id, name, category, color, is_default }: Status): Row => ({ id, name, category, color, is_default });
const categories = ["todo", "in_progress", "done", "cancelled"] as const;

// The project's ordered statuses (R-TK-1, R-TK-2). The tickets of a removed
// status move to the chosen status in the same save (R-TK-4).
export default function StatusesForm({ projectKey, statuses }: { projectKey: string; statuses: Status[] }) {
  const t = useTranslations("statuses");
  const problemText = useProblemText();
  const router = useRouter();
  const [rows, setRows] = useState<Row[]>(() => statuses.map(toRow));
  const [moves, setMoves] = useState<Record<number, number>>({});
  const [status, setStatus] = useState("");
  const removed = statuses.filter((s) => !rows.some((r) => r.id === s.id));
  const kept = rows.filter((r): r is Row & { id: number } => r.id !== undefined);

  const update = (i: number, patch: Partial<Row>) =>
    setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : patch.is_default ? { ...r, is_default: false } : r)));
  const swap = (i: number, j: number) =>
    setRows((rs) => {
      const next = [...rs];
      [next[i], next[j]] = [next[j], next[i]];
      return next;
    });

  async function save() {
    const { data, error } = await api.PUT("/projects/{key}/statuses", {
      params: { path: { key: projectKey } },
      body: { statuses: rows, move_to: removed.filter((s) => moves[s.id]).map((s) => ({ from: s.id, to: moves[s.id] })) },
    });
    if (error) return setStatus(problemText(error));
    setRows(data.items.map(toRow));
    setMoves({});
    setStatus(t("saved"));
    router.refresh();
  }

  const iconButton = "inline-flex size-7 cursor-pointer items-center justify-center rounded text-muted hover:bg-well hover:text-ink disabled:opacity-30";
  return (
    <section aria-labelledby="statuses-title" className={cx(panel, "flex flex-col gap-3 p-4")}>
      <h2 id="statuses-title" className="text-sm font-semibold">{t("title")}</h2>
      <div className="overflow-x-auto rounded border border-line-soft">
        <table className={table.table}>
          <thead className={table.head}>
            <tr>
              <th className={table.th}>{t("color")}</th>
              <th className={table.th}>{t("name")}</th>
              <th className={table.th}>{t("category")}</th>
              <th className={table.th}>{t("default")}</th>
              <th className={table.th} />
            </tr>
          </thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={r.id ?? `new-${i}`} className={table.row}>
                <td className={cx(table.td, "w-12")}>
                  <input
                    type="color"
                    aria-label={t("color")}
                    value={r.color}
                    onChange={(e) => update(i, { color: e.target.value.toUpperCase() })}
                    className="h-7 w-9 cursor-pointer rounded border border-line bg-white p-0.5"
                  />
                </td>
                <td className={table.td}>
                  <input aria-label={t("name")} value={r.name} maxLength={50} onChange={(e) => update(i, { name: e.target.value })} className={field.compact} />
                </td>
                <td className={table.td}>
                  <select aria-label={t("category")} value={r.category} onChange={(e) => update(i, { category: e.target.value as Row["category"] })} className={field.compact}>
                    {categories.map((c) => (
                      <option key={c} value={c}>{t(c)}</option>
                    ))}
                  </select>
                </td>
                <td className={table.td}>
                  <input type="radio" name="default-status" aria-label={t("default")} checked={r.is_default} onChange={() => update(i, { is_default: true })} className="mt-2 size-4 accent-accent" />
                </td>
                <td className={cx(table.td, "whitespace-nowrap text-right")}>
                  <button type="button" aria-label={t("up")} title={t("up")} disabled={i === 0} onClick={() => swap(i, i - 1)} className={iconButton}>
                    <Icon name="chevron" className="size-4 rotate-180" />
                  </button>
                  <button type="button" aria-label={t("down")} title={t("down")} disabled={i === rows.length - 1} onClick={() => swap(i, i + 1)} className={iconButton}>
                    <Icon name="chevron" className="size-4" />
                  </button>
                  <button type="button" onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))} className={cx(button.quiet, "ml-2")}>
                    {t("remove")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <button
        type="button"
        onClick={() => setRows((rs) => [...rs, { name: "", category: "in_progress", color: "#7D746C", is_default: false }])}
        className={cx(button.secondary, "self-start")}
      >
        <Icon name="plus" />
        {t("add")}
      </button>
      {removed.map((s) => (
        <label key={s.id} className="flex flex-wrap items-center gap-2 rounded border border-warn-line bg-warn-soft px-3 py-2 text-[13px] text-warn">
          {t("moveFrom", { name: s.name })}
          <select value={moves[s.id] ?? ""} onChange={(e) => setMoves((m) => ({ ...m, [s.id]: Number(e.target.value) }))} className={field.compact}>
            <option value="">—</option>
            {kept.map((r) => (
              <option key={r.id} value={r.id}>{r.name}</option>
            ))}
          </select>
        </label>
      ))}
      <div className="flex items-center gap-3 border-t border-line-soft pt-3">
        <button type="button" onClick={save} className={button.primary}>{t("save")}</button>
        {status && <p role="status" className="text-[13px] text-muted">{status}</p>}
      </div>
    </section>
  );
}
