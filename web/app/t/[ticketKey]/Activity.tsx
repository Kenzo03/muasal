"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { utc } from "@/lib/format";
import { useProblemText, type ActivityItem } from "@/lib/problem";

type Change = { old?: unknown; new?: unknown };
type Filter = "all" | "comments" | "history";

const shown = (v: unknown) =>
  v === null || v === undefined || v === "" ? "—" : Array.isArray(v) ? v.join(", ") || "—" : String(v);

// A ticket's comments and history, oldest first (FSD §8.7).
export default function Activity({ ticketKey, items, meId, canComment }: {
  ticketKey: string;
  items: ActivityItem[];
  meId: number;
  canComment: boolean;
}) {
  const t = useTranslations("activity");
  const problemText = useProblemText();
  const router = useRouter();
  const [filter, setFilter] = useState<Filter>("all");
  const [editing, setEditing] = useState<number | null>(null);
  const [error, setError] = useState("");
  const visible = items.filter((it) => filter === "all" || (filter === "comments") === (it.kind === "comment"));
  const field = (k: string) => (t.has(`fields.${k}`) ? t(`fields.${k}`) : k);

  async function send(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const formEl = e.currentTarget;
    const form = new FormData(formEl);
    const { error } = await api.POST("/tickets/{key}/comments", {
      params: { path: { key: ticketKey } },
      body: { body: String(form.get("body")), internal: form.get("internal") === "on" },
    });
    if (error) return setError(problemText(error));
    formEl.reset();
    setError("");
    router.refresh();
  }

  async function saveEdit(e: React.FormEvent<HTMLFormElement>, id: number) {
    e.preventDefault();
    const { error } = await api.PATCH("/comments/{id}", {
      params: { path: { id } },
      body: { body: String(new FormData(e.currentTarget).get("body")) },
    });
    if (error) return setError(problemText(error));
    setEditing(null);
    setError("");
    router.refresh();
  }

  async function remove(id: number) {
    if (!window.confirm(t("confirmDelete"))) return;
    const { error } = await api.DELETE("/comments/{id}", { params: { path: { id } } });
    if (error) return setError(problemText(error));
    router.refresh();
  }

  function describe(it: ActivityItem) {
    const actor = it.actor?.name ?? t("system");
    const c = (it.changes ?? {}) as Record<string, unknown>;
    switch (it.action) {
      case "create":
        return t("created", { actor });
      case "transition": {
        const s = c.status as Change;
        return t("transition", { actor, old: shown(s.old), new: shown(s.new) });
      }
      case "update":
        return t("updated", { actor, fields: Object.keys(c).map(field).join(", ") });
      case "comment_edit":
        return t("commentEdited", { actor });
      case "comment_delete":
        return t("commentDeleted", { actor });
      case "attachment_add":
        return t("attachmentAdded", { actor, file: shown(c.filename) });
      case "attachment_delete":
        return t("attachmentDeleted", { actor, file: shown(c.filename) });
      default:
        return `${actor}: ${it.action}`;
    }
  }

  // Updates list their old and new values; comment edits keep the earlier text (AC-TK-6).
  function details(it: ActivityItem) {
    const c = (it.changes ?? {}) as Record<string, Change>;
    if (it.action === "update") {
      return (
        <details className="mt-1">
          <summary className="cursor-pointer text-xs">{t("details")}</summary>
          <ul className="mt-1 text-xs">
            {Object.entries(c).map(([k, v]) => (
              <li key={k}>{field(k)}: {shown(v.old)} → {shown(v.new)}</li>
            ))}
          </ul>
        </details>
      );
    }
    if (it.action === "comment_edit") {
      return (
        <details className="mt-1">
          <summary className="cursor-pointer text-xs">{t("earlier")}</summary>
          <p className="mt-1 whitespace-pre-wrap text-xs">{shown(c.body?.old)}</p>
        </details>
      );
    }
    return null;
  }

  const tab = (f: Filter, label: string) => (
    <button type="button" aria-pressed={filter === f} onClick={() => setFilter(f)} className={filter === f ? "font-semibold underline" : "hover:underline"}>
      {label}
    </button>
  );

  return (
    <section aria-labelledby="activity-title" className="flex flex-col gap-3">
      <div className="flex items-center gap-4">
        <h2 id="activity-title" className="font-medium">{t("title")}</h2>
        <div className="flex gap-3 text-sm">
          {tab("all", t("all"))}
          {tab("comments", t("comments"))}
          {tab("history", t("history"))}
        </div>
      </div>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <ol className="flex flex-col gap-2">
        {visible.map((it, i) =>
          it.kind === "comment" ? (
            <li key={`c${it.comment_id}`} className="rounded border bg-white p-3 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{it.actor?.name}</span>
                <span className="rounded bg-neutral-100 px-1.5 text-xs">{it.internal ? t("internal") : t("clientSafe")}</span>
                <span className="text-xs text-neutral-500">{utc(it.at)}{it.edited ? ` · ${t("edited")}` : ""}</span>
              </div>
              {it.deleted ? (
                <p className="mt-1 italic text-neutral-500">{t("deleted")}{it.body ? `: ${it.body}` : ""}</p>
              ) : editing === it.comment_id ? (
                <form onSubmit={(e) => saveEdit(e, it.comment_id!)} className="mt-2 flex flex-col gap-2">
                  <textarea name="body" defaultValue={it.body} required maxLength={20000} rows={3} aria-label={t("edit")} className="rounded border px-3 py-2" />
                  <div className="flex gap-2">
                    <button className="rounded bg-neutral-900 px-3 py-1 text-white">{t("save")}</button>
                    <button type="button" onClick={() => setEditing(null)} className="rounded border px-3 py-1">{t("cancel")}</button>
                  </div>
                </form>
              ) : (
                <p className="mt-1 whitespace-pre-wrap">{it.body}</p>
              )}
              {!it.deleted && canComment && it.actor?.id === meId && editing !== it.comment_id && (
                <div className="mt-2 flex gap-3 text-xs">
                  <button type="button" onClick={() => setEditing(it.comment_id!)} className="underline">{t("edit")}</button>
                  <button type="button" onClick={() => remove(it.comment_id!)} className="underline">{t("delete")}</button>
                </div>
              )}
            </li>
          ) : (
            <li key={`e${i}`} className="text-sm text-neutral-600">
              {describe(it)} · <span className="text-xs">{utc(it.at)}</span>
              {details(it)}
            </li>
          ),
        )}
      </ol>
      {canComment && (
        <form onSubmit={send} className="flex flex-col gap-2">
          <textarea name="body" required maxLength={20000} rows={3} aria-label={t("placeholder")} placeholder={t("placeholder")} className="rounded border px-3 py-2" />
          <div className="flex items-center gap-3 text-sm">
            <label className="flex items-center gap-2">
              <input type="checkbox" name="internal" defaultChecked />
              {t("internalToggle")}
            </label>
            <button className="ml-auto rounded bg-neutral-900 px-4 py-2 text-white">{t("send")}</button>
          </div>
        </form>
      )}
    </section>
  );
}
