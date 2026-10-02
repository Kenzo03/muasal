"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { button, cx, field } from "@/lib/ui";

const messageFor: Record<string, string> = { rate_limited: "rateLimited" };

export default function LoginForm({ email }: { email?: string }) {
  const t = useTranslations("login");
  const router = useRouter();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    const { data, error } = await api.POST("/auth/login", {
      body: { email: String(form.get("email")), password: String(form.get("password")) },
    });
    setBusy(false);
    if (error) {
      setError(t(messageFor[error.code] ?? "failed"));
      return;
    }
    document.cookie = `locale=${data.locale}; path=/; max-age=31536000; samesite=lax`;
    router.push("/");
    router.refresh();
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-3.5">
      <label className={field.label}>
        {t("email")}
        {/* MSL-20: a new user arrives from their setup page with the email filled in. */}
        <input name="email" type="email" required autoComplete="username" defaultValue={email} className={field.input} />
      </label>
      <label className={field.label}>
        {t("password")}
        <input name="password" type="password" required autoComplete="current-password" className={field.input} />
      </label>
      {error && <p role="alert" className={field.error}>{error}</p>}
      <button disabled={busy} className={cx(button.primary, "mt-1 h-9 w-full")}>
        {t("submit")}
      </button>
    </form>
  );
}
