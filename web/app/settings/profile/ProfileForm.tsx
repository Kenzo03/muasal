"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { problemKey, type User } from "@/lib/problem";

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

  const input = "rounded border px-3 py-2";
  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4">
      <label className="flex flex-col gap-1 text-sm">
        {t("name")}
        <input name="name" defaultValue={me.name} required maxLength={200} className={input} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("language")}
        <select name="locale" defaultValue={me.locale} className={input}>
          <option value="id">Bahasa Indonesia</option>
          <option value="en">English</option>
        </select>
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("timezone")}
        <select name="timezone" defaultValue={me.timezone} className={input}>
          {zones.map((z) => (
            <option key={z} value={z}>{z}</option>
          ))}
        </select>
      </label>
      <fieldset className="flex flex-col gap-4 border-t pt-4">
        <legend className="text-sm font-medium">{t("passwordTitle")}</legend>
        <label className="flex flex-col gap-1 text-sm">
          {t("currentPassword")}
          <input name="current_password" type="password" autoComplete="current-password" className={input} />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          {t("newPassword")}
          <input name="new_password" type="password" minLength={12} autoComplete="new-password" className={input} />
        </label>
      </fieldset>
      {status && <p role="status" className="text-sm">{status}</p>}
      <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("save")}</button>
    </form>
  );
}
