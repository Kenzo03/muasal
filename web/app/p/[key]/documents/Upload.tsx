"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import type { components } from "@/lib/api-types";
import { documentExtensions, toMarkdown } from "@/lib/convert";
import { useProblemText, type Client, type Problem } from "@/lib/problem";
import { button, cx, field, panel } from "@/lib/ui";

type DocumentListItem = components["schemas"]["DocumentListItem"];

// Upload (§7.7): the browser converts the file to Markdown first, so headings
// become sections; the server keeps both.
export default function Upload({ projectKey, clients, documents }: { projectKey: string; clients: Client[]; documents: DocumentListItem[] }) {
  const t = useTranslations("documents");
  const router = useRouter();
  const problemText = useProblemText();
  const [title, setTitle] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");

  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const file = form.get("file") as File;
    if (file.size > 20 << 20) return setError(t("tooLarge"));
    setError("");
    setBusy(t("converting"));
    let markdown: string;
    try {
      markdown = await toMarkdown(file);
    } catch {
      setBusy("");
      return setError(t("unreadable"));
    }
    form.set("markdown", markdown);
    if (!form.get("client_id")) form.delete("client_id");
    if (!form.get("supersedes")) form.delete("supersedes");
    setBusy(t("uploading"));
    const res = await fetch(`/api/v1/projects/${encodeURIComponent(projectKey)}/documents`, { method: "POST", body: form });
    setBusy("");
    const body = await res.json().catch(() => ({}));
    if (!res.ok) {
      const p = body as Problem;
      return setError(p.errors?.[0]?.message ?? problemText(p));
    }
    router.push(`/documents/${body.key}`);
  }

  return (
    <form onSubmit={submit} aria-label={t("upload")} className={cx(panel, "flex flex-wrap items-end gap-3 p-4")}>
      <label className={field.label}>
        {t("file")}
        <input
          type="file"
          name="file"
          required
          accept={documentExtensions.join(",")}
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f && !title) setTitle(f.name.replace(/\.[^.]+$/, ""));
          }}
          className="text-sm"
        />
      </label>
      <label className={field.label}>
        {t("title")}
        <input name="title" required maxLength={200} value={title} onChange={(e) => setTitle(e.target.value)} className={field.input} />
      </label>
      <label className={field.label}>
        {t("client")}
        <select name="client_id" defaultValue="" className={field.input}>
          <option value="">{t("allClients")}</option>
          {clients.map((c) => (
            <option key={c.id} value={c.id}>{c.name}</option>
          ))}
        </select>
      </label>
      {documents.length > 0 && (
        <label className={field.label}>
          {t("supersedes")}
          <select name="supersedes" defaultValue="" className={field.input}>
            <option value="">{t("supersedesNone")}</option>
            {documents.filter((d) => !d.superseded_by).map((d) => (
              <option key={d.key} value={d.key}>{d.key} · {d.title}</option>
            ))}
          </select>
        </label>
      )}
      <button disabled={busy !== ""} className={button.primary}>{busy || t("upload")}</button>
      {error && <p role="alert" className={cx(field.error, "w-full")}>{error}</p>}
      <p className="w-full text-xs text-muted">{t("uploadHint")}</p>
    </form>
  );
}
