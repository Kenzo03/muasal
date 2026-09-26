"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import type { User } from "@/lib/problem";
import { button, field } from "@/lib/ui";

const events = ["assigned", "comment", "mention", "status", "job_done"] as const;

// Settings → Profile → Notifications (FSD §8.10): each event on or off, and
// browser notifications, which also ask the browser's permission.
export default function NotifyPrefsForm({ me }: { me: User }) {
  const t = useTranslations("notifyPrefs");
  const router = useRouter();
  const prefs = me.notify_prefs ?? {};
  const [status, setStatus] = useState("");

  async function save(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const next: Record<string, boolean> = {};
    for (const ev of events) next[ev] = form.get(ev) === "on";
    next.browser = form.get("browser") === "on";
    if (next.browser && "Notification" in window && Notification.permission === "default") {
      next.browser = (await Notification.requestPermission()) === "granted";
    }
    const { error } = await api.PATCH("/me", { body: { notify_prefs: next } });
    setStatus(error ? t("failed") : t("saved"));
    router.refresh();
  }

  return (
    <form onSubmit={save} aria-label={t("title")} className="flex flex-col gap-2.5">
      <h2 className="text-sm font-semibold">{t("title")}</h2>
      {events.map((ev) => (
        <label key={ev} className="flex items-center gap-2 text-[13px]">
          <input type="checkbox" name={ev} defaultChecked={prefs[ev] !== false} className="size-4 accent-accent" />
          {t(`events.${ev}`)}
        </label>
      ))}
      <label className="flex items-center gap-2 text-[13px]">
        <input type="checkbox" name="browser" defaultChecked={prefs.browser === true} className="size-4 accent-accent" />
        {t("browser")}
      </label>
      <p className={field.hint}>{t("browserHint")}</p>
      <div className="flex items-center gap-3">
        <button className={button.secondary}>{t("save")}</button>
        {status && <span role="status" className="text-[13px] text-muted">{status}</span>}
      </div>
    </form>
  );
}
