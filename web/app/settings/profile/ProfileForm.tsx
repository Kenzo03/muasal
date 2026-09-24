"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { problemKey, type User } from "@/lib/problem";
import { button, field } from "@/lib/ui";

export default function ProfileForm({ me }: { me: User }) {
  const t = useTranslations("profile");
  const tErr = useTranslations("errors");
  const router = useRouter();
  const [status, setStatus] = useState("");
  // The server renders only the current zone; the browser adds the full list, so hydration always matches.
  const [zones, setZones] = useState<string[]>([me.timezone]);
  useEffect(() => setZones(Intl.supportedValuesOf("timeZone")), []);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const newPassword = String(form.get("new_password") ?? "");
    const { data, error } = await api.PATCH("/me", {
      body: {
        name: String(form.get("name")),
        locale: form.get("locale") === "en" ? "en" : "id",
        timezone: String(form.get("timezone")),
        ...(newPassword ? { current_password: String(form.get("current_password")), new_password: newPassword } : {}),
      },
    });
    if (error) {
      const key = problemKey(error);
      setStatus(tErr.has(key) ? tErr(key) : tErr("generic"));
      return;
    }
    document.cookie = `locale=${data.locale}; path=/; max-age=31536000; samesite=lax`;
    setStatus(t("saved"));
    router.refresh();
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-3.5">
      <label className={field.label}>
        {t("name")}
        <input name="name" defaultValue={me.name} required maxLength={200} className={field.input} />
      </label>
      <div className="grid gap-3.5 sm:grid-cols-2">
        <label className={field.label}>
          {t("language")}
          <select name="locale" defaultValue={me.locale} className={field.input}>
            <option value="id">Bahasa Indonesia</option>
            <option value="en">English</option>
          </select>
        </label>
        <label className={field.label}>
          {t("timezone")}
          <select name="timezone" defaultValue={me.timezone} className={field.input}>
            {zones.map((z) => (
              <option key={z} value={z}>{z}</option>
            ))}
          </select>
        </label>
      </div>
      <fieldset className="flex flex-col gap-3.5 border-t border-line-soft pt-3.5">
        <legend className="mb-1 text-sm font-semibold">{t("passwordTitle")}</legend>
        <label className={field.label}>
          {t("currentPassword")}
          <input name="current_password" type="password" autoComplete="current-password" className={field.input} />
        </label>
        <label className={field.label}>
          {t("newPassword")}
          <input name="new_password" type="password" minLength={12} autoComplete="new-password" className={field.input} />
        </label>
      </fieldset>
      <div className="flex items-center gap-3">
        <button className={button.primary}>{t("save")}</button>
        {status && <p role="status" className="text-[13px] text-muted">{status}</p>}
      </div>
    </form>
  );
}
