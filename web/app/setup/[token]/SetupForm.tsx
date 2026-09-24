"use client";

import Link from "next/link";
import { useState } from "react";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { problemKey } from "@/lib/problem";

export default function SetupForm({ token }: { token: string }) {
  const t = useTranslations("setup");
  const tErr = useTranslations("errors");
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const password = String(form.get("password"));
    if (password !== String(form.get("confirm"))) {
      setError(t("mismatch"));
      return;
    }
    setBusy(true);
    const { error } = await api.POST("/auth/setup", { body: { token, password } });
    setBusy(false);
    if (error) {
      const key = problemKey(error);
      setError(tErr.has(key) ? tErr(key) : tErr("generic"));
      return;
    }
    setDone(true);
  }

  if (done) {
    return (
      <div className="flex flex-col gap-4">
        <p role="status">{t("done")}</p>
        <Link className="underline" href="/login">{t("toLogin")}</Link>
      </div>
    );
  }
  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4">
      <label className="flex flex-col gap-1 text-sm">
        {t("password")}
        <input name="password" type="password" required minLength={12} autoComplete="new-password" className="rounded border px-3 py-2" />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("confirm")}
        <input name="confirm" type="password" required minLength={12} autoComplete="new-password" className="rounded border px-3 py-2" />
      </label>
      <p className="text-xs text-neutral-500">{t("hint")}</p>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <button disabled={busy} className="rounded bg-neutral-900 px-4 py-2 text-white disabled:opacity-50">
        {t("submit")}
      </button>
    </form>
  );
}
