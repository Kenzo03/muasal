"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText } from "@/lib/problem";
import { button, cx, field } from "@/lib/ui";

// "Run backup now": the backup service picks it up within 30 seconds (§19.4).
export default function RunBackup({ requested }: { requested: boolean }) {
  const t = useTranslations("backups");
  const problemText = useProblemText();
  const router = useRouter();
  const [state, setState] = useState<"idle" | "busy" | "done">(requested ? "done" : "idle");
  const [error, setError] = useState("");
  return (
    <div className="ml-auto flex items-center gap-3">
      {state === "done" && <span role="status" className="text-[13px] text-muted">{t("requested")}</span>}
      {error && <span role="alert" className={field.error}>{error}</span>}
      <button
        type="button"
        disabled={state !== "idle"}
        className={cx(button.primary)}
        onClick={async () => {
          setState("busy");
          const { error } = await api.POST("/admin/backups/run");
          if (error) {
            setError(problemText(error));
            return setState("idle");
          }
          setState("done");
          setTimeout(() => router.refresh(), 45_000);
        }}
      >
        {t("runNow")}
      </button>
    </div>
  );
}
