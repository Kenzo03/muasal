"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { useProblemText } from "@/lib/problem";
import { button } from "@/lib/ui";

type TicketCreate = components["schemas"]["TicketCreate"];

// MSL-60: files every action item still without a ticket, each prefilled as
// its "Create ticket" link is, and linked to the note.
export default function CreateAll({ projectKey, tickets }: { projectKey: string; tickets: TicketCreate[] }) {
  const t = useTranslations("notes");
  const router = useRouter();
  const problemText = useProblemText();
  const [busy, setBusy] = useState("");
  const [failed, setFailed] = useState<string[]>([]);

  async function createAll() {
    const bad: string[] = [];
    for (const [i, body] of tickets.entries()) {
      setBusy(t("creating", { done: i + 1, total: tickets.length }));
      const { error } = await api.POST("/projects/{key}/tickets", { params: { path: { key: projectKey } }, body });
      if (error) bad.push(`${body.title}: ${problemText(error)}`);
    }
    setBusy("");
    setFailed(bad);
    router.refresh();
  }

  return (
    <div className="flex flex-col gap-1">
      <button type="button" onClick={createAll} disabled={busy !== ""} className={button.secondary}>
        {busy || t("createAll", { count: tickets.length })}
      </button>
      {failed.length > 0 && (
        <p role="alert" className="text-xs text-danger">
          {t("createFailed")} {failed.join("; ")}
        </p>
      )}
    </div>
  );
}
