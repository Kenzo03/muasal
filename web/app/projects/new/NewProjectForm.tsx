"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText } from "@/lib/problem";

export default function NewProjectForm() {
  const t = useTranslations("newProject");
  const problemText = useProblemText();
  const router = useRouter();
  const [error, setError] = useState("");

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const { data, error } = await api.POST("/projects", {
      body: { key: String(form.get("key")), name: String(form.get("name")), description: String(form.get("description")) },
    });
    if (error) return setError(problemText(error));
    router.push(`/p/${data.key}/settings`); // next: link clients and add members
  }

  const input = "rounded border px-3 py-2";
  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4">
      <label className="flex flex-col gap-1 text-sm">
        {t("key")}
        <input name="key" required maxLength={10} aria-describedby="key-hint" className={`${input} font-mono uppercase`} />
      </label>
      <p id="key-hint" className="-mt-2 text-xs text-neutral-500">{t("keyHint")}</p>
      <label className="flex flex-col gap-1 text-sm">
        {t("name")}
        <input name="name" required maxLength={200} className={input} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("description")}
        <textarea name="description" maxLength={2000} rows={3} className={input} />
      </label>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("create")}</button>
    </form>
  );
}
