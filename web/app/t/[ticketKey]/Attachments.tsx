"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import { fileSize } from "@/lib/format";
import { useProblemText, type Ticket } from "@/lib/problem";
import { button, cx, field, panel, sectionTitle } from "@/lib/ui";

type Props = { ticketKey: string; files: Ticket["attachments"]; meId: number; isProjectAdmin: boolean; canUpload: boolean };

// A ticket's files (FSD §8.7). Go serves each download after the ticket's visibility check.
export default function Attachments({ ticketKey, files, meId, isProjectAdmin, canUpload }: Props) {
  const t = useTranslations("attachments");
  const problemText = useProblemText();
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function upload(e: React.ChangeEvent<HTMLInputElement>) {
    const input = e.currentTarget;
    const file = input.files?.[0];
    if (!file) return;
    const form = new FormData();
    form.append("file", file);
    setBusy(true);
    // openapi-fetch sends JSON, so the multipart upload uses fetch directly (same origin, same cookie).
    const res = await fetch(`/api/v1/tickets/${encodeURIComponent(ticketKey)}/attachments`, { method: "POST", body: form });
    setBusy(false);
    input.value = "";
    if (!res.ok) return setError(problemText(await res.json().catch(() => undefined)));
    setError("");
    router.refresh();
  }

  async function remove(id: number) {
    const { error } = await api.DELETE("/attachments/{id}", { params: { path: { id } } });
    if (error) return setError(problemText(error));
    router.refresh();
  }

  return (
    <section aria-labelledby="attachments-title" className={cx(panel, "flex flex-col gap-2.5 px-4 py-3.5")}>
      <h2 id="attachments-title" className={sectionTitle}>{t("title")}</h2>
      {files.length === 0 ? (
        <p className="text-[13px] text-muted">{t("none")}</p>
      ) : (
        <ul className="flex flex-col gap-2 text-[13px]">
          {files.map((f) => (
            <li key={f.id} className="flex items-center gap-2">
              <Icon name="file" className="size-4 text-muted" />
              <a href={`/api/v1/attachments/${f.id}`} className="min-w-0 truncate">{f.filename}</a>
              <span className="shrink-0 text-xs text-muted">{fileSize(f.size_bytes)}</span>
              {canUpload && (f.uploader.id === meId || isProjectAdmin) && (
                <button type="button" onClick={() => remove(f.id)} className={cx(button.quiet, "ml-auto text-xs")}>{t("remove")}</button>
              )}
            </li>
          ))}
        </ul>
      )}
      {canUpload && (
        <label className={cx(button.secondary, "w-full border-dashed border-field bg-paper", busy && "opacity-40")}>
          <Icon name="upload" />
          {t("upload")}
          <input type="file" onChange={upload} disabled={busy} className="sr-only" />
        </label>
      )}
      <p className={field.hint}>{t("limit")}</p>
      {error && <p role="alert" className={field.error}>{error}</p>}
    </section>
  );
}
