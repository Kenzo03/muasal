"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { day } from "@/lib/format";
import { useProblemText, type Client } from "@/lib/problem";
import { button, chip, cx, field, panel } from "@/lib/ui";

type Schedule = components["schemas"]["SummarySchedule"];

// weekdayName names an ISO weekday (1 is Monday) in the locale; 5 Jan 2026 was a Monday.
const weekdayName = (n: number, locale: string) =>
  new Date(Date.UTC(2026, 0, 4 + n)).toLocaleDateString(locale, { weekday: "long", timeZone: "UTC" });

// Weekly change summaries for project admins: each writes the past seven days'
// summary on its weekday, into the summaries list and the scheduler's bell.
export default function Schedules({ projectKey, clients, schedules }: { projectKey: string; clients: Client[]; schedules: Schedule[] }) {
  const t = useTranslations("summaries");
  const locale = useLocale();
  const router = useRouter();
  const problemText = useProblemText();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function add(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const client = String(form.get("client_id") ?? "");
    setBusy(true);
    const { error } = await api.POST("/projects/{key}/summary-schedules", {
      params: { path: { key: projectKey } },
      body: {
        weekday: Number(form.get("weekday")),
        client_id: client ? Number(client) : undefined,
        language: form.get("language") === "en" ? "en" : "id",
        audience: form.get("audience") === "internal" ? "internal" : "client",
      },
    });
    setBusy(false);
    if (error) return setError(problemText(error));
    setError("");
    router.refresh();
  }

  async function stop(id: number) {
    if (!window.confirm(t("stopConfirm"))) return;
    const { error } = await api.DELETE("/summary-schedules/{id}", { params: { path: { id } } });
    if (error) return setError(problemText(error));
    router.refresh();
  }

  return (
    <section aria-labelledby="weekly-title" className={cx(panel, "flex flex-col gap-4 p-5")}>
      <div className="flex flex-col gap-1">
        <h2 id="weekly-title" className="flex items-center gap-2 text-[15px] font-extrabold">
          <Icon name="calendar" className="size-4 text-accent" />
          {t("weekly")}
        </h2>
        <p className="max-w-3xl text-[13px] leading-relaxed text-ink-soft">{t("weeklyHint")}</p>
      </div>
      {schedules.length === 0 ? (
        <p className="text-[13px] text-muted">{t("weeklyNone")}</p>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {schedules.map((s) => (
            <li key={s.id} className="flex flex-wrap items-center gap-x-3 gap-y-1.5 rounded-xl bg-paper px-3.5 py-2.5 text-[13px]">
              <span className="font-bold">{t("every", { day: weekdayName(s.weekday, locale) })}</span>
              <span className={cx(chip, "bg-well text-ink-soft")}>{s.client?.name ?? t("allClients")}</span>
              <span className="text-ink-soft">
                {s.audience === "client" ? t("clientFacing") : t("internal")} · {s.language === "en" ? "English" : "Bahasa Indonesia"}
              </span>
              <span className="text-xs text-muted">
                {s.last_run_on ? t("lastRun", { date: day(s.last_run_on, locale) }) : t("notRunYet")} · {s.created_by.name}
              </span>
              <button type="button" onClick={() => stop(s.id)} className={cx(button.quiet, "ml-auto")}>{t("stop")}</button>
            </li>
          ))}
        </ul>
      )}
      <form onSubmit={add} aria-label={t("weeklyAdd")} className="flex flex-wrap items-end gap-3 border-t border-line-soft pt-4">
        <label className={field.label}>
          {t("day")}
          <select name="weekday" defaultValue="1" className={field.compact}>
            {[1, 2, 3, 4, 5, 6, 7].map((n) => (
              <option key={n} value={n}>{weekdayName(n, locale)}</option>
            ))}
          </select>
        </label>
        <label className={field.label}>
          {t("client")}
          <select name="client_id" defaultValue="" className={field.compact}>
            <option value="">{t("allClients")}</option>
            {clients.map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
        </label>
        <label className={field.label}>
          {t("audience")}
          <select name="audience" defaultValue="client" className={field.compact}>
            <option value="client">{t("clientFacing")}</option>
            <option value="internal">{t("internal")}</option>
          </select>
        </label>
        <label className={field.label}>
          {t("language")}
          <select name="language" defaultValue={locale === "en" ? "en" : "id"} className={field.compact}>
            <option value="id">Bahasa Indonesia</option>
            <option value="en">English</option>
          </select>
        </label>
        <button disabled={busy} className={button.primary}>
          <Icon name="plus" />
          {t("schedule")}
        </button>
      </form>
      {error && <p role="alert" className={field.error}>{error}</p>}
    </section>
  );
}
