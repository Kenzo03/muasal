"use client";

import { Fragment, useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import { utc } from "@/lib/format";
import { useProblemText, type DecisionRecord } from "@/lib/problem";
import { button, chip, cx, field, panel } from "@/lib/ui";

// The decision record on the ticket page (FSD §8.6, §9): what changed, why and
// what was rejected. Project admins and the confirmer reword a confirmed record
// (R-DC-5); a reopened ticket's record is a draft until the next close (R-DC-6).
export default function DecisionCard({ ticketKey, decision, canEdit }: { ticketKey: string; decision: DecisionRecord; canEdit: boolean }) {
  const t = useTranslations("decision");
  const locale = useLocale();
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

  const rows: [string, string][] = [
    [whatLabel, decision.what_changed],
    [t("why"), decision.why],
    [t("alternatives"), decision.alternatives],
  ];
  return (
    <section aria-labelledby="decision-title" className={cx(panel, "overflow-hidden")}>
      <div className={cx("flex flex-wrap items-center gap-2.5 border-b px-4 py-2.5", confirmed ? "border-[#D3E4D8] bg-[#EEF5F0]" : "border-line-soft bg-paper")}>
        <Icon name={confirmed ? "check" : "edit"} className={confirmed ? "text-ok" : "text-muted"} />
        <h2 id="decision-title" className="text-sm font-semibold">{t("title")}</h2>
        <span className={cx(chip, implemented ? "bg-ok-soft text-ok" : "bg-well text-[#4A423C]")}>{implemented ? t("implemented") : t("rejected")}</span>
        <span className="text-xs text-muted">
          {confirmed && decision.confirmed_by && decision.confirmed_at
            ? t("confirmedBy", { name: decision.confirmed_by.name, at: utc(decision.confirmed_at, locale) })
            : t("draft")}
        </span>
        {confirmed && canEdit && !editing && (
          <button type="button" onClick={() => setEditing(true)} className={cx(button.secondary, "ml-auto h-7")}>
            {t("edit")}
          </button>
        )}
      </div>
      {editing ? (
        <form onSubmit={save} aria-label={t("editTitle")} className="flex flex-col gap-3 p-4">
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
        <dl className="grid gap-x-4 gap-y-3 px-4 py-3.5 text-sm leading-normal md:grid-cols-[190px_minmax(0,1fr)]">
          {rows.map(([label, value]) => (
            <Fragment key={label}>
              <dt className="text-muted">{label}</dt>
              <dd className="whitespace-pre-wrap">{value || "—"}</dd>
            </Fragment>
          ))}
        </dl>
      )}
    </section>
  );
}
