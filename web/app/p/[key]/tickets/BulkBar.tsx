"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { changesFields, updateBody, type BulkChange } from "@/lib/bulk";
import { useProblemText, type Ref, type Status } from "@/lib/problem";
import { button, cx, field, panel } from "@/lib/ui";

type Priority = components["schemas"]["Priority"];

const rows = () => document.querySelectorAll<HTMLInputElement>('input[name="key"][form="bulk"]');

// "Select all": ticks every row's box, which the rows tie to the bulk form.
export function SelectAll({ label }: { label: string }) {
  return (
    <input
      type="checkbox"
      aria-label={label}
      className="size-4 accent-accent"
      onChange={(e) => {
        rows().forEach((c) => (c.checked = e.target.checked));
        document.dispatchEvent(new Event("change"));
      }}
    />
  );
}

// Bulk changes on the ticket list (MSL-53): assignee, priority, due date or an
// open status for every ticked ticket. Each goes through the same API as the
// ticket page, so permissions, history and notifications stay as they are.
export default function BulkBar({ people, statuses }: { people: Ref[]; statuses: Status[] }) {
  const t = useTranslations("tickets.bulk");
  const router = useRouter();
  const problemText = useProblemText();
  const [count, setCount] = useState(0);
  const [busy, setBusy] = useState("");
  const [failed, setFailed] = useState<string[]>([]);
  useEffect(() => {
    const onChange = () => setCount([...rows()].filter((c) => c.checked).length);
    document.addEventListener("change", onChange);
    return () => document.removeEventListener("change", onChange);
  }, []);

  async function apply(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const keys = form.getAll("key").map(String);
    const c: BulkChange = {};
    const assignee = String(form.get("assignee") ?? "keep");
    if (assignee !== "keep") c.assignee = assignee === "none" ? "" : assignee;
    if (form.get("priority")) c.priority = String(form.get("priority")) as Priority;
    if (form.get("clear_due") === "on") c.due = "";
    else if (form.get("due")) c.due = String(form.get("due"));
    if (form.get("status")) c.status = Number(form.get("status"));
    if (keys.length === 0 || Object.keys(c).length === 0) return;
    const bad: string[] = [];
    for (const [i, key] of keys.entries()) {
      setBusy(t("progress", { done: i + 1, total: keys.length }));
      if (changesFields(c)) {
        const { data, error } = await api.GET("/tickets/{key}", { params: { path: { key } } });
        const put = data
          ? await api.PUT("/tickets/{key}", { params: { path: { key }, header: { "If-Match": `"${data.version}"` } }, body: updateBody(data, c) })
          : undefined;
        const problem = error ?? put?.error;
        if (problem) {
          bad.push(`${key}: ${problemText(problem)}`);
          continue;
        }
      }
      if (c.status) {
        const { error } = await api.POST("/tickets/{key}/transition", { params: { path: { key } }, body: { status_id: c.status } });
        if (error) bad.push(`${key}: ${problemText(error)}`);
      }
    }
    setBusy("");
    setFailed(bad);
    rows().forEach((r) => (r.checked = false));
    setCount(0);
    router.refresh();
  }

  const open = statuses.filter((s) => s.category === "todo" || s.category === "in_progress");
  return (
    // Explicit label ids: some assistive tech misses wrapping labels (MSL-62).
    <form id="bulk" aria-label={t("label")} onSubmit={apply} className={cx(panel, "flex flex-wrap items-end gap-3 p-3")}>
      <p className="basis-full text-[13px] font-semibold text-ink-soft">{count > 0 ? t("selected", { count }) : t("hint")}</p>
      <fieldset disabled={count === 0 || busy !== ""} className="contents">
        <label htmlFor="bulk-assignee" className={field.label}>
          {t("assignee")}
          <select id="bulk-assignee" name="assignee" defaultValue="keep" className={field.compact}>
            <option value="keep">{t("keep")}</option>
            <option value="none">{t("nobody")}</option>
            {people.map((p) => (
              <option key={p.id} value={p.id}>{p.name}</option>
            ))}
          </select>
        </label>
        <label htmlFor="bulk-priority" className={field.label}>
          {t("priority")}
          <select id="bulk-priority" name="priority" defaultValue="" className={field.compact}>
            <option value="">{t("keep")}</option>
            {(["low", "medium", "high", "urgent"] as const).map((p) => (
              <option key={p} value={p}>{t(`priorities.${p}`)}</option>
            ))}
          </select>
        </label>
        <label htmlFor="bulk-due" className={field.label}>
          {t("due")}
          <input id="bulk-due" type="date" name="due" className={field.compact} />
        </label>
        <label htmlFor="bulk-clear-due" className="flex items-center gap-1.5 pb-2 text-[13px]">
          <input id="bulk-clear-due" type="checkbox" name="clear_due" className="size-4 accent-accent" />
          {t("clearDue")}
        </label>
        <label htmlFor="bulk-status" className={field.label}>
          {t("status")}
          <select id="bulk-status" name="status" defaultValue="" className={field.compact}>
            <option value="">{t("keep")}</option>
            {open.map((s) => (
              <option key={s.id} value={s.id}>{s.name}</option>
            ))}
          </select>
        </label>
        <button className={button.secondary}>{busy || t("apply", { count })}</button>
      </fieldset>
      {failed.length > 0 && (
        <div role="alert" className="basis-full text-xs text-danger">
          {t("failed")} {failed.join("; ")}
        </div>
      )}
    </form>
  );
}
