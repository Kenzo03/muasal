"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { problemKey } from "@/lib/problem";
import { button, cx, field } from "@/lib/ui";

export default function SetupForm({ token }: { token: string }) {
  const t = useTranslations("setup");
  const tErr = useTranslations("errors");
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);
  // MSL-20: say whose password this is, and that a dead link is dead, before any typing.
  const [account, setAccount] = useState<{ name: string; email: string }>();
  const [dead, setDead] = useState(false);
  useEffect(() => {
    api.POST("/auth/setup/account", { body: { token } }).then(({ data, error }) => {
      if (data) setAccount(data);
      else if (error && problemKey(error) === "setup_link_invalid") setDead(true);
    });
  }, [token]);

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
      <div className="flex flex-col gap-3">
        <p role="status" className="text-sm text-ok">{t("done")}</p>
        <Link className={cx(button.primary, "h-9 w-full")} href={account ? `/login?${new URLSearchParams({ email: account.email })}` : "/login"}>{t("toLogin")}</Link>
      </div>
    );
  }
  if (dead) return <p role="alert" className={field.error}>{tErr("setup_link_invalid")}</p>;
  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-3.5">
      {account && <p className="text-sm text-ink-soft">{t("forAccount", account)}</p>}
      <label className={field.label}>
        {t("password")}
        <input name="password" type="password" required minLength={12} autoComplete="new-password" className={field.input} />
      </label>
      <label className={field.label}>
        {t("confirm")}
        <input name="confirm" type="password" required minLength={12} autoComplete="new-password" className={field.input} />
      </label>
      <p className={field.hint}>{t("hint")}</p>
      {error && <p role="alert" className={field.error}>{error}</p>}
      <button disabled={busy} className={cx(button.primary, "mt-1 h-9 w-full")}>
        {t("submit")}
      </button>
    </form>
  );
}
