"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText } from "@/lib/problem";
import { button, cx, field } from "@/lib/ui";

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

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-3.5">
      <div className="grid gap-3.5 sm:grid-cols-[160px_minmax(0,1fr)]">
        <label className={field.label}>
          {t("key")}
          <input name="key" required maxLength={10} aria-describedby="key-hint" className={cx(field.input, "font-mono uppercase")} />
        </label>
        <label className={field.label}>
          {t("name")}
          <input name="name" required maxLength={200} className={field.input} />
        </label>
      </div>
      <p id="key-hint" className={cx(field.hint, "-mt-2")}>{t("keyHint")}</p>
      <label className={field.label}>
        {t("description")}
        <textarea name="description" maxLength={2000} rows={3} className={field.textarea} />
      </label>
      {error && <p role="alert" className={field.error}>{error}</p>}
      <button className={cx(button.primary, "self-start")}>{t("create")}</button>
    </form>
  );
}
