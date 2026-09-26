"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import Markdown from "@/components/Markdown";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { useProblemText } from "@/lib/problem";
import { button, cx, field, panel } from "@/lib/ui";

type Summary = components["schemas"]["Summary"];

// A saved summary is editable markdown (FSD §12.1); editing it never changes
// tickets. It prints through the print view and copies as markdown.
export default function Editor({ summary }: { summary: Summary }) {
  const t = useTranslations("summaries");
  const router = useRouter();
  const problemText = useProblemText();
  const [title, setTitle] = useState(summary.title);
  const [markdown, setMarkdown] = useState(summary.markdown);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const dirty = title !== summary.title || markdown !== summary.markdown;

  async function save(e: React.FormEvent) {
    e.preventDefault();
    const { error } = await api.PATCH("/summaries/{id}", { params: { path: { id: summary.id } }, body: { title, markdown } });
    if (error) return setError(problemText(error));
    setError("");
    setStatus(t("saved"));
    router.refresh();
  }

  return (
    <form onSubmit={save} aria-label={t("summary")} className="mx-auto flex max-w-6xl flex-col gap-3">
      <div className="flex flex-wrap items-end gap-2">
        <label className={cx(field.label, "min-w-0 flex-1")}>
          {t("title")}
          <input value={title} onChange={(e) => setTitle(e.target.value)} required maxLength={300} className={field.input} />
        </label>
        <button disabled={!dirty} className={button.primary}>{t("save")}</button>
        <button
          type="button"
          className={button.secondary}
          onClick={async () => {
            await navigator.clipboard.writeText(markdown);
            setStatus(t("copied"));
          }}
        >
          {t("copy")}
        </button>
        <Link href={`/summaries/${summary.id}/print`} target="_blank" className={button.secondary}>{t("print")}</Link>
      </div>
      <p className="text-xs text-muted">
        {t("meta", { model: summary.model, items: summary.items.length, audience: summary.scope.audience === "client" ? t("clientFacing") : t("internal") })}
      </p>
      {status && <p role="status" className="text-xs text-ok">{status}</p>}
      {error && <p role="alert" className={field.error}>{error}</p>}
      <div className="grid gap-3 lg:grid-cols-2">
        <label className="flex flex-col gap-1.5 text-[13px] font-semibold">
          {t("markdown")}
          <textarea value={markdown} onChange={(e) => setMarkdown(e.target.value)} rows={28} className={cx(field.textarea, "font-mono text-xs")} />
        </label>
        <section aria-label={t("previewLabel")} className={cx(panel, "overflow-auto p-4")}>
          <Markdown text={markdown} />
        </section>
      </div>
    </form>
  );
}
