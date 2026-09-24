"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Status } from "@/lib/problem";

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

  const input = "rounded border px-2 py-1";
  return (
    <section aria-labelledby="statuses-title" className="flex flex-col gap-4 rounded-lg border bg-white p-4">
      <h2 id="statuses-title" className="font-medium">{t("title")}</h2>
      <table className="w-full border-collapse text-left text-sm">
        <thead>
          <tr className="border-b">
            <th className="p-2">{t("name")}</th>
            <th className="p-2">{t("category")}</th>
            <th className="p-2">{t("color")}</th>
            <th className="p-2">{t("default")}</th>
            <th className="p-2" />
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={r.id ?? `new-${i}`} className="border-b">
              <td className="p-2">
                <input aria-label={t("name")} value={r.name} maxLength={50} onChange={(e) => update(i, { name: e.target.value })} className={input} />
              </td>
              <td className="p-2">
                <select aria-label={t("category")} value={r.category} onChange={(e) => update(i, { category: e.target.value as Row["category"] })} className={input}>
                  {categories.map((c) => (
                    <option key={c} value={c}>{t(c)}</option>
                  ))}
                </select>
              </td>
              <td className="p-2">
                <input type="color" aria-label={t("color")} value={r.color} onChange={(e) => update(i, { color: e.target.value.toUpperCase() })} />
              </td>
              <td className="p-2">
                <input type="radio" name="default-status" aria-label={t("default")} checked={r.is_default} onChange={() => update(i, { is_default: true })} />
              </td>
              <td className="flex gap-2 p-2">
                <button type="button" disabled={i === 0} onClick={() => swap(i, i - 1)} className="underline disabled:opacity-40">{t("up")}</button>
                <button type="button" disabled={i === rows.length - 1} onClick={() => swap(i, i + 1)} className="underline disabled:opacity-40">{t("down")}</button>
                <button type="button" onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))} className="underline">{t("remove")}</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <button
        type="button"
        onClick={() => setRows((rs) => [...rs, { name: "", category: "in_progress", color: "#7D746C", is_default: false }])}
        className="self-start rounded border px-3 py-1 text-sm"
      >
        {t("add")}
      </button>
      {removed.map((s) => (
        <label key={s.id} className="flex items-center gap-2 text-sm">
          {t("moveFrom", { name: s.name })}
          <select value={moves[s.id] ?? ""} onChange={(e) => setMoves((m) => ({ ...m, [s.id]: Number(e.target.value) }))} className={input}>
            <option value="">—</option>
            {kept.map((r) => (
              <option key={r.id} value={r.id}>{r.name}</option>
            ))}
          </select>
        </label>
      ))}
      <div className="flex items-center gap-3">
        <button type="button" onClick={save} className="rounded bg-neutral-900 px-4 py-2 text-white">{t("save")}</button>
        {status && <p role="status" className="text-sm">{status}</p>}
      </div>
    </section>
  );
}
