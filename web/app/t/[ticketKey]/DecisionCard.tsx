"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useLocale, useTimeZone, useTranslations } from "next-intl";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import { dateTime } from "@/lib/format";
import { useProblemText, type DecisionRecord } from "@/lib/problem";
import { button, chip, cx, field, panel } from "@/lib/ui";

// The decision record on the ticket page (FSD §8.6, §9): what changed, why and
// what was rejected. Project admins and the confirmer reword a confirmed record
// (R-DC-5); a reopened ticket's record is a draft until the next close (R-DC-6).
export default function DecisionCard({ ticketKey, decision, canEdit }: { ticketKey: string; decision: DecisionRecord; canEdit: boolean }) {
  const t = useTranslations("decision");
  const locale = useLocale();
  const timeZone = useTimeZone();
  const router = useRouter();
  const problemText = useProblemText();
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState("");
  const confirmed = decision.state === "confirmed";
  const implemented = decision.outcome === "implemented";
  const whatLabel = implemented ? t("whatChanged") : t("whatDecided");

  async function save(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const { error } = await api.PUT("/tickets/{key}/decision", {
      params: { path: { key: ticketKey } },
      body: { what_changed: String(form.get("what_changed")), why: String(form.get("why")), alternatives: String(form.get("alternatives")) },
    });
    if (error) return setError(problemText(error));
    setEditing(false);
    setError("");
    router.refresh();
  }

  return (
    <section aria-labelledby="decision-title" className={cx(panel, "overflow-hidden")}>
      <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1 px-5 pb-1 pt-4">
        <span className={cx("flex size-7 items-center justify-center rounded-full", confirmed ? "bg-ok-soft text-ok" : "bg-well text-muted")}>
          <Icon name={confirmed ? "check" : "edit"} />
        </span>
        <h2 id="decision-title" className="text-[15px] font-extrabold">{t("title")}</h2>
        <span className={cx(chip, "rounded-full", implemented ? "bg-ok-soft text-ok" : "bg-well text-ink-soft")}>{implemented ? t("implemented") : t("rejected")}</span>
        <span className="text-xs text-muted">
          {confirmed && decision.confirmed_by && decision.confirmed_at
            ? t("confirmedBy", { name: decision.confirmed_by.name, at: dateTime(decision.confirmed_at, locale, timeZone) })
            : t("draft")}
        </span>
        {confirmed && canEdit && !editing && (
          <button type="button" onClick={() => setEditing(true)} className={cx(button.quiet, "ml-auto")}>
            <Icon name="edit" className="size-3.5" />
            {t("edit")}
          </button>
        )}
      </div>
      {decision.superseded_by && (
        <p className="mx-5 mt-3 flex items-center gap-2 rounded-xl border border-warn-line bg-warn-soft px-3.5 py-2.5 text-[13px] font-medium text-warn">
          <Icon name="warning" />
          {t.rich("supersededBy", { key: decision.superseded_by, link: (chunks) => <Link href={`/t/${decision.superseded_by}`} className="font-bold">{chunks}</Link> })}
        </p>
      )}
      {editing ? (
        <form onSubmit={save} aria-label={t("editTitle")} className="flex flex-col gap-3 px-5 pb-5 pt-3">
          <label className={field.label}>
            {whatLabel}
            <textarea name="what_changed" defaultValue={decision.what_changed} required minLength={10} maxLength={1000} rows={2} className={field.textarea} />
          </label>
          <label className={field.label}>
            {t("why")}
            <textarea name="why" defaultValue={decision.why} required minLength={10} maxLength={2000} rows={3} className={field.textarea} />
          </label>
          <label className={field.label}>
            {t("alternatives")}
            <textarea name="alternatives" defaultValue={decision.alternatives} maxLength={2000} rows={2} className={field.textarea} />
          </label>
          {error && <p role="alert" className={field.error}>{error}</p>}
          <div className="flex gap-2">
            <button className={button.primary}>{t("save")}</button>
            <button type="button" onClick={() => setEditing(false)} className={button.secondary}>{t("cancel")}</button>
          </div>
        </form>
      ) : (
        // What changed leads; why and the rejected alternatives sit side by side under it.
        <dl className="grid gap-3 px-5 pb-5 pt-3 md:grid-cols-2">
          <div className="flex flex-col gap-1.5 pb-1 md:col-span-2">
            <dt className="text-[12.5px] font-bold text-muted">{whatLabel}</dt>
            <dd className="whitespace-pre-wrap text-base font-semibold leading-relaxed [text-wrap:pretty]">{decision.what_changed || "—"}</dd>
          </div>
          <div className="flex flex-col gap-1.5 rounded-xl bg-paper px-4 py-3.5">
            <dt className="flex items-center gap-1.5 text-[12.5px] font-bold text-ink-soft">
              <Icon name="help" className="size-3.5 text-ok" />
              {t("why")}
            </dt>
            <dd className="whitespace-pre-wrap text-sm leading-relaxed">{decision.why || "—"}</dd>
          </div>
          <div className="flex flex-col gap-1.5 rounded-xl bg-paper px-4 py-3.5">
            <dt className="flex items-center gap-1.5 text-[12.5px] font-bold text-ink-soft">
              <Icon name="xCircle" className="size-3.5 text-muted" />
              {t("alternatives")}
            </dt>
            <dd className="whitespace-pre-wrap text-sm leading-relaxed">{decision.alternatives || "—"}</dd>
          </div>
        </dl>
      )}
    </section>
  );
}
