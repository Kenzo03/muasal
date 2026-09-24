"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { Avatar } from "@/components/Chips";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import { utc } from "@/lib/format";
import { useProblemText, type ActivityItem } from "@/lib/problem";
import { button, chip, cx, field, panel } from "@/lib/ui";

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
  const locale = useLocale();
  const problemText = useProblemText();
  const router = useRouter();
  const [filter, setFilter] = useState<Filter>("all");
  const [editing, setEditing] = useState<number | null>(null);
  const [error, setError] = useState("");
  const visible = items.filter((it) => filter === "all" || (filter === "comments") === (it.kind === "comment"));
  const field_ = (k: string) => (t.has(`fields.${k}`) ? t(`fields.${k}`) : k);

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
        return t("updated", { actor, fields: Object.keys(c).map(field_).join(", ") });
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
        <details className="mt-1 pl-6">
          <summary className="cursor-pointer text-xs text-link">{t("details")}</summary>
          <ul className="mt-1 flex flex-col gap-0.5 text-xs">
            {Object.entries(c).map(([k, v]) => (
              <li key={k}>{field_(k)}: {shown(v.old)} → {shown(v.new)}</li>
            ))}
          </ul>
        </details>
      );
    }
    if (it.action === "comment_edit") {
      return (
        <details className="mt-1 pl-6">
          <summary className="cursor-pointer text-xs text-link">{t("earlier")}</summary>
          <p className="mt-1 whitespace-pre-wrap text-xs">{shown(c.body?.old)}</p>
        </details>
      );
    }
    return null;
  }

  const eventIcon = (it: ActivityItem) =>
    it.action === "transition" ? "chevronRight" : it.action === "create" ? "plus" : it.action?.startsWith("attachment") ? "file" : "edit";

  return (
    <section aria-labelledby="activity-title" className={cx(panel, "flex flex-col gap-3.5 px-4 py-3.5")}>
      <div className="flex items-center gap-3">
        <h2 id="activity-title" className="text-sm font-semibold">{t("title")}</h2>
        <div className="flex overflow-hidden rounded border border-line text-xs">
          {(["all", "comments", "history"] as const).map((f, i) => (
            <button
              key={f}
              type="button"
              aria-pressed={filter === f}
              onClick={() => setFilter(f)}
              className={cx(
                "h-[26px] cursor-pointer px-2.5",
                i > 0 && "border-l border-line",
                filter === f ? "bg-accent-soft font-semibold text-accent-strong" : "bg-white text-muted hover:text-ink",
              )}
            >
              {t(f)}
            </button>
          ))}
        </div>
      </div>
      {error && <p role="alert" className={field.error}>{error}</p>}
      <ol className="flex flex-col gap-3">
        {visible.map((it, i) =>
          it.kind === "comment" ? (
            <li key={`c${it.comment_id}`} className="flex gap-2.5">
              <Avatar name={it.actor?.name ?? "?"} className="size-7 bg-well text-ink" />
              <div className={cx("flex min-w-0 flex-1 flex-col gap-1 rounded border border-line px-3 py-2.5", it.internal && "bg-paper")}>
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
                  <span className="text-[13px] font-semibold text-ink">{it.actor?.name}</span>
                  {it.internal ? (
                    <span className={cx(chip, "bg-ink text-white")}>
                      <Icon name="lock" className="size-3" />
                      {t("internal")}
                    </span>
                  ) : (
                    <span className={cx(chip, "bg-[#DDEEE9] text-[#145B4E]")}>{t("clientSafe")}</span>
                  )}
                  <span className="ml-auto">{utc(it.at, locale)}{it.edited ? ` · ${t("edited")}` : ""}</span>
                </div>
                {it.deleted ? (
                  <p className="text-sm italic text-muted">{t("deleted")}{it.body ? `: ${it.body}` : ""}</p>
                ) : editing === it.comment_id ? (
                  <form onSubmit={(e) => saveEdit(e, it.comment_id!)} className="flex flex-col gap-2">
                    <textarea name="body" defaultValue={it.body} required maxLength={20000} rows={3} aria-label={t("edit")} className={field.textarea} />
                    <div className="flex gap-2">
                      <button className={button.primary}>{t("save")}</button>
                      <button type="button" onClick={() => setEditing(null)} className={button.secondary}>{t("cancel")}</button>
                    </div>
                  </form>
                ) : (
                  <p className="whitespace-pre-wrap text-sm leading-relaxed">{it.body}</p>
                )}
                {!it.deleted && canComment && it.actor?.id === meId && editing !== it.comment_id && (
                  <div className="flex gap-3">
                    <button type="button" onClick={() => setEditing(it.comment_id!)} className={cx(button.quiet, "text-xs")}>{t("edit")}</button>
                    <button type="button" onClick={() => remove(it.comment_id!)} className={cx(button.quiet, "text-xs")}>{t("delete")}</button>
                  </div>
                )}
              </div>
            </li>
          ) : (
            <li key={`e${i}`} className="text-[13px] text-muted">
              <div className="flex items-start gap-2">
                <Icon name={eventIcon(it)} className="mt-0.5 size-4" />
                <span className="min-w-0 flex-1">{describe(it)}</span>
                <span className="shrink-0 text-xs">{utc(it.at, locale)}</span>
              </div>
              {details(it)}
            </li>
          ),
        )}
      </ol>
      {canComment && (
        <form onSubmit={send} className="flex flex-col gap-2 border-t border-line-soft pt-3.5">
          <textarea name="body" required maxLength={20000} rows={3} aria-label={t("placeholder")} placeholder={t("placeholder")} className={field.textarea} />
          <div className="flex flex-wrap items-center gap-3">
            <label className="flex items-center gap-2 text-[13px]">
              <input type="checkbox" name="internal" defaultChecked className="size-4 accent-accent" />
              {t("internalToggle")}
            </label>
            <button className={cx(button.primary, "ml-auto")}>{t("send")}</button>
          </div>
        </form>
      )}
    </section>
  );
}
