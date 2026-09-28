"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import Icon from "@/components/Icon";
import type { components } from "@/lib/api-types";
import { documentExtensions, toMarkdown } from "@/lib/convert";
import { fileSize } from "@/lib/format";
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
  const [file, setFile] = useState<File>();
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
    <form onSubmit={submit} aria-label={t("upload")} className={cx(panel, "flex flex-col gap-4 p-5")}>
      <h2 className="text-[15px] font-extrabold">{t("uploadTitle")}</h2>
      {/* The file input spans the zone, invisible, so a click opens the picker and a dropped file lands in it. */}
      <label className="relative flex flex-col items-center gap-1.5 rounded-xl border-2 border-dashed border-field bg-paper px-4 py-6 text-center hover:border-accent has-[:focus-visible]:border-accent has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-accent">
        <span className="sr-only">{t("file")}</span>
        <span aria-hidden="true" className="mb-1 flex size-10 items-center justify-center rounded-full bg-white text-accent shadow-[0_1px_3px_rgba(43,36,32,0.12)]">
          <Icon name="upload" className="size-5" />
        </span>
        <span aria-hidden="true" className={cx("max-w-full text-[13.5px] font-semibold text-ink", file && "truncate")}>{file ? file.name : t("drop")}</span>
        <span aria-hidden="true" className="text-xs text-muted">{file ? fileSize(file.size) : t("fileTypes")}</span>
        <input
          type="file"
          name="file"
          required
          accept={documentExtensions.join(",")}
          onChange={(e) => {
            const f = e.target.files?.[0];
            setFile(f);
            if (f && !title) setTitle(f.name.replace(/\.[^.]+$/, ""));
          }}
          className="absolute inset-0 cursor-pointer opacity-0"
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
      <button disabled={busy !== ""} className={cx(button.primary, "w-full")}>
        <Icon name="upload" />
        {busy || t("upload")}
      </button>
      {error && <p role="alert" className={field.error}>{error}</p>}
      <p className="text-xs leading-relaxed text-muted">{t("uploadHint")}</p>
    </form>
  );
}
