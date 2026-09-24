"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { fileSize } from "@/lib/format";
import { useProblemText, type Ticket } from "@/lib/problem";

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
    <section aria-labelledby="attachments-title" className="flex flex-col gap-3">
      <h2 id="attachments-title" className="font-medium">{t("title")}</h2>
      {files.length === 0 ? (
        <p className="text-sm text-neutral-600">{t("none")}</p>
      ) : (
        <ul className="flex flex-col gap-1 text-sm">
          {files.map((f) => (
            <li key={f.id} className="flex items-center gap-2">
              <a href={`/api/v1/attachments/${f.id}`} className="underline">{f.filename}</a>
              <span className="text-xs text-neutral-500">{fileSize(f.size_bytes)}</span>
              {canUpload && (f.uploader.id === meId || isProjectAdmin) && (
                <button type="button" onClick={() => remove(f.id)} className="ml-auto text-xs underline">{t("remove")}</button>
              )}
            </li>
          ))}
        </ul>
      )}
      {canUpload && (
        <label className="flex flex-col gap-1 text-sm">
          {t("upload")}
          <input type="file" onChange={upload} disabled={busy} />
        </label>
      )}
      <p className="text-xs text-neutral-500">{t("limit")}</p>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
    </section>
  );
}
