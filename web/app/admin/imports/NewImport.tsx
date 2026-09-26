"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { button, cx, field, panel } from "@/lib/ui";

// Upload a file into a new import; the server dry-runs it at once.
export default function NewImport({ projects }: { projects: { key: string; name: string }[] }) {
  const t = useTranslations("imports");
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function upload(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    const res = await fetch("/api/v1/imports", { method: "POST", body: new FormData(e.currentTarget) });
    setBusy(false);
    const body = await res.json().catch(() => undefined);
    if (!res.ok) return setError(body?.errors?.[0]?.message ?? body?.title ?? t("failed"));
    router.push(`/admin/imports/${body.id}`);
  }

  return (
    <form onSubmit={upload} aria-label={t("new")} className={cx(panel, "flex flex-wrap items-end gap-3 p-4")}>
      <label className={field.label}>
        {t("project")}
        <select name="project_key" required className={field.input}>
          {projects.map((p) => (
            <option key={p.key} value={p.key}>{p.key} · {p.name}</option>
          ))}
        </select>
      </label>
      <label className={field.label}>
        {t("preset")}
        <select name="preset" defaultValue="jira" className={field.input}>
          <option value="jira">{t("presets.jira")}</option>
          <option value="csv">{t("presets.csv")}</option>
        </select>
      </label>
      <label className={field.label}>
        {t("file")}
        <input type="file" name="file" required accept=".csv,text/csv" className="text-[13px]" />
      </label>
      <button disabled={busy} className={button.primary}>{busy ? t("uploading") : t("upload")}</button>
      {error && <p role="alert" className={cx(field.error, "w-full")}>{error}</p>}
    </form>
  );
}
