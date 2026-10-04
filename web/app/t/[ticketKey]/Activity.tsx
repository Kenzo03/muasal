"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTimeZone, useTranslations } from "next-intl";
import { Avatar } from "@/components/Chips";
import Icon from "@/components/Icon";
import Markdown from "@/components/Markdown";
import MentionBox from "@/components/MentionBox";
import { api } from "@/lib/api";
import { describeChange, shown, type Change } from "@/lib/activity";
import { dateTime } from "@/lib/format";
import { pasteImages } from "@/lib/paste";
import { useProblemText, type ActivityItem } from "@/lib/problem";
import { button, chip, cx, field, panel } from "@/lib/ui";
import type { Person } from "@/lib/mentions";

type Filter = "all" | "comments" | "history";

// A ticket's comments and history, oldest first (FSD §8.7).
export default function Activity({ ticketKey, items, meId, canComment, people }: {
  ticketKey: string;
  items: ActivityItem[];
  meId: number;
  canComment: boolean;
  people: Person[]; // their @handles show as names (MSL-30)
}) {
  const t = useTranslations("activity");
  const locale = useLocale();
  const timeZone = useTimeZone();
  const problemText = useProblemText();
  const router = useRouter();
  const [filter, setFilter] = useState<Filter>("all");
  const [editing, setEditing] = useState<number | null>(null);
  const [error, setError] = useState("");
  const paste = pasteImages(ticketKey, (p) => setError(problemText(p)), () => router.refresh());
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

  // Updates list their old and new values; comment edits keep the earlier text (AC-TK-6).
  function details(it: ActivityItem) {
    const c = (it.changes ?? {}) as Record<string, Change>;
    // Updates, closes and decision records list their old and new values.
    if (it.action === "update" || it.action?.startsWith("decision_") || (it.action === "transition" && Object.keys(c).length > 1)) {
      return (
        <details className="mt-1 pl-11">
          <summary className="cursor-pointer text-xs font-semibold text-link">{t("details")}</summary>
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
        <details className="mt-1 pl-11">
          <summary className="cursor-pointer text-xs font-semibold text-link">{t("earlier")}</summary>
          <p className="mt-1 whitespace-pre-wrap text-xs">{shown(c.body?.old)}</p>
        </details>
      );
    }
    return null;
  }

  const eventIcon = (it: ActivityItem) =>
    it.action === "transition" ? "chevronRight"
    : it.action === "create" ? "plus"
    : it.action === "decision_confirm" ? "check"
    : it.action?.startsWith("attachment") ? "file"
    : "edit";

  return (
    <section aria-labelledby="activity-title" className={cx(panel, "flex flex-col gap-4 px-5 py-4")}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="activity-title" className="text-base font-extrabold">{t("title")}</h2>
        <div className="flex gap-0.5 rounded-[11px] bg-well p-[3px] text-[13px]">
          {(["all", "comments", "history"] as const).map((f) => (
            <button
              key={f}
              type="button"
              aria-pressed={filter === f}
              onClick={() => setFilter(f)}
              className={cx(
                "h-[30px] cursor-pointer rounded-lg px-3",
                filter === f ? "bg-white font-bold text-ink shadow-[0_1px_2px_rgba(43,36,32,0.1)]" : "font-semibold text-ink-soft hover:text-ink",
              )}
            >
              {t(f)}
            </button>
          ))}
        </div>
      </div>
      {error && <p role="alert" className={field.error}>{error}</p>}
      <ol className="flex flex-col gap-3.5">
        {visible.map((it, i) =>
          it.kind === "comment" ? (
            <li key={`c${it.comment_id}`} className="flex gap-2.5">
              <Avatar name={it.actor?.name ?? "?"} className="size-8 bg-accent-soft text-[11px] font-bold text-accent-strong" />
              <div className="flex min-w-0 flex-1 flex-col gap-1.5 rounded-[4px_16px_16px_16px] border border-line bg-white px-4 py-3">
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
                  <span className="text-[13.5px] font-bold text-ink">{it.actor?.name}</span>
                  {it.internal ? (
                    <span className={cx(chip, "bg-warn-soft text-warn")}>
                      <Icon name="lock" className="size-3" />
                      {t("internal")}
                    </span>
                  ) : (
                    <span className={cx(chip, "bg-ok-soft text-ok")}>{t("clientSafe")}</span>
                  )}
                  <span className="ml-auto">{dateTime(it.at, locale, timeZone)}{it.edited ? ` · ${t("edited")}` : ""}</span>
                </div>
                {it.deleted ? (
                  <p className="text-sm italic text-muted">{t("deleted")}{it.body ? `: ${it.body}` : ""}</p>
                ) : editing === it.comment_id ? (
                  <form onSubmit={(e) => saveEdit(e, it.comment_id!)} className="flex flex-col gap-2">
                    <textarea name="body" defaultValue={it.body} required maxLength={20000} rows={3} aria-label={t("edit")} onPaste={paste} className={field.textarea} />
                    <div className="flex gap-2">
                      <button className={button.primary}>{t("save")}</button>
                      <button type="button" onClick={() => setEditing(null)} className={button.secondary}>{t("cancel")}</button>
                    </div>
                  </form>
                ) : (
                  <Markdown text={it.body ?? ""} people={people} />
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
            <li key={`e${i}`} className="text-[13px] text-ink-soft">
              <div className="flex items-center gap-3">
                <span className="flex w-8 shrink-0 justify-center">
                  <span className={cx("flex size-6 items-center justify-center rounded-full", it.action === "decision_confirm" ? "bg-ok-soft text-ok" : "bg-well text-muted")}>
                    <Icon name={eventIcon(it)} className="size-3.5" />
                  </span>
                </span>
                <span className="min-w-0 flex-1">{describeChange(t, it)}</span>
                <span className="shrink-0 text-xs text-muted">{dateTime(it.at, locale, timeZone)}</span>
              </div>
              {details(it)}
            </li>
          ),
        )}
      </ol>
      {canComment && (
        <form onSubmit={send} className="flex flex-col gap-2.5 border-t border-line-soft pt-4">
          <MentionBox ticketKey={ticketKey} name="body" required maxLength={20000} rows={3} aria-label={t("placeholder")} placeholder={t("placeholder")} onPaste={paste} />
          <p className={field.hint}>{t("markdownHint")} {t("mentionHint")}</p>
          <div className="flex flex-wrap items-center gap-3">
            <label htmlFor="comment-internal" className="flex items-center gap-2 text-[13px] font-semibold">
              <input id="comment-internal" type="checkbox" name="internal" defaultChecked className="size-4 accent-accent" />
              {t("internalToggle")}
            </label>
            <button className={cx(button.primary, "ml-auto")}>{t("send")}</button>
          </div>
        </form>
      )}
    </section>
  );
}
