"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText } from "@/lib/problem";
import { button, field } from "@/lib/ui";

// Archives a finished project, or restores an archived one (MSL-64).
export default function ArchiveButton({ projectKey, name, archived }: { projectKey: string; name: string; archived: boolean }) {
  const t = useTranslations("archive");
  const router = useRouter();
  const problemText = useProblemText();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function go() {
    if (!archived && !window.confirm(t("confirm", { name }))) return;
    setBusy(true);
    const params = { params: { path: { key: projectKey } } };
    const { error } = archived ? await api.POST("/projects/{key}/restore", params) : await api.POST("/projects/{key}/archive", params);
    setBusy(false);
    if (error) return setError(problemText(error));
    router.refresh();
  }

  return (
    <span className="flex flex-col items-start gap-1">
      <button type="button" onClick={go} disabled={busy} className={archived ? button.secondary : button.danger}>
        {archived ? t("restore") : t("archive")}
      </button>
      {error && <span role="alert" className={field.error}>{error}</span>}
    </span>
  );
}
