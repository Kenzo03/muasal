"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Project } from "@/lib/problem";
import { button, cx, field, panel } from "@/lib/ui";

export default function ProjectForm({ project }: { project: Project }) {
  const t = useTranslations("settings");
  const problemText = useProblemText();
  const router = useRouter();
  const [status, setStatus] = useState("");

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const { data, error } = await api.PATCH("/projects/{key}", {
      params: { path: { key: project.key } },
      body: { key: String(form.get("key")), name: String(form.get("name")), description: String(form.get("description")) },
    });
    if (error) return setStatus(problemText(error));
    setStatus(t("saved"));
    if (data.key !== project.key) router.replace(`/p/${data.key}/settings`); // the key is in every project URL
    else router.refresh();
  }

  return (
    <form aria-label={t("general")} onSubmit={onSubmit} className={cx(panel, "flex flex-col gap-3 p-4")}>
      <h2 className="text-sm font-semibold">{t("general")}</h2>
      <div className="grid gap-3 sm:grid-cols-[160px_minmax(0,1fr)]">
        <label className={field.label}>
          {t("key")}
          <input name="key" defaultValue={project.key} required maxLength={10} className={cx(field.input, "font-mono uppercase")} />
        </label>
        <label className={field.label}>
          {t("name")}
          <input name="name" defaultValue={project.name} required maxLength={200} className={field.input} />
        </label>
      </div>
      <label className={field.label}>
        {t("description")}
        <textarea name="description" defaultValue={project.description} maxLength={2000} rows={3} className={field.textarea} />
      </label>
      <div className="flex items-center gap-3">
        <button className={button.primary}>{t("save")}</button>
        {status && <p role="status" className="text-[13px] text-muted">{status}</p>}
      </div>
    </form>
  );
}
