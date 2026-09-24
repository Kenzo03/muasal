"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Project } from "@/lib/problem";

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

  const input = "rounded border px-3 py-2";
  return (
    <form aria-label={t("general")} onSubmit={onSubmit} className="flex flex-col gap-4 rounded-lg border bg-white p-4">
      <h2 className="font-medium">{t("general")}</h2>
      <label className="flex flex-col gap-1 text-sm">
        {t("key")}
        <input name="key" defaultValue={project.key} required maxLength={10} className={`${input} font-mono uppercase`} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("name")}
        <input name="name" defaultValue={project.name} required maxLength={200} className={input} />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("description")}
        <textarea name="description" defaultValue={project.description} maxLength={2000} rows={3} className={input} />
      </label>
      {status && <p role="status" className="text-sm">{status}</p>}
      <button className="self-start rounded bg-neutral-900 px-4 py-2 text-white">{t("save")}</button>
    </form>
  );
}
