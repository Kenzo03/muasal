"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useLocale, useTranslations } from "next-intl";
import CloseDialog from "@/components/CloseDialog";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { problemKey, type Node, type Status, type Ticket } from "@/lib/problem";
import { button, cx, field, panel } from "@/lib/ui";

type Draft = components["schemas"]["DecisionDraft"];
type Row = { key: string; ticket?: Ticket; draft?: Draft; state: "loading" | "missing" | "closed" | "waiting" | "drafting" | "drafted" | "off" | "failed" };

// The close-out queue (MSL-65): the picked tickets' decision records are
// drafted with AI one after another in the background, then each opens in the
// close dialog, prefilled, to confirm or edit; closing one opens the next.
export default function CloseOut({ keys, statuses, nodes }: { keys: string[]; statuses: Status[]; nodes: Node[] }) {
  const t = useTranslations("closeout");
  const locale = useLocale();
  const closing = statuses.filter((s) => s.category === "done" || s.category === "cancelled");
  const [statusId, setStatusId] = useState(closing[0]?.id);
  const [rows, setRows] = useState<Row[]>(() => keys.map((key) => ({ key, state: "loading" })));
  const [open, setOpen] = useState<{ ticket: Ticket; draft?: Draft }>();
  const status = closing.find((s) => s.id === statusId);
  const set = (key: string, change: Partial<Row>) => setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...change } : r)));

  useEffect(() => {
    let stop = false;
    (async () => {
      const loaded = await Promise.all(keys.map((key) => api.GET("/tickets/{key}", { params: { path: { key } } })));
      const open = keys.filter((key, i) => {
        const tk = loaded[i].data;
        set(key, { ticket: tk, state: !tk ? "missing" : tk.status.category === "done" || tk.status.category === "cancelled" ? "closed" : "waiting" });
        return tk && tk.status.category !== "done" && tk.status.category !== "cancelled";
      });
      // One draft at a time: a local model answers one question at a time too.
      for (const [i, key] of open.entries()) {
        if (stop) return;
        set(key, { state: "drafting" });
        const { data, error } = await api.POST("/tickets/{key}/decision-draft", { params: { path: { key } }, body: { language: locale === "en" ? "en" : "id" } });
        if (error && problemKey(error) === "ai_off") {
          for (const k of open.slice(i)) set(k, { state: "off" });
          return;
        }
        set(key, data ? { draft: data, state: "drafted" } : { state: "failed" });
      }
    })();
    return () => {
      stop = true;
    };
  }, [keys, locale]);

  // The dialog guards the close with the ticket's version, so read it fresh.
  async function review(key: string) {
    const { data } = await api.GET("/tickets/{key}", { params: { path: { key } } });
    if (data) setOpen({ ticket: data, draft: rows.find((r) => r.key === key)?.draft });
  }
  const pending = rows.filter((r) => r.ticket && r.state !== "closed");

  if (keys.length === 0) return <p className="text-muted">{t("empty")}</p>;
  return (
    <div className="flex max-w-4xl flex-col gap-3">
      <div className="flex flex-wrap items-end gap-3">
        <label htmlFor="closeout-status" className={field.label}>
          {t("closeAs")}
          <select id="closeout-status" value={statusId} onChange={(e) => setStatusId(Number(e.target.value))} className={field.compact}>
            {closing.map((s) => (
              <option key={s.id} value={s.id}>{s.name}</option>
            ))}
          </select>
        </label>
        {pending.length > 0 && (
          <button type="button" onClick={() => review(pending[0].key)} className={button.primary}>{t("start", { count: pending.length })}</button>
        )}
        <p role="status" className="text-[13px] text-muted">{t("progress", { closed: rows.filter((r) => r.state === "closed").length, total: rows.length })}</p>
      </div>
      <ul className={cx(panel, "flex flex-col divide-y divide-line-soft")}>
        {rows.map((r) => (
          <li key={r.key} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 text-[13px]">
            <Link href={`/t/${r.key}`} className="font-mono font-semibold">{r.key}</Link>
            <span className="min-w-0 flex-1">{r.ticket?.title}</span>
            <span className={cx("text-xs", r.state === "failed" ? "text-danger" : "text-muted")}>{t(`state.${r.state}`)}</span>
            {r.ticket && r.state !== "closed" && (
              <button type="button" onClick={() => review(r.key)} className={button.quiet}>{t("review")}</button>
            )}
          </li>
        ))}
      </ul>
      {open && status && (
        <CloseDialog
          key={open.ticket.key}
          ticket={open.ticket}
          status={status}
          nodes={nodes}
          draft={open.draft}
          onCancel={() => setOpen(undefined)}
          onDone={() => {
            const key = open.ticket.key;
            set(key, { state: "closed" });
            setOpen(undefined);
            const next = pending.find((r) => r.key !== key);
            if (next) review(next.key);
          }}
        />
      )}
    </div>
  );
}
