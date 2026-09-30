"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Problem } from "@/lib/problem";
import { suggestKey } from "@/lib/projects";
import { button, cx, field } from "@/lib/ui";

// templates are the projects a new one can copy its statuses and module tree from.
export default function NewProjectForm({ templates }: { templates: { key: string; name: string }[] }) {
  const t = useTranslations("newProject");
  const problemText = useProblemText();
  const router = useRouter();
  const [problem, setProblem] = useState<Problem>();
  const [key, setKey] = useState("");
  const typed = useRef(false); // MSL-36: the key follows the name until the user types one
  // The fields the server rejected get a red border.
  const bad = (name: string) => problem?.errors?.some((e) => e.field === name) || undefined;

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const template = String(form.get("template_key") ?? "");
    const { data, error } = await api.POST("/projects", {
      // The key shows in capitals, so it is sent in capitals.
      body: {
        key: String(form.get("key")).toUpperCase(), name: String(form.get("name")), description: String(form.get("description")),
        template_key: template || undefined,
      },
    });
    if (error) return setProblem(error);
    router.push(`/p/${data.key}/settings`); // next: link clients and add members
    router.refresh(); // the sidebar lists projects from the layout, which a push keeps
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-3.5">
      <div className="grid gap-3.5 sm:grid-cols-[160px_minmax(0,1fr)]">
        <label className={field.label}>
          {t("key")}
          {/* The browser checks the server's rule first and points at this field. */}
          <input
            name="key"
            value={key}
            onChange={(e) => {
              typed.current = e.target.value !== "";
              setKey(e.target.value);
            }}
            required
            maxLength={10}
            pattern="[A-Za-z][A-Za-z0-9]{1,9}"
            title={t("keyHint")}
            aria-invalid={bad("key")}
            aria-describedby="key-hint"
            className={cx(field.input, "font-mono uppercase")}
          />
        </label>
        <label className={field.label}>
          {t("name")}
          <input
            name="name"
            required
            maxLength={200}
            onChange={(e) => !typed.current && setKey(suggestKey(e.target.value))}
            aria-invalid={bad("name")}
            className={field.input}
          />
        </label>
      </div>
      <p id="key-hint" className={cx(field.hint, "-mt-2")}>{t("keyHint")}</p>
      <label className={field.label}>
        {t("description")}
        <textarea name="description" maxLength={2000} rows={3} aria-invalid={bad("description")} className={field.textarea} />
      </label>
      {templates.length > 0 && (
        <div className="flex flex-col gap-1.5">
          <label className={field.label}>
            {t("template")}
            <select name="template_key" defaultValue="" aria-invalid={bad("template_key")} aria-describedby="template-hint" className={field.input}>
              <option value="">{t("templateNone")}</option>
              {templates.map((p) => (
                <option key={p.key} value={p.key}>{p.key} · {p.name}</option>
              ))}
            </select>
          </label>
          <p id="template-hint" className={field.hint}>{t("templateHint")}</p>
        </div>
      )}
      {problem && <p role="alert" className={field.error}>{problemText(problem)}</p>}
      <button className={cx(button.primary, "self-start")}>{t("create")}</button>
    </form>
  );
}
