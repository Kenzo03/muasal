"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { useProblemText } from "@/lib/problem";
import { button, cx, field, panel, sectionTitle } from "@/lib/ui";

type Release = components["schemas"]["Release"];

// Releases (MSL-67): add one such as v1.0, rename it, or set the day it
// shipped; tickets name their release, and a summary can cover one.
export default function Releases({ projectKey, releases }: { projectKey: string; releases: Release[] }) {
  const t = useTranslations("releases");
  const router = useRouter();
  const problemText = useProblemText();
  const [error, setError] = useState("");
  const body = (form: FormData) => ({ name: String(form.get("name")), released_on: String(form.get("released_on") ?? "") || undefined });

  async function add(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const formEl = e.currentTarget;
    const { error } = await api.POST("/projects/{key}/releases", { params: { path: { key: projectKey } }, body: body(new FormData(formEl)) });
    if (error) return setError(problemText(error));
    setError("");
    formEl.reset();
    router.refresh();
  }
  async function save(e: React.FormEvent<HTMLFormElement>, id: number) {
    e.preventDefault();
    const { error } = await api.PATCH("/releases/{id}", { params: { path: { id } }, body: body(new FormData(e.currentTarget)) });
    if (error) return setError(problemText(error));
    setError("");
    router.refresh();
  }

  return (
    <section aria-labelledby="releases-title" className={cx(panel, "flex flex-col gap-3 p-4")}>
      <h2 id="releases-title" className={sectionTitle}>{t("title")}</h2>
      <p className="text-[13px] text-muted">{t("intro")}</p>
      {releases.map((r) => (
        <form key={r.id} onSubmit={(e) => save(e, r.id)} aria-label={r.name} className="flex flex-wrap items-end gap-2">
          <label htmlFor={`release-${r.id}-name`} className={field.label}>
            {t("name")}
            <input id={`release-${r.id}-name`} name="name" required maxLength={50} defaultValue={r.name} className={field.compact} />
          </label>
          <label htmlFor={`release-${r.id}-date`} className={field.label}>
            {t("releasedOn")}
            <input id={`release-${r.id}-date`} type="date" name="released_on" defaultValue={r.released_on ?? ""} className={field.compact} />
          </label>
          <button className={button.secondary}>{t("save")}</button>
          <a href={`/p/${projectKey}/tickets?release=${r.id}`} className="pb-2 text-[13px]">{t("tickets")}</a>
        </form>
      ))}
      <form onSubmit={add} aria-label={t("add")} className="flex flex-wrap items-end gap-2 border-t border-line-soft pt-3">
        <label htmlFor="release-new-name" className={field.label}>
          {t("name")}
          <input id="release-new-name" name="name" required maxLength={50} placeholder="v1.0" className={field.compact} />
        </label>
        <label htmlFor="release-new-date" className={field.label}>
          {t("releasedOn")}
          <input id="release-new-date" type="date" name="released_on" className={field.compact} />
        </label>
        <button className={button.secondary}>{t("add")}</button>
      </form>
      {error && <p role="alert" className={field.error}>{error}</p>}
    </section>
  );
}
